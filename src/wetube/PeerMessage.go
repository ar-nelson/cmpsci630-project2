package wetube

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/gob"
	"fmt"
)

type PeerMessage struct {
	SenderId  int
	IpAddress string
	Type      MsgType
	Payload   []byte
	Signature []byte
}

func NewPeerMessage(client *Client, messageType MsgType, contents interface{}) (*PeerMessage, error) {
	var buffer bytes.Buffer
	enc := gob.NewEncoder(&buffer)
	err := enc.Encode(contents)
	if err != nil {
		return nil, err
	}
	b := buffer.Bytes()
	hash := sha256.Sum256(b)
	sig, err := rsa.SignPKCS1v15(rand.Reader, client.PrivateKey, crypto.SHA256, hash[:])
	if err != nil {
		return nil, err
	}
	return &PeerMessage{client.Id, "0.0.0.0", messageType, b, sig}, nil
}

func (message *PeerMessage) MsgType() MsgType {
	return message.Type
}

func (message *PeerMessage) Sender() (int, string, bool) {
	return message.SenderId, message.IpAddress, false
}

func (message *PeerMessage) Respond(client *Client, messageType MsgType, contents interface{}) error {
	peer := (*client.Peers)[message.SenderId]
	if peer == nil {
		return fmt.Errorf("PeerMessage: No peer with id %d.", message.SenderId)
	}
	response, err := NewPeerMessage(client, messageType, contents)
	if err != nil {
		return err
	}
	peer.OutChannel <- *response
	return nil
}

func (message *PeerMessage) ReadValue(client *Client, into interface{}) error {
	peer := (*client.Peers)[message.SenderId]
	if peer == nil {
		return fmt.Errorf("PeerMessage: No peer with id %d.", message.SenderId)
	}
	err := rsa.VerifyPKCS1v15(peer.PublicKey, crypto.SHA256, message.Payload, message.Signature)
	if err != nil {
		return err
	}
	decoder := gob.NewDecoder(bytes.NewBuffer(message.Payload))
	return decoder.Decode(into)
}
