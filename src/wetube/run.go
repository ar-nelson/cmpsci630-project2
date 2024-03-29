package wetube

import (
	"crypto/rsa"
	"fmt"
	"log"
	"math/big"
	"math/rand"
	"net/http"
	"time"
)

const (
	browserConnectionTimeout time.Duration = 30 * time.Second
	browserHeartbeatTimeout                = 10 * time.Second
	peerHeartbeatInterval                  = 5 * time.Second
	peerHeartbeatTimeout                   = 10 * time.Second

	PublicKeyFile  string = "certs/ssl_cert_public_key.rsa"
	PrivateKeyFile        = "certs/ssl_cert_private_key.rsa"

	DefaultVideo string = "M7lc1UVf-VE"
)

func (client *Client) Run(input chan Message, serveHtml bool) bool {
	go func() {
		mux := http.NewServeMux()
		if serveHtml {
			mux.Handle("/", http.FileServer(http.Dir("./web/")))
		}
		mux.HandleFunc("/browserSocket", browserSocketHandler(client, input))
		mux.HandleFunc("/peerSocket", peerSocketHandler(client, input))
		log.Printf("Starting WeTube HTTP server on localhost:%d...", client.Port)
		err := http.ListenAndServeTLS(fmt.Sprintf(":%d", client.Port),
			PublicKeyFile, PrivateKeyFile, mux)
		if err != nil {
			log.Fatalf("wetube http server: %s", err)
		}
	}()
	return connectToBrowser(client, input) &&
		startSession(client, input) &&
		waitForInvitation(client, input) &&
		eventLoop(client, input)
}

func connectToBrowser(client *Client, input <-chan Message) bool {
	browserConnectionTimer := time.NewTimer(browserConnectionTimeout)
	select {
	case <-browserConnectionTimer.C:
		log.Fatal("Timed out waiting for browser to connect.")
		return false
	case browserChan := <-client.BrowserConnect:
		client.ToBrowser = browserChan
		msg, err := NewBrowserMessage(T_BrowserConnectAck, BrowserConnectAck{true, "", client.Id})
		if err != nil {
			log.Fatalf("NewBrowserMessage: %s", err)
			return false
		}
		browserChan <- msg
		client.BrowserTimeout = time.NewTimer(browserHeartbeatTimeout)
	}
	return true
}

func startSession(client *Client, input <-chan Message) bool {
	client.Rank = Unknown
	client.UpdateVideo(&VideoInstant{DefaultVideo, Unstarted, 0.0})
	for running := true; running; {
		select {
		case <-client.BrowserTimeout.C:
			log.Fatal("Timed out waiting for Heartbeat from browser. Disconnecting.")
			return false
		case browserChan := <-client.BrowserConnect:
			msg, err := NewBrowserMessage(T_BrowserConnectAck, BrowserConnectAck{
				false, "A browser is already connected to this client.", client.Id})
			if err == nil {
				browserChan <- msg
			}
		case message := <-input:
			switch message.MsgType() {
			case T_Heartbeat:
				handleHeartbeat(client, message)
			case T_SessionInit:
				var payload SessionInit
				err := message.ReadValue(client, &payload, false)
				if err == nil {
					client.Name = payload.Name
					if payload.Leader {
						client.BecomeLeader()
					}
					err := message.Respond(client, T_SessionOk, SessionOk{})
					if err != nil {
						log.Printf("Failed to respond to SessionInit from browser: %v", err)
					}
					if payload.Leader {
						client.TrySendToBrowser(T_VideoUpdate, client.GetVideo())
					}
					running = false
				} else {
					respondWithError(client, message, err.Error())
				}
			case T_Error:
				handleError(client, message)
			default:
				respondWithError(client, message, fmt.Sprintf(
					"Did not expect message of type %s from browser.", message.MsgType()))
			}
		}
	}
	ticker := time.NewTicker(peerHeartbeatInterval)
	go func() {
		for {
			<-ticker.C
			if client.GetLeader() != nil {
				message, err := NewPeerMessage(client, T_Heartbeat, Heartbeat{rand.Int31()})
				if err == nil {
					client.GetLeader().OutChannel <- message
				}
			}
		}
	}()
	return true
}

func waitForInvitation(client *Client, input <-chan Message) bool {
	invitationReceived := false
	var invitation Invitation
	for client.Rank == Unknown {
		select {
		case <-client.BrowserTimeout.C:
			log.Fatal("Timed out waiting for Heartbeat from browser. Disconnecting.")
			return false
		case browserChan := <-client.BrowserConnect:
			msg, err := NewBrowserMessage(T_BrowserConnectAck, BrowserConnectAck{
				false, "A browser is already connected to this client.", client.Id})
			if err == nil {
				browserChan <- msg
			}
		case message := <-input:
			switch message.MsgType() {
			case T_Heartbeat:
				handleHeartbeat(client, message)
			case T_Invitation:
				err := message.ReadValue(client, &invitation, false)
				if err != nil {
					respondWithError(client, message, err.Error())
					continue
				}
				invitationReceived = true
				if peerId, ip, fromBrowser := message.Sender(); !fromBrowser {
					log.Printf("Got invitation from leader at %s:%d.", ip, invitation.Leader.Port)
					invitation.Leader.Id = peerId
					invitation.Leader.Address = ip
					client.TrySendToBrowser(T_Invitation, invitation)
				} else {
					respondWithError(client, message, "Cannot accept Invitation from browser.")
				}
			case T_InvitationResponse:
				if !invitationReceived {
					log.Println("Got InvitationResponse without Invitation.")
					continue
				}
				var payload InvitationResponse
				err := message.ReadValue(client, &payload, false)
				if err != nil {
					respondWithError(client, message, err.Error())
					continue
				}
				if _, _, fromBrowser := message.Sender(); fromBrowser {
					leader, err := client.establishPeerConnection(invitation.Leader)
					if err != nil {
						log.Printf("Failed to connect to inviter: %s", err)
						invitationReceived = false
						continue
					}
					payload.PublicKey = &SerializedPublicKey{
						N: client.PrivateKey.PublicKey.N.Bytes(),
						E: client.PrivateKey.PublicKey.E,
					}
					peerMessage, err := NewPeerMessage(client, T_InvitationResponse, payload)
					if err != nil {
						log.Printf("Failed to send InvitationResponse: %s", err)
						invitationReceived = false
						continue
					}
					leader.OutChannel <- peerMessage
					if payload.Accepted {
						client.SetLeader(leader)
					} else {
						invitationReceived = false
						leader.CloseChannel <- true
					}
				} else {
					respondWithError(client, message,
						"Cannot accept InvitationResponse from non-browser.")
				}
			case T_JoinConfirmation:
				if !invitationReceived || client.GetLeader() == nil {
					log.Println("Got JoinConfirmation without Invitation.")
					continue
				}
				var payload JoinConfirmation
				err := message.ReadValue(client, &payload, true)
				if err != nil {
					respondWithError(client, message, err.Error())
					continue
				}
				if peerId, _, fromBrowser := message.Sender(); !fromBrowser && peerId == client.GetLeader().Id {
					bmessage, err := NewBrowserMessage(T_JoinConfirmation, payload)
					if err == nil {
						client.ToBrowser <- bmessage
					} else {
						log.Printf("BrowserMessage encode failed: %s", err)
					}
					if payload.Success {
						client.Rank = invitation.RankOffered
						client.UpdateRoster(payload.Roster)
					} else {
						log.Fatalf("Join refused: %s", payload.Reason)
						time.Sleep(2000) // Leave time to send the message to the browser...
						return false
					}
				} else {
					respondWithError(client, message,
						"Cannot accept JoinConfirmation from non-leader.")
				}
			case T_Error:
				handleError(client, message)
			default:
				respondWithError(client, message, fmt.Sprintf(
					"Did not expect message of type %s.", message.MsgType()))
			}
		}
	}
	return true
}

func eventLoop(client *Client, input <-chan Message) bool {
	client.LeaderTimeout = time.NewTimer(peerHeartbeatTimeout)
	for {
		select {
		case <-client.BrowserTimeout.C:
			log.Fatal("Timed out waiting for Heartbeat from browser. Disconnecting.")
			return false
		case <-client.LeaderTimeout.C:
			if !client.IsLeader() {
				log.Println("Timed out waiting for HeartbeatAck from leader.")
				client.SetLeader(nil)
				client.ElectNewLeader()
			}
		case browserChan := <-client.BrowserConnect:
			msg, err := NewBrowserMessage(T_BrowserConnectAck, BrowserConnectAck{
				false, "A browser is already connected to this client.", client.Id})
			if err == nil {
				browserChan <- msg
			}
		case message := <-input:
			switch message.MsgType() {
			case T_Heartbeat:
				handleHeartbeat(client, message)
			case T_HeartbeatAck:
				handleHeartbeatAck(client, message)
			case T_InvitationResponse:
				handleInvitationResponse(client, message)
			case T_VideoUpdateRequest:
				handleVideoUpdateRequest(client, message)
			case T_RankChangeRequest:
				handleRankChangeRequest(client, message)
			case T_InvitationRequest:
				handleInvitationRequest(client, message)
			case T_RosterUpdate:
				handleRosterUpdate(client, message)
			case T_VideoUpdate:
				handleVideoUpdate(client, message)
			case T_LeaderVote:
				handleLeaderVote(client, message)
			case T_Error:
				handleError(client, message)
			case T_EndSession:
				var payload EndSession
				err := message.ReadValue(client, &payload, true)
				if err == nil {
					peerId, _, fromBrowser := message.Sender()
					if fromBrowser && client.IsLeader() {
						client.BroadcastToPeers(T_EndSession, payload)
						time.Sleep(5000) // Leave time to send the messages...
						return true
					} else if !fromBrowser && client.GetLeader() != nil && peerId == client.GetLeader().Id {
						return true
					} else {
						respondWithError(client, message, "Not authorized to end the session.")
					}
				} else {
					respondWithError(client, message, err.Error())
				}
			default:
				respondWithError(client, message, fmt.Sprintf(
					"Did not expect message of type %s.", message.MsgType()))
			}
		}
	}
}

func respondWithError(client *Client, message Message, err string) {
	err2 := message.Respond(client, T_Error, Error{err})
	if err2 != nil {
		log.Printf("Failed to send error response '%v': %v", err, err2)
	}
}

func handleHeartbeat(client *Client, message Message) {
	var payload Heartbeat
	err := message.ReadValue(client, &payload, true)
	if err != nil {
		respondWithError(client, message, err.Error())
		return
	}
	peerId, _, isBrowser := message.Sender()
	if isBrowser {
		client.BrowserTimeout.Reset(browserHeartbeatTimeout)
		err := message.Respond(client, T_HeartbeatAck, HeartbeatAck{payload.Random})
		if err != nil {
			log.Printf("Failed to respond to Heartbeat from browser: %v", err)
		}
	} else if client.IsLeader() {
		if peer := client.GetPeer(peerId); peer != nil {
			if peer.Timeout == nil {
				peer.Timeout = time.AfterFunc(peerHeartbeatTimeout, func() {
					log.Printf("Timed out waiting for Heartbeat from peer at %s.", peer.Address)
					peer.CloseChannel <- true
				})
			} else {
				peer.Timeout.Reset(peerHeartbeatTimeout)
			}
			client.TrySendToPeer(peer, T_HeartbeatAck, HeartbeatAck{payload.Random})
		}
	}
}

func handleHeartbeatAck(client *Client, message Message) {
	var payload HeartbeatAck
	err := message.ReadValue(client, &payload, true)
	if err != nil {
		respondWithError(client, message, err.Error())
		return
	}
	peerId, _, fromBrowser := message.Sender()
	if leader := client.GetLeader(); !fromBrowser && leader != nil && peerId == leader.Id {
		client.LeaderTimeout.Reset(peerHeartbeatTimeout)
	}
}

func handleInvitationResponse(client *Client, message Message) {
	if !client.IsLeader() {
		respondWithError(client, message, "Only leader can handle InvitationResponse.")
		return
	}

	var payload InvitationResponse
	err := message.ReadValue(client, &payload, false)
	if err != nil {
		respondWithError(client, message, err.Error())
		return
	}
	peerId, address, fromBrowser := message.Sender()
	if fromBrowser {
		respondWithError(client, message, "Cannot accept InvitationResponse from browser.")
		return
	}

	if oi, ok := client.OutstandingInvitations[payload.Random]; ok {
		delete(client.OutstandingInvitations, payload.Random)
		if !payload.Accepted {
			log.Printf("Peer at %s rejected invitation.", address)
			oi.Peer.CloseChannel <- true
			return
		}
		if collision := client.GetPeer(payload.Id); collision != nil {
			client.TrySendToPeer(oi.Peer, T_JoinConfirmation, JoinConfirmation{
				Success: false,
				Reason:  "Peer ID collision.",
			})
			oi.Peer.CloseChannel <- true
			return
		}
		if peerId != payload.Id {
			log.Printf("Peer at %s provided wrong ID (expected %d, got %d).", address,
				peerId, payload.Id)
			oi.Peer.CloseChannel <- true
			return
		}
		if payload.PublicKey == nil {
			log.Printf("Peer at %s did not provide a public key.", address)
			oi.Peer.CloseChannel <- true
			return
		}
		oi.Peer.Id = payload.Id
		oi.Peer.Name = payload.Name
		oi.Peer.PublicKey = &rsa.PublicKey{
			N: &big.Int{},
			E: payload.PublicKey.E,
		}
		oi.Peer.PublicKey.N.SetBytes(payload.PublicKey.N)
		client.AddPeer(oi.Peer)

		client.TrySendToPeer(oi.Peer, T_JoinConfirmation, JoinConfirmation{
			Success: true,
			Roster:  client.GetRoster(),
		})

		client.BroadcastToPeers(T_RosterUpdate, RosterUpdate{client.GetRoster()})
	} else {
		log.Printf("Ignored InvitationResponse from %s; no outstanding invitation with ID %d.",
			address, payload.Random)
	}
}

func handleVideoUpdateRequest(client *Client, message Message) {
	var payload VideoUpdateRequest
	err := message.ReadValue(client, &payload, true)
	if err != nil {
		respondWithError(client, message, err.Error())
		return
	}
	peerId, _, fromBrowser := message.Sender()
	if fromBrowser {
		if client.IsLeader() {
			go handleVideoUpdateRequestAsLeader(client, payload, true)
		} else if leader := client.GetLeader(); leader != nil {
			client.TrySendToPeer(leader, T_VideoUpdateRequest, payload)
		} else {
			log.Println("Ignored VideoUpdateRequest from browser: no leader.")
		}
	} else {
		if client.IsLeader() {
			if peer := client.GetPeer(peerId); peer != nil {
				if peer.Rank == Editor || peer.Rank == Director {
					go handleVideoUpdateRequestAsLeader(client, payload, false)
				} else {
					log.Println("Ignored VideoUpdateRequest: peer not authorized.")
				}
			} else {
				log.Println("Ignored VideoUpdateRequest: peer not recognized.")
			}
		} else {
			log.Println("Ignored VideoUpdateRequest: only leader can respond to this.")
		}
	}
}

func handleVideoUpdateRequestAsLeader(client *Client, request VideoUpdateRequest, fromBrowser bool) {
	vi := VideoInstant(request)
	client.UpdateVideo(&vi)
	if !fromBrowser {
		client.TrySendToBrowser(T_VideoUpdate, request)
	}
	client.BroadcastToPeers(T_VideoUpdate, request)
}

func handleRankChangeRequest(client *Client, message Message) {
	var payload RankChangeRequest
	err := message.ReadValue(client, &payload, true)
	if err != nil {
		respondWithError(client, message, err.Error())
		return
	}
	peerId, _, fromBrowser := message.Sender()
	if fromBrowser {
		if client.IsLeader() {
			go handleRankChangeRequestAsLeader(client, payload)
		} else if leader := client.GetLeader(); leader != nil {
			client.TrySendToPeer(leader, T_RankChangeRequest, payload)
		} else {
			log.Println("Ignored RankChangeRequest from browser: no leader.")
		}
	} else {
		if client.IsLeader() {
			if peer := client.GetPeer(peerId); peer != nil {
				if peer.Rank == Director {
					go handleRankChangeRequestAsLeader(client, payload)
				} else {
					log.Println("Ignored RankChangeRequest: peer not authorized.")
				}
			} else {
				log.Println("Ignored RankChangeRequest: peer not recognized.")
			}
		} else {
			log.Println("Ignored RankChangeRequest: only leader can respond to this.")
		}
	}
}

func handleRankChangeRequestAsLeader(client *Client, request RankChangeRequest) {
	if request.PeerId == client.Id {
		log.Println("Ignored RankChangeRequest: cannot change rank of leader.")
		return
	} else {
		if peer := client.GetPeer(request.PeerId); peer != nil {
			rosterEntry := peer.ToRosterEntry()
			rosterEntry.Rank = request.NewRank
			client.UpdatePeer(rosterEntry)
			log.Printf("Updated rank of peer '%s' to %d.", peer.Name, request.NewRank)
		} else {
			log.Printf("Ignored RankChangeRequest: no peer with id %d.", request.PeerId)
			return
		}
	}
}

func handleInvitationRequest(client *Client, message Message) {
	var payload InvitationRequest
	err := message.ReadValue(client, &payload, true)
	if err != nil {
		respondWithError(client, message, err.Error())
		return
	}
	peerId, _, fromBrowser := message.Sender()
	if fromBrowser {
		log.Println("Got InvitationRequest from browser.")
		if client.IsLeader() {
			go handleInvitationRequestAsLeader(client, payload)
		} else if leader := client.GetLeader(); leader != nil {
			client.TrySendToPeer(leader, T_InvitationRequest, payload)
		} else {
			log.Println("Ignored InvitationRequest from browser: no leader.")
		}
	} else {
		if client.IsLeader() {
			if peer := client.GetPeer(peerId); peer != nil {
				if peer.Rank == Director {
					go handleInvitationRequestAsLeader(client, payload)
				} else {
					log.Println("Ignored InvitationRequest: peer not authorized.")
				}
			} else {
				log.Println("Ignored InvitationRequest: peer not recognized.")
			}
		} else {
			log.Println("Ignored InvitationRequest: only leader can respond to this.")
		}
	}
}

func handleInvitationRequestAsLeader(client *Client, request InvitationRequest) {
	if _, ok := client.OutstandingInvitations[request.Invitation.Random]; ok {
		log.Printf("Cannot invite new peer at %s; ID collision.", request.Address)
		return
	}
	request.Invitation.Leader = client.ToRosterEntry()

	go func() {
		newPeer, err := client.establishPeerConnection(&RosterEntry{
			Id:      -91,
			Name:    "Invited Peer",
			Rank:    request.Invitation.RankOffered,
			Address: request.Address,
			Port:    request.Port,
		})
		if err != nil {
			log.Printf("Failed to connect to invited peer at %s: %s", request.Address, err)
			return
		}
		client.OutstandingInvitations[request.Invitation.Random] = &OutstandingInvitation{
			Peer:       newPeer,
			Invitation: &request.Invitation,
		}
		client.TrySendToPeer(newPeer, T_Invitation, request.Invitation)
	}()
}

func handleRosterUpdate(client *Client, message Message) {
	var payload RosterUpdate
	err := message.ReadValue(client, &payload, true)
	if err != nil {
		respondWithError(client, message, err.Error())
		return
	}
	peerId, _, fromBrowser := message.Sender()
	if !fromBrowser && client.GetLeader() != nil && peerId == client.GetLeader().Id {
		client.UpdateRoster(payload.Roster)
	} else {
		log.Println("RosterUpdate ignored: unauthorized source.")
	}
}

func handleVideoUpdate(client *Client, message Message) {
	var payload VideoUpdate
	err := message.ReadValue(client, &payload, true)
	if err != nil {
		respondWithError(client, message, err.Error())
		return
	}
	peerId, _, fromBrowser := message.Sender()
	if !fromBrowser && client.GetLeader() != nil && peerId == client.GetLeader().Id {
		vi := VideoInstant(payload)
		client.UpdateVideo(&vi)
		client.TrySendToBrowser(T_VideoUpdate, payload)
	} else {
		log.Println("VideoUpdate ignored: unauthorized source.")
	}
}

func handleLeaderVote(client *Client, message Message) {
	var payload LeaderVote
	err := message.ReadValue(client, &payload, true)
	if err != nil {
		respondWithError(client, message, err.Error())
		return
	}
	peerId, _, fromBrowser := message.Sender()
	if !fromBrowser {
		payload.Sender = peerId
		client.AcceptLeaderVote(&payload)
	}
}

func handleError(client *Client, message Message) {
	_, source, fromBrowser := message.Sender()
	if fromBrowser {
		source = "browser"
	}
	var payload Error
	err := message.ReadValue(client, &payload, false)
	if err == nil {
		log.Printf("Remote error returned from %s: %s", source, payload.Message)
	} else {
		log.Printf("Error parsing error from %s: %s", source, err)
	}
}
