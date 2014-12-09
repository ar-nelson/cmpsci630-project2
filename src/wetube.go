package main

import (
	"log"
	"math/rand"
	"time"
	"wetube"
)

func main() {
	rand.Seed(time.Now().Unix())
	client := wetube.NewClient(rand.Int31(), 9191)
	log.Printf("Your client ID is %d.", client.Id)
	recvChannel := make(chan wetube.Message, 91)
	client.Run(recvChannel, true)
}
