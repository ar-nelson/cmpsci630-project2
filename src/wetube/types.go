package wetube

import (
	"crypto/rsa"
	"sync"
	"time"
)

type Rank byte

const (
	// Ranks
	Unknown Rank = iota
	Viewer
	Editor
	Director
)

type Peer struct {
	Id           int
	Name         string
	Address      string
	Rank         Rank
	PublicKey    *rsa.PublicKey
	CloseChannel chan<- bool
	OutChannel   chan<- *PeerMessage
	Timeout      *time.Timer
}

type VideoState byte

const (
	// Video States
	Stop VideoState = 1 + iota
	Play
	Pause
)

type Video struct {
	Id       string
	State    VideoState
	Position uint
}

type Message interface {
	MsgType() MsgType
	Sender() (peerId int, ip string, isBrowser bool)
	Respond(client *Client, messageType MsgType, message interface{}) error
	ReadValue(client *Client, into interface{}) error
}

type Client struct {
	Id              int
	Name            string
	Rank            Rank
	PrivateKey      *rsa.PrivateKey
	VideoMutex      sync.RWMutex
	Video           Video
	IsLeader        bool
	PeersMutex      sync.RWMutex
	Leader          *Peer
	Peers           *map[int]*Peer
	BrowserConnect  chan chan *BrowserMessage
	ToBrowser       chan<- *BrowserMessage
	BrowserTimeout  *time.Timer
	HeartbeatTicker *time.Ticker
}
