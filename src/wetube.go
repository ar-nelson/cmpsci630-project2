package main

import (
	crypto_rand "crypto/rand"
	"crypto/rsa"
	"encoding/gob"
	"github.com/gorilla/websocket"
	"log"
	"math/rand"
	"net/http"
	"wetube"
)

func main() {
	client := &wetube.Client{
		Id:             rand.Int(),
		BrowserConnect: make(chan chan *wetube.BrowserMessage, 1),
	}
	privateKey, err := rsa.GenerateKey(crypto_rand.Reader, 1024)
	if err != nil {
		log.Fatalf("generating rsa keys: %s", err)
		return
	}
	client.PrivateKey = privateKey
	log.Println("Starting WeTube HTTP server on localhost:9191...")
	recvChannel := make(chan wetube.Message, 91)
	go func() {
		http.Handle("/", http.FileServer(http.Dir("./web/")))
		http.HandleFunc("/browserSocket", browserSocketHandler(client, recvChannel))
		http.HandleFunc("/peerSocket", peerSocketHandler(client, recvChannel))
		err := http.ListenAndServeTLS(":9191", wetube.PublicKeyFile, wetube.PrivateKeyFile, nil)
		if err != nil {
			log.Fatalf("wetube server: %s", err)
		}
	}()
	wetube.ClientLoop(client, recvChannel)
}

func browserSocketHandler(client *wetube.Client, out chan<- wetube.Message) func(w http.ResponseWriter, request *http.Request) {
	return func(w http.ResponseWriter, request *http.Request) {
		var upgrader websocket.Upgrader
		conn, err := upgrader.Upgrade(w, request, nil)
		if err != nil {
			log.Println(err)
			return
		}
		log.Printf("Connected to browser at %s.", conn.RemoteAddr().String())

		// Connect to client ------------------------------

		in := make(chan *wetube.BrowserMessage, 91)
		client.BrowserConnect <- in
		for message := range in {
			if message.Type == wetube.T_BrowserConnectAck {
				var payload wetube.BrowserConnectAck
				err = message.ReadValue(client, &payload)
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
				var nextMessage wetube.BrowserMessage
				err := conn.ReadJSON(&nextMessage)
				if err == nil {
					out <- &nextMessage
				} else {
					if err.Error() == "EOF" {
						log.Println("EOF on browser connection; closing.")
						close(in)
						running = false
					} else {
						log.Printf("browserSocket recv: %s", err)
					}
				}
			}
		}()

		// Outgoing messages ------------------------------

		for message := range in {
			if message.MsgType() == wetube.T_BrowserDisconnect {
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

func peerSocketHandler(client *wetube.Client, out chan<- wetube.Message) func(w http.ResponseWriter, request *http.Request) {
	return func(w http.ResponseWriter, request *http.Request) {
		var upgrader websocket.Upgrader
		conn, err := upgrader.Upgrade(w, request, nil)
		if err != nil {
			log.Println(err)
			return
		}
		address := conn.RemoteAddr().String()
		log.Printf("Opening input socket for peer at %s.", address)

		// Incoming messages ------------------------------

		running := true
		for running {
			var nextMessage wetube.PeerMessage
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
				if err.Error() == "EOF" {
					log.Printf("EOF on peer socket %s; closing.", address)
					running = false
				} else {
					log.Printf("peerSocket recv: %s", err)
				}
			}
		}

		log.Printf("Closing input socket for peer at %s.", address)
		conn.Close()
	}
}
