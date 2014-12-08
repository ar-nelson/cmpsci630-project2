package wetube

import (
	"fmt"
	"log"
	"math"
	"math/rand"
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

func ClientLoop(client *Client, input chan Message) bool {
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
	newMap := make(map[int]*Peer)
	client.Peers = &newMap
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
				err := message.ReadValue(client, &payload)
				if err == nil {
					client.Name = payload.Name
					if payload.Leader {
						client.IsLeader = true
						client.Rank = Director
					}
					err := message.Respond(client, T_SessionOk, SessionOk{})
					if err != nil {
						log.Printf("Failed to respond to SessionInit from browser: %v", err)
					}
					if payload.Leader {
						browserMessage, err := NewBrowserMessage(T_VideoUpdate, client.Video)
						if err == nil {
							client.ToBrowser <- browserMessage
						} else {
							log.Printf("Failed to send VideoUpdate to browser: %v", err)
						}
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
			client.PeersMutex.RLock()
			if client.Leader != nil {
				message, err := NewPeerMessage(client, T_Heartbeat, Heartbeat{rand.Int()})
				if err == nil {
					client.Leader.OutChannel <- message
				}
			}
			client.PeersMutex.RUnlock()
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
				err := message.ReadValue(client, &invitation)
				if err == nil {
					invitationReceived = true
					if peerId, ip, fromBrowser := message.Sender(); !fromBrowser {
						invitation.Leader.Id = peerId
						invitation.Leader.Address = ip
					} else {
						respondWithError(client, message, "Cannot accept Invitation from browser.")
					}
				} else {
					respondWithError(client, message, err.Error())
				}
			case T_InvitationResponse:
				if invitationReceived {
					var payload InvitationResponse
					err := message.ReadValue(client, &payload)
					if err == nil {
						if _, _, fromBrowser := message.Sender(); fromBrowser {
							leader, err := client.establishPeerConnection(&invitation.Leader)
							if err == nil {
								peerMessage, err := NewPeerMessage(client, T_InvitationResponse, payload)
								if err == nil {
									leader.OutChannel <- peerMessage
									if payload.Accepted {
										client.Rank = invitation.RankOffered
										client.SetLeader(leader)
									} else {
										invitationReceived = false
										leader.CloseChannel <- true
									}
								} else {
									log.Printf("Failed to send InvitationResponse: %s", err)
									invitationReceived = false
								}
							} else {
								log.Printf("Failed to connect to inviter: %s", err)
								invitationReceived = false
							}
						} else {
							respondWithError(client, message,
								"Cannot accept InvitationResponse from non-browser.")
						}
					} else {
						respondWithError(client, message, err.Error())
					}
				} else {
					respondWithError(client, message, "Got InvitationResponse without Invitation.")
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
	for {
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
			case T_JoinConfirmation:
				handleJoinConfirmation(client, message)
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
			case T_Error:
				handleError(client, message)
			case T_EndSession:
				var payload EndSession
				err := message.ReadValue(client, &payload)
				if err == nil {
					peerId, _, fromBrowser := message.Sender()
					client.PeersMutex.RLock()
					if fromBrowser && client.IsLeader {
						client.BroadcastToPeers(T_EndSession, payload)
						time.Sleep(5000) // Leave time to send the messages...
						return true
					} else if !fromBrowser && client.Leader != nil && peerId == client.Leader.Id {
						client.PeersMutex.RUnlock()
						return true
					} else {
						respondWithError(client, message, "Not authorized to end the session.")
					}
					client.PeersMutex.RUnlock()
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
		log.Println("Failed to send error response: %v", err2)
	}
}

func handleJoinConfirmation(client *Client, message Message) {

}

func handleHeartbeat(client *Client, message Message) {
	var payload Heartbeat
	err := message.ReadValue(client, &payload)
	if err == nil {
		_, _, isBrowser := message.Sender()
		if isBrowser {
			client.BrowserTimeout.Reset(browserHeartbeatTimeout)
			err := message.Respond(client, T_HeartbeatAck, HeartbeatAck{payload.Random})
			if err != nil {
				log.Printf("Failed to respond to Heartbeat from browser: %v", err)
			}
		} else {
			// TODO: Handle Heartbeat messages from peers.
		}
	} else {
		respondWithError(client, message, err.Error())
	}
}

func handleHeartbeatAck(client *Client, message Message) {
	// TODO: Keep track of HeartbeatAcks from the leader.
}

func handleVideoUpdateRequest(client *Client, message Message) {
	var payload VideoUpdateRequest
	err := message.ReadValue(client, &payload)
	if err != nil {
		respondWithError(client, message, err.Error())
		return
	}
	peerId, _, fromBrowser := message.Sender()
	client.PeersMutex.RLock()
	if fromBrowser {
		if client.IsLeader {
			go handleVideoUpdateRequestAsLeader(client, payload, true)
		} else if client.Leader != nil {
			peerMessage, err := NewPeerMessage(client, T_VideoUpdateRequest, payload)
			if err == nil {
				client.Leader.OutChannel <- peerMessage
			} else {
				log.Printf("PeerMessage encode failed: %s", err)
			}
		} else {
			log.Println("Ignored VideoUpdateRequest from browser: no leader.")
		}
	} else {
		if client.IsLeader {
			if peer, found := (*client.Peers)[peerId]; found {
				if peer.Rank == Editor || peer.Rank == Director {
					go handleVideoUpdateRequestAsLeader(client, payload, false)
				} else {
					log.Println("Ignored VideoUpdateRequet: peer not authorized.")
				}
			} else {
				log.Println("Ignored VideoUpdateRequest: peer not recognized.")
			}
		} else {
			log.Println("Ignored VideoUpdateRequest: only leader can respond to this.")
		}
	}
	client.PeersMutex.RUnlock()
}

func handleVideoUpdateRequestAsLeader(client *Client, request VideoUpdateRequest, fromBrowser bool) {
	client.SetVideo(VideoInstant(request), !fromBrowser)
	err := client.BroadcastToPeers(T_VideoUpdate, request)
	if err != nil {
		log.Printf("video update broadcast: %s", err)
	}
}

func handleRankChangeRequest(client *Client, message Message) {
	var payload RankChangeRequest
	err := message.ReadValue(client, &payload)
	if err != nil {
		respondWithError(client, message, err.Error())
		return
	}
	peerId, _, fromBrowser := message.Sender()
	client.PeersMutex.RLock()
	if fromBrowser {
		if client.IsLeader {
			go handleRankChangeRequestAsLeader(client, payload)
		} else if client.Leader != nil {
			peerMessage, err := NewPeerMessage(client, T_RankChangeRequest, payload)
			if err == nil {
				client.Leader.OutChannel <- peerMessage
			} else {
				log.Printf("PeerMessage encode failed: %s", err)
			}
		} else {
			log.Println("Ignored RankChangeRequest from browser: no leader.")
		}
	} else {
		if client.IsLeader {
			if peer, found := (*client.Peers)[peerId]; found {
				if peer.Rank == Director {
					go handleRankChangeRequestAsLeader(client, payload)
				} else {
					log.Println("Ignored RankChangeRequet: peer not authorized.")
				}
			} else {
				log.Println("Ignored RankChangeRequest: peer not recognized.")
			}
		} else {
			log.Println("Ignored RankChangeRequest: only leader can respond to this.")
		}
	}
	client.PeersMutex.RUnlock()
}

func handleRankChangeRequestAsLeader(client *Client, request RankChangeRequest) {
	client.PeersMutex.Lock()
	if request.PeerId == client.Id {
		log.Println("Ignored RankChangeRequest: cannot change rank of leader.")
	} else {
		// TODO: Something...
	}
	client.PeersMutex.Unlock()
	err := client.BroadcastToPeers(T_RosterUpdate, RosterUpdate{client.Roster()})
	if err != nil {
		log.Printf("roster update broadcast: %s", err)
	}
}

func handleInvitationRequest(client *Client, message Message) {

}

func handleRosterUpdate(client *Client, message Message) {

}

func handleVideoUpdate(client *Client, message Message) {
	var payload VideoUpdate
	err := message.ReadValue(client, &payload)
	if err != nil {
		respondWithError(client, message, err.Error())
		return
	}
	peerId, _, fromBrowser := message.Sender()
	client.PeersMutex.RLock()
	if !fromBrowser && client.Leader != nil && peerId == client.Leader.Id {
		client.SetVideo(VideoInstant(payload), true)
	} else {
		log.Println("VideoUpdate ignored: unauthorized source.")
	}
	client.PeersMutex.RUnlock()
}

func (client *Client) SetVideo(video VideoInstant, updateBrowser bool) bool {
	client.VideoMutex.Lock()
	defer client.VideoMutex.Unlock()
	oldVideo := client.Video.ToInstant()
	// If they're the same video with less than 2 seconds of difference, don't bother updating.
	if video.Id != oldVideo.Id || video.State != oldVideo.State ||
		math.Abs(video.SecondsElapsed-oldVideo.SecondsElapsed) >= 2 {
		// ...otherwise, update the video information.
		client.Video = Video{video, time.Now()}
		log.Printf("Video state update: %v, state %d, at %f seconds.", video.Id, video.State,
			video.SecondsElapsed)
		if updateBrowser {
			message, err := NewBrowserMessage(T_VideoUpdate, video)
			if err == nil {
				client.ToBrowser <- message
			} else {
				log.Printf("BrowserMessage encode failed: %s", err)
			}
		}
		return true
	} else {
		return false
	}
}

func (client *Client) GetVideo() VideoInstant {
	client.VideoMutex.RLock()
	video := client.Video.ToInstant()
	client.VideoMutex.RUnlock()
	return video
}

func handleError(client *Client, message Message) {
	_, source, fromBrowser := message.Sender()
	if fromBrowser {
		source = "browser"
	}
	var payload Error
	err := message.ReadValue(client, &payload)
	if err == nil {
		log.Printf("Remote error returned from %s: %s", source, payload.Message)
	} else {
		log.Printf("Error parsing error from %s: %s", source, err)
	}
}
