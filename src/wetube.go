package main

import (
	crypto_rand "crypto/rand"
	"crypto/rsa"
	"log"
	"math/rand"
	"time"
	"wetube"
)

func main() {
	rand.Seed(time.Now().Unix())
	client := &wetube.Client{
		Id:             rand.Int31(),
		Port:           9191,
		ServeHtml:      true,
		BrowserConnect: make(chan chan *wetube.BrowserMessage, 1),
	}
	log.Printf("Your client ID is %d.", client.Id)
	privateKey, err := rsa.GenerateKey(crypto_rand.Reader, 1024)
	if err != nil {
		log.Fatalf("generating rsa keys: %s", err)
		return
	}
	client.PrivateKey = privateKey
	recvChannel := make(chan wetube.Message, 91)
	client.Run(recvChannel)
}
