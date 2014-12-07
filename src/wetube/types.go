package wetube

import (
	"crypto/rsa"
	"net"
	"time"
)

type Rank byte

const (
	// Ranks
	Unknown Rank = iota
	Director
	Editor
	Viewer
)

type Peer struct {
	Id         int
	Name       string
	Address    string
	Rank       Rank
	PublicKey  *rsa.PublicKey
	OutChannel chan<- PeerMessage
	conn       *net.Conn
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
	Id             int
	Name           string
	Rank           Rank
	PublicKey      *rsa.PublicKey
	PrivateKey     *rsa.PrivateKey
	Video          Video
	Leader         *Peer
	Peers          *map[int]*Peer
	BrowserConnect chan chan *BrowserMessage
	ToBrowser      chan<- *BrowserMessage
	BrowserTimeout *time.Timer
}
