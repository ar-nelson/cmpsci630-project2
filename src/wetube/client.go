package wetube

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"encoding/gob"
	"fmt"
	"github.com/gorilla/websocket"
	"log"
	"math"
	"math/big"
	mrand "math/rand"
	"time"
)

type Client struct {
	Id                     int32
	Port                   uint16
	Name                   string
	Rank                   Rank
	PrivateKey             *rsa.PrivateKey
	BrowserConnect         chan chan *BrowserMessage
	ToBrowser              chan<- *BrowserMessage
	BrowserTimeout         *time.Timer
	LeaderTimeout          *time.Timer
	HeartbeatTicker        *time.Ticker
	ElectionRetryTimeout   *time.Timer
	OutstandingInvitations map[int32]*OutstandingInvitation
	isLeader               bool
	leader                 *Peer
	peers                  map[int32]*Peer
	video                  *Video
	nextVoteRoundN         uint
	voteRound              *LeaderVoteRound
	c_isLeader             chan chan bool
	c_getLeader            chan chan *Peer
	c_becomeLeader         chan bool
	c_setLeader            chan *Peer
	c_deletePeer           chan int32
	c_addPeer              chan *Peer
	c_updatePeer           chan *RosterEntry
	c_getPeer              chan req_getPeer
	c_listPeers            chan chan *Peer
	c_getRoster            chan chan []*RosterEntry
	c_updateRoster         chan []*RosterEntry
	c_getVideo             chan chan Video
	c_updateVideo          chan *VideoInstant
	c_leaderVote           chan *LeaderVote
	c_electLeader          chan bool
}

func NewClient(id int32, port uint16) *Client {
	client := &Client{
		Id:             id,
		Port:           port,
		BrowserConnect: make(chan chan *BrowserMessage, 1),
		peers:          make(map[int32]*Peer),
		video: &Video{
			VideoInstant: VideoInstant{
				Id:             DefaultVideo,
				State:          Unstarted,
				SecondsElapsed: 0.0,
			},
			LastUpdate: time.Now(),
		},
		c_isLeader:     make(chan chan bool, 16),
		c_getLeader:    make(chan chan *Peer, 16),
		c_becomeLeader: make(chan bool, 1),
		c_setLeader:    make(chan *Peer, 1),
		c_deletePeer:   make(chan int32, 16),
		c_addPeer:      make(chan *Peer, 16),
		c_updatePeer:   make(chan *RosterEntry, 16),
		c_getPeer:      make(chan req_getPeer, 16),
		c_listPeers:    make(chan chan *Peer, 16),
		c_getRoster:    make(chan chan []*RosterEntry, 16),
		c_updateRoster: make(chan []*RosterEntry, 16),
		c_getVideo:     make(chan chan Video, 16),
		c_updateVideo:  make(chan *VideoInstant, 16),
		c_leaderVote:   make(chan *LeaderVote, 91),
		c_electLeader:  make(chan bool, 16),
	}
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		panic(fmt.Sprintf("generating rsa keys: %s", err))
	}
	client.PrivateKey = privateKey
	go func() {
		for {
			select {
			case c := <-client.c_isLeader:
				c <- client.isLeader
				close(c)
			case c := <-client.c_getLeader:
				c <- client.leader
				close(c)
			case <-client.c_becomeLeader:
				client.becomeLeader()
			case leader := <-client.c_setLeader:
				client.setLeader(leader)
			case peerId := <-client.c_deletePeer:
				delete(client.peers, peerId)
				go client.syncRoster()
			case newPeer := <-client.c_addPeer:
				if _, existing := client.peers[newPeer.Id]; existing {
					log.Printf("Cannot add peer with id %d; peer already exists.", newPeer.Id)
				} else {
					client.peers[newPeer.Id] = newPeer
				}
				go client.syncRoster()
			case entry := <-client.c_updatePeer:
				if peer, ok := client.peers[entry.Id]; ok {
					if entry.Name != "" {
						peer.Name = entry.Name
					}
					if entry.Address != "" {
						peer.Address = entry.Address
					}
					peer.Rank = entry.Rank
				} else {
					log.Printf("Cannot update peer with id %d; peer does not exist.", entry.Id)
				}
				go client.syncRoster()
			case req := <-client.c_getPeer:
				if peer, ok := client.peers[req.id]; ok {
					req.c <- peer
				} else {
					if client.leader != nil && req.id == client.leader.Id {
						req.c <- client.leader
					} else {
						req.c <- nil
					}
				}
				close(req.c)
			case c := <-client.c_listPeers:
				if client.leader != nil {
					c <- client.leader
				}
				for _, peer := range client.peers {
					c <- peer
				}
				close(c)
			case c := <-client.c_getRoster:
				var roster, remaining []*RosterEntry
				if client.leader == nil {
					roster = make([]*RosterEntry, len(client.peers)+1)
					remaining = roster[1:]
				} else {
					roster = make([]*RosterEntry, len(client.peers)+2)
					roster[1] = client.leader.ToRosterEntry()
					remaining = roster[2:]
				}
				roster[0] = client.ToRosterEntry()
				i := 0
				for _, peer := range client.peers {
					remaining[i] = peer.ToRosterEntry()
					i++
				}
				c <- roster
				close(c)
			case roster := <-client.c_updateRoster:
				newPeers := make(map[int32]*Peer)
				for _, entry := range roster {
					if entry.Id == client.Id {
						client.Name = entry.Name
						client.Rank = entry.Rank
						continue
					}
					if client.leader != nil && entry.Id == client.leader.Id {
						client.leader.Name = entry.Name
						client.leader.Rank = entry.Rank
						continue
					}
					if peer, ok := client.peers[entry.Id]; ok {
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
				for id, peer := range client.peers {
					if _, found := newPeers[id]; !found {
						peer.CloseChannel <- true
					}
				}
				client.peers = newPeers
				go client.syncRoster()
			case c := <-client.c_getVideo:
				c <- *client.video
				close(c)
			case video := <-client.c_updateVideo:
				oldVideo := client.video.ToInstant()
				// If they're the same video with less than 2 seconds of difference, don't bother updating.
				if video.Id != oldVideo.Id || video.State != oldVideo.State ||
					math.Abs(video.SecondsElapsed-oldVideo.SecondsElapsed) >= 2 {
					// ...otherwise, update the video information.
					client.video = &Video{*video, time.Now()}
					log.Printf("Video state update: %v, state %d, at %f seconds.", video.Id, video.State,
						video.SecondsElapsed)
				}
			case vote := <-client.c_leaderVote:
				client.acceptLeaderVote(vote)
			case <-client.c_electLeader:
				client.electNewLeader()
			}
		}
	}()
	return client
}

type req_getPeer struct {
	id int32
	c  chan *Peer
}

func (client *Client) IsLeader() bool {
	c := make(chan bool, 1)
	client.c_isLeader <- c
	return <-c
}

func (client *Client) GetLeader() *Peer {
	c := make(chan *Peer, 1)
	client.c_getLeader <- c
	return <-c
}

func (client *Client) BecomeLeader() {
	client.c_becomeLeader <- true
}

func (client *Client) SetLeader(newLeader *Peer) {
	client.c_setLeader <- newLeader
}

func (client *Client) DeletePeer(id int32) {
	client.c_deletePeer <- id
}

func (client *Client) AddPeer(peer *Peer) {
	client.c_addPeer <- peer
}

func (client *Client) UpdatePeer(data *RosterEntry) {
	client.c_updatePeer <- data
}

func (client *Client) GetPeer(id int32) *Peer {
	c := make(chan *Peer, 1)
	client.c_getPeer <- req_getPeer{id, c}
	return <-c
}

func (client *Client) ListPeers() chan *Peer {
	c := make(chan *Peer, 256)
	client.c_listPeers <- c
	return c
}

func (client *Client) GetRoster() []*RosterEntry {
	c := make(chan []*RosterEntry, 1)
	client.c_getRoster <- c
	return <-c
}

func (client *Client) UpdateRoster(roster []*RosterEntry) {
	client.c_updateRoster <- roster
}

func (client *Client) GetVideo() *Video {
	c := make(chan Video, 1)
	client.c_getVideo <- c
	v := <-c
	return &v
}

func (client *Client) UpdateVideo(video *VideoInstant) {
	client.c_updateVideo <- video
}

func (client *Client) AcceptLeaderVote(vote *LeaderVote) {
	client.c_leaderVote <- vote
}

func (client *Client) ElectNewLeader() {
	client.c_electLeader <- true
}

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

		client.DeletePeer(peer.Id)
	}()

	return peer, nil
}

func (client *Client) ToRosterEntry() *RosterEntry {
	return &RosterEntry{
		Id:   client.Id,
		Name: client.Name,
		// Address is left blank; client can't determine its own address.
		Port: client.Port,
		Rank: client.Rank,
		PublicKey: &SerializedPublicKey{
			N: client.PrivateKey.PublicKey.N.Bytes(),
			E: client.PrivateKey.PublicKey.E,
		},
	}
}

func (peer *Peer) ToRosterEntry() *RosterEntry {
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

func (client *Client) SyncRosterWithBrowser() {
	bmessage, err := NewBrowserMessage(T_RosterUpdate, RosterUpdate{client.GetRoster()})
	if err == nil {
		client.ToBrowser <- bmessage
	} else {
		log.Printf("roster update to browser: %s", err)
	}
}

func (client *Client) BroadcastToPeers(t MsgType, message interface{}) {
	pmessage, err := NewPeerMessage(client, t, message)
	if err != nil {
		log.Printf("PeerMessage encoding: %s", err)
		return
	}
	for peer := range client.ListPeers() {
		peer.OutChannel <- pmessage
	}
}

func (client *Client) syncRoster() {
	client.SyncRosterWithBrowser()
	if client.IsLeader() {
		client.BroadcastToPeers(T_RosterUpdate, RosterUpdate{client.GetRoster()})
	}
}

func (client *Client) TrySendToBrowser(t MsgType, payload interface{}) {
	browserMessage, err := NewBrowserMessage(t, payload)
	if err == nil {
		client.ToBrowser <- browserMessage
	} else {
		log.Printf("BrowserMessage encode failed: %s", err)
	}
}

func (client *Client) TrySendToPeer(peer *Peer, t MsgType, payload interface{}) {
	peerMessage, err := NewPeerMessage(client, t, payload)
	if err == nil {
		peer.OutChannel <- peerMessage
	} else {
		log.Printf("PeerMessage encode failed: %s", err)
	}
}

func (client *Client) becomeLeader() {
	client.Rank = Director
	client.OutstandingInvitations = make(map[int32]*OutstandingInvitation)
	if !client.isLeader {
		if client.leader != nil {
			client.leader.CloseChannel <- true
			client.leader = nil
		}
		client.isLeader = true
		for _, peer := range client.peers {
			peer.Timeout = time.AfterFunc(peerHeartbeatTimeout, func() {
				log.Printf("Timed out waiting for Heartbeat from peer at %s.", peer.Address)
				peer.CloseChannel <- true
			})
		}
	}
	log.Println("Became leader.")
	go client.syncRoster()
}

func (client *Client) setLeader(leader *Peer) {
	if leader == nil {
		if client.leader != nil {
			client.leader.CloseChannel <- true
		}
		client.leader = nil
		log.Printf("Leader set to nil.")
		go client.syncRoster()
		return
	}
	if client.isLeader {
		if leader.Id == client.Id {
			return
		} else {
			panic("Tried to set leader when already the leader. This shouldn't happen.")
		}
	}
	if peer, ok := client.peers[leader.Id]; ok {
		leader = peer
		delete(client.peers, leader.Id)
	}
	if client.leader != nil {
		client.leader.CloseChannel <- true
	}
	client.leader = leader
	log.Printf("Leader updated: %s (id %d, addr %s:%d)", leader.Name, leader.Id, leader.Address,
		leader.Port)
	go client.syncRoster()
}

func (client *Client) acceptLeaderVote(vote *LeaderVote) {
	if client.voteRound == nil {
		client.electNewLeader()
		client.acceptLeaderVote(vote)
	} else if vote.N > client.voteRound.N {
		client.nextVoteRoundN = vote.N
		client.electNewLeader()
		client.acceptLeaderVote(vote)
	} else if vote.N == client.voteRound.N {
		client.voteRound.Votes[vote.Sender] = vote.Vote
		peerCount := len(client.peers)
		if client.leader != nil {
			peerCount++
		}
		if len(client.voteRound.Votes) >= peerCount {
			client.ElectionRetryTimeout.Stop()
			tally := make(map[int32]int32)
			tally[client.voteRound.MyVote] = 1
			for _, vote := range client.voteRound.Votes {
				if count, ok := tally[vote]; ok {
					tally[vote] = count + 1
				} else {
					tally[vote] = 1
				}
			}
			if len(tally) == 1 {
				myVote := client.voteRound.MyVote
				if myVote == client.Id {
					client.becomeLeader()
				} else if peer, ok := client.peers[myVote]; ok {
					client.setLeader(peer)
				} else if client.leader == nil || myVote != client.leader.Id {
					log.Printf("Could not elect leader with ID %d; no peer with this ID.")
					go client.ElectNewLeader()
					return
				} else {
					log.Println("Election concluded by electing the existing leader.")
				}
				client.voteRound = nil
			} else {
				var (
					max      int32 = 0
					majority       = client.voteRound.MyVote
				)
				for tvote, count := range tally {
					if count > max || (count == max && tvote < majority) {
						max = count
						majority = tvote
					}
				}
				client.electSpecificLeader(majority)
			}
		}
	}
}

func (client *Client) electNewLeader() {
	// Elect the director with the lowest ID, or the peer with the lowest ID if no directors.
	if client.leader != nil {
		client.electSpecificLeader(client.leader.Id)
		return
	}
	var min int32 = math.MinInt32
	if client.Rank == Director {
		min = client.Id
	}
	for _, peer := range client.peers {
		if peer.Rank == Director && peer.Id < min {
			min = peer.Id
		}
	}
	if min == math.MinInt32 {
		min = client.Id
		for _, peer := range client.peers {
			if peer.Id < min {
				min = peer.Id
			}
		}
	}
	client.electSpecificLeader(min)
}

func (client *Client) electSpecificLeader(vote int32) {
	log.Println("Attempting to elect new leader.")
	if client.ElectionRetryTimeout != nil {
		client.ElectionRetryTimeout.Stop()
	}
	if client.leader == nil && len(client.peers) == 0 && vote == client.Id {
		client.becomeLeader()
		return
	}
	client.voteRound = &LeaderVoteRound{
		N:      client.nextVoteRoundN,
		MyVote: vote,
		Votes:  make(map[int32]int32),
	}
	client.nextVoteRoundN++
	client.ElectionRetryTimeout = time.AfterFunc(time.Duration(250+mrand.Intn(500))*time.Millisecond, func() {
		if !client.IsLeader() && client.GetLeader() == nil {
			client.ElectNewLeader()
		}
	})
	go client.BroadcastToPeers(T_LeaderVote, LeaderVote{client.voteRound.N, vote, client.Id})
}
