package wetube

import (
	"encoding/gob"
	"github.com/gorilla/websocket"
	"log"
	"net"
	"net/http"
	"strings"
)

func browserSocketHandler(client *Client, out chan<- Message) func(w http.ResponseWriter, request *http.Request) {
	return func(w http.ResponseWriter, request *http.Request) {
		var upgrader websocket.Upgrader
		conn, err := upgrader.Upgrade(w, request, nil)
		if err != nil {
			log.Println(err)
			return
		}
		log.Printf("Connected to browser at %s.", conn.RemoteAddr().String())

		// Connect to client ------------------------------

		in := make(chan *BrowserMessage, 91)
		client.BrowserConnect <- in
		for message := range in {
			if message.Type == T_BrowserConnectAck {
				var payload BrowserConnectAck
				err = message.ReadValue(client, &payload, false)
				if err == nil {
					err = conn.WriteJSON(message)
					if err != nil {
						log.Printf("browserSocket send: %s", err)
					}
					if !payload.Success {
						log.Printf("Browser at %s rejected: %s",
							conn.RemoteAddr().String(), payload.Reason)
						close(in)
						defer conn.Close()
						return
					}
					break
				} else {
					log.Printf("browserSocket ack: %s", err)
				}
			} else {
				log.Printf("browserSocket send: %s rejected", message.Type)
			}
		}

		// Incoming messages ------------------------------

		running := true
		go func() {
			for running {
				var nextMessage BrowserMessage
				err := conn.ReadJSON(&nextMessage)
				if err == nil {
					out <- &nextMessage
				} else {
					if strings.Contains(strings.ToLower(err.Error()), "json") {
						log.Printf("browserSocket recv: %s", err)
					} else {
						log.Printf("Fatal error on browserSocket recv: %s", err)
						close(in)
						running = false
					}
				}
			}
		}()

		// Outgoing messages ------------------------------

		for message := range in {
			if message.MsgType() == T_BrowserDisconnect {
				log.Printf("Disconnected from browser at %s.", conn.RemoteAddr().String())
				running = false
				close(in)
			} else {
				err = conn.WriteJSON(message)
				if err != nil {
					log.Printf("browserSocket send: %s", err)
				}
			}
		}

		conn.Close()
	}
}

func peerSocketHandler(client *Client, out chan<- Message) func(w http.ResponseWriter, request *http.Request) {
	return func(w http.ResponseWriter, request *http.Request) {
		var upgrader websocket.Upgrader
		conn, err := upgrader.Upgrade(w, request, nil)
		if err != nil {
			log.Println(err)
			return
		}
		address := conn.RemoteAddr().(*net.TCPAddr).IP.String()
		log.Printf("Opening input socket for peer at %s.", address)

		// Incoming messages ------------------------------

		running := true
		for running {
			var nextMessage PeerMessage
			messageType, reader, err := conn.NextReader()
			if err == nil {
				if messageType == websocket.BinaryMessage {
					decoder := gob.NewDecoder(reader)
					err = decoder.Decode(&nextMessage)
					if err == nil {
						nextMessage.IpAddress = address
						out <- &nextMessage
					} else {
						log.Printf("peerSocket decode: %s", err)
					}
				} else {
					log.Printf("Ignoring message of type %d from peer socket %s.", messageType,
						address)
				}
			} else {
				log.Printf("peerSocket recv: %s", err)
				running = false
			}
		}

		log.Printf("Closing input socket for peer at %s.", address)
		conn.Close()
	}
}
