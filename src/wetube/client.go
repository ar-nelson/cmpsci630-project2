package wetube

import (
	"fmt"
	"log"
	"time"
)

const (
	browserConnectionTimeout time.Duration = 30 * time.Second
	browserHeartbeatTimeout                = 10 * time.Second

	PublicKeyFile  string = "certs/ssl_cert_public_key.rsa"
	PrivateKeyFile        = "certs/ssl_cert_private_key.rsa"

	DefaultPort  uint   = 9100
	DefaultVideo string = "M7lc1UVf-VE"
)

func ClientLoop(client *Client, input chan Message) bool {
	return connectToBrowser(client, input) &&
		startSession(client, input) //&&
	//waitForInvitation(client, input) &&
	//eventLoop(client, input)
}

func connectToBrowser(client *Client, input <-chan Message) bool {
	browserConnectionTimer := time.NewTimer(browserConnectionTimeout)
	select {
	case <-browserConnectionTimer.C:
		log.Fatal("Timed out waiting for browser to connect.")
		return false
	case browserChan := <-client.BrowserConnect:
		client.ToBrowser = browserChan
		msg, err := NewBrowserMessage(T_BrowserConnectAck, BrowserConnectAck{true, ""})
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
	for running := true; running; {
		select {
		case <-client.BrowserTimeout.C:
			log.Fatal("Timed out waiting for Heartbeat from browser. Disconnecting.")
			return false
		case browserChan := <-client.BrowserConnect:
			msg, err := NewBrowserMessage(T_BrowserConnectAck, BrowserConnectAck{
				false, "A browser is already connected to this client."})
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
						client.Rank = Director
					}
					err := message.Respond(client, T_SessionOk, SessionOk{})
					if err != nil {
						log.Printf("Failed to respond to SessionInit from browser: %v", err)
					}
					browserMessage, err := NewBrowserMessage(T_VideoUpdate, client.Video)
					if err == nil {
						client.ToBrowser <- browserMessage
					} else {
						log.Printf("Failed to send VideoUpdate to browser: %v", err)
					}
					running = false
				} else {
					respondWithError(client, message, err.Error())
				}
			default:
				respondWithError(client, message, fmt.Sprintf(
					"Did not expect message of type %s from browser.", message.MsgType()))
			}
		}
	}
	return true
}

func waitForInvitation(client *Client, input <-chan Message) bool {
	for client.Rank == Unknown {
		select {
		case <-client.BrowserTimeout.C:
			log.Fatal("Timed out waiting for Heartbeat from browser. Disconnecting.")
			return false
		case browserChan := <-client.BrowserConnect:
			msg, err := NewBrowserMessage(T_BrowserConnectAck, BrowserConnectAck{
				false, "A browser is already connected to this client."})
			if err == nil {
				browserChan <- msg
			}
		case message := <-input:
			switch message.MsgType() {
			case T_Heartbeat:
				handleHeartbeat(client, message)
			case T_Invitation:
				var payload Invitation
				err := message.ReadValue(client, &payload)
				if err == nil {
					//if peerId, ip, fromBrowser := message.Sender(); !fromBrowser {

					//} else {
					respondWithError(client, message, "Cannot accept Invitation from browser.")
					//}
				} else {
					respondWithError(client, message, err.Error())
				}
			default:
				respondWithError(client, message, fmt.Sprintf(
					"Did not expect message of type %s from browser.", message.MsgType()))
			}
		}
	}
	return true
}

func respondWithError(client *Client, message Message, err string) {
	err2 := message.Respond(client, T_Error, Error{err})
	if err2 != nil {
		log.Println("Failed to send error response: %v", err2)
	}
}

func handleInvitationResponse(client *Client, message Message) {

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

}

func handleVideoUpdateRequest(client *Client, message Message) {

}

func handleRankChangeRequest(client *Client, message Message) {

}

func handleInvitationRequest(client *Client, message Message) {

}

func handleRosterUpdate(client *Client, message Message) {

}

func handleVideoUpdate(client *Client, message Message) {

}

func handleEndSession(client *Client, message Message) {

}

func handleError(client *Client, message Message) {

}
