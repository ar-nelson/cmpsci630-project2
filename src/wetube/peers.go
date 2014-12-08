package wetube

import (
	"crypto/rsa"
	"crypto/tls"
	"encoding/gob"
	"fmt"
	"github.com/gorilla/websocket"
	"log"
	"math/big"
	"time"
)

func (client *Client) establishPeerConnection(rosterEntry *RosterEntry) (*Peer, error) {
	// Open a WebSocket connection to the peer.
	dialer := websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
		TLSClientConfig:  &tls.Config{InsecureSkipVerify: true},
	}
	conn, _, err := dialer.Dial(
		fmt.Sprintf("wss://%s:%d/peerSocket", rosterEntry.Address, rosterEntry.Port), nil)
	if err != nil {
		return nil, err
	}
	log.Printf("Opening output socket for peer at %s.", rosterEntry.Address)

	// Build the Peer struct.
	var (
		closeChannel = make(chan bool, 1)
		inChannel    = make(chan *PeerMessage, 91)
		peer         = &Peer{
			Id:           rosterEntry.Id,
			Name:         rosterEntry.Name,
			Address:      rosterEntry.Address,
			Port:         rosterEntry.Port,
			Rank:         rosterEntry.Rank,
			CloseChannel: closeChannel,
			OutChannel:   inChannel,
		}
	)
	if rosterEntry.PublicKey != nil {
		peer.PublicKey = &rsa.PublicKey{
			N: &big.Int{},
			E: rosterEntry.PublicKey.E,
		}
		peer.PublicKey.N.SetBytes(rosterEntry.PublicKey.N)
	}

	// If this client is the leader, set up a timeout.
	//if client.IsLeader {
	//	peer.Timeout = time.AfterFunc(peerHeartbeatTimeout, func() {
	//		log.Printf("Timed out waiting for Heartbeat from peer at %s.", peer.Address)
	//		closeChannel <- true
	//	})
	//}

	// Read from the connection, and discard everything received. This connection is only used for
	// output, so its only purpose is to notify us of an error.
	running := true
	go func() {
		for running {
			if _, _, err := conn.NextReader(); err != nil {
				log.Printf("peerSocket external close: %s", err)
				closeChannel <- true
				running = false
			}
		}
	}()

	// Start a goroutine to send messages to the peer.
	go func() {
		for running {
			select {
			case <-closeChannel:
				running = false
			case message := <-inChannel:
				writer, err := conn.NextWriter(websocket.BinaryMessage)
				if err != nil {
					log.Printf("peerSocket send: %s", err)
					running = false
				} else {
					encoder := gob.NewEncoder(writer)
					err = encoder.Encode(message)
					if err != nil {
						log.Printf("peerSocket encode: %s", err)
					}
					writer.Close()
				}
			}
		}
		log.Printf("Closing output socket for peer at %s.", peer.Address)
		conn.Close()

		client.PeersMutex.Lock()
		_, exists := (*client.Peers)[peer.Id]
		delete(*client.Peers, peer.Id)
		client.PeersMutex.Unlock()

		if exists {
			client.PeersMutex.RLock()
			err := client.SyncRosterWithBrowser()
			if err != nil {
				log.Printf("roster update to browser: %s", err)
			}
			if client.IsLeader {
				err = client.BroadcastToPeers(T_RosterUpdate, RosterUpdate{client.Roster()})
				if err != nil {
					log.Printf("roster update broadcast: %s", err)
				}
			}
			client.PeersMutex.RUnlock()
		}
	}()

	return peer, nil
}

func (peer *Peer) toRosterEntry() *RosterEntry {
	return &RosterEntry{
		Id:      peer.Id,
		Name:    peer.Name,
		Address: peer.Address,
		Port:    peer.Port,
		Rank:    peer.Rank,
		PublicKey: &SerializedPublicKey{
			N: peer.PublicKey.N.Bytes(),
			E: peer.PublicKey.E,
		},
	}
}

func (client *Client) Roster() []*RosterEntry {
	client.PeersMutex.RLock()
	var (
		roster    []*RosterEntry
		nonleader []*RosterEntry
	)
	if client.Leader == nil {
		roster = make([]*RosterEntry, len(*client.Peers))
		nonleader = roster
	} else {
		roster = make([]*RosterEntry, len(*client.Peers)+1)
		roster[0] = client.Leader.toRosterEntry()
		nonleader = roster[1:]
	}
	i := 0
	for _, peer := range *client.Peers {
		nonleader[i] = peer.toRosterEntry()
		i++
	}
	client.PeersMutex.RUnlock()
	return roster
}

func (client *Client) UpdateRoster(roster []*RosterEntry) {
	client.PeersMutex.Lock()
	newPeers := make(map[int32]*Peer)
	for _, entry := range roster {
		if entry.Id == client.Id {
			client.Name = entry.Name
			client.Rank = entry.Rank
			continue
		}
		if peer, ok := (*client.Peers)[entry.Id]; ok {
			newPeers[entry.Id] = peer
			peer.Name = entry.Name
			peer.Rank = entry.Rank
		} else {
			newPeer, err := client.establishPeerConnection(entry)
			if err == nil {
				newPeers[entry.Id] = newPeer
			} else {
				fmt.Printf("Cannot connect to peer '%s' at %s: %s",
					entry.Name, entry.Address, err)
			}
		}
	}
	for id, peer := range *client.Peers {
		if _, found := newPeers[id]; !found {
			peer.CloseChannel <- true
		}
	}
	log.Printf("Roster updated [%d peer(s)]:", len(newPeers))
	for _, peer := range newPeers {
		log.Printf("- %s (id %d, addr %s:%d)", peer.Name, peer.Id, peer.Address, peer.Port)
	}
	client.Peers = &newPeers
	client.PeersMutex.Unlock()
}

func (client *Client) SyncRosterWithBrowser() error {
	bmessage, err := NewBrowserMessage(T_RosterUpdate, RosterUpdate{client.Roster()})
	if err != nil {
		return err
	}
	client.ToBrowser <- bmessage
	return nil
}

func (client *Client) BroadcastToPeers(t MsgType, message interface{}) error {
	pmessage, err := NewPeerMessage(client, t, message)
	if err != nil {
		return err
	}
	client.PeersMutex.RLock()
	for _, peer := range *client.Peers {
		peer.OutChannel <- pmessage
	}
	client.PeersMutex.RUnlock()
	return nil
}

func (client *Client) SetLeader(leader *Peer) {
	client.PeersMutex.Lock()
	defer client.PeersMutex.Unlock()
	if client.IsLeader {
		if leader.Id == client.Id {
			return
		} else {
			panic("Tried to set leader when already the leader. This shouldn't happen.")
		}
	}
	if peer, ok := (*client.Peers)[leader.Id]; ok {
		leader = peer
		delete(*client.Peers, leader.Id)
	}
	if client.Leader != nil {
		client.Leader.CloseChannel <- true
	}
	client.Leader = leader
	log.Printf("Leader updated: %s (id %d, addr %s:%d)", leader.Name, leader.Id, leader.Address,
		leader.Port)
}

func (client *Client) BecomeLeader() {
	client.PeersMutex.Lock()
	client.Rank = Director
	client.OutstandingInvitations = make(map[int32]*OutstandingInvitation)
	if !client.IsLeader {
		if client.Leader != nil {
			client.Leader.CloseChannel <- true
			client.Leader = nil
		}
		client.IsLeader = true
		for _, peer := range *client.Peers {
			peer.Timeout = time.AfterFunc(peerHeartbeatTimeout, func() {
				log.Printf("Timed out waiting for Heartbeat from peer at %s.", peer.Address)
				peer.CloseChannel <- true
			})
		}
	}
	client.PeersMutex.Unlock()
	err := client.SyncRosterWithBrowser()
	if err != nil {
		log.Printf("roster update to browser: %s", err)
	}
	err = client.BroadcastToPeers(T_RosterUpdate, RosterUpdate{client.Roster()})
	if err != nil {
		log.Printf("roster update broadcast: %s", err)
	}
}
