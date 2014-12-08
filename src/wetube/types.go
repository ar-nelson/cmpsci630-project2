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
	Id           int32
	Name         string
	Address      string
	Port         uint16
	Rank         Rank
	PublicKey    *rsa.PublicKey
	CloseChannel chan<- bool
	OutChannel   chan<- *PeerMessage
	Timeout      *time.Timer
}

type VideoState byte

const (
	// Video States
	Unstarted VideoState = iota
	Playing
	Paused
	Ended
)

type VideoInstant struct {
	Id             string
	State          VideoState
	SecondsElapsed float64
}

type Video struct {
	VideoInstant
	LastUpdate time.Time
}

func (video *Video) ToInstant() VideoInstant {
	adjustedSeconds := video.SecondsElapsed
	if video.State == Playing {
		adjustedSeconds += time.Now().Sub(video.LastUpdate).Seconds()
	}
	return VideoInstant{
		Id:             video.Id,
		State:          video.State,
		SecondsElapsed: adjustedSeconds,
	}
}

type Message interface {
	MsgType() MsgType
	Sender() (peerId int32, ip string, isBrowser bool)
	Respond(client *Client, messageType MsgType, message interface{}) error
	ReadValue(client *Client, into interface{}, secure bool) error
}

type OutstandingInvitation struct {
	Peer       *Peer
	Invitation *Invitation
}

type Client struct {
	Id                     int32
	Port                   uint16
	ServeHtml              bool
	Name                   string
	Rank                   Rank
	PrivateKey             *rsa.PrivateKey
	VideoMutex             sync.RWMutex
	Video                  Video
	IsLeader               bool
	PeersMutex             sync.RWMutex
	Leader                 *Peer
	Peers                  *map[int32]*Peer
	BrowserConnect         chan chan *BrowserMessage
	ToBrowser              chan<- *BrowserMessage
	BrowserTimeout         *time.Timer
	HeartbeatTicker        *time.Ticker
	OutstandingInvitations map[int32]*OutstandingInvitation
}
