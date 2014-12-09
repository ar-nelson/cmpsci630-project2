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
	SenderId  int32
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
	return &PeerMessage{client.Id, "", messageType, b, sig}, nil
}

func (message *PeerMessage) MsgType() MsgType {
	return message.Type
}

func (message *PeerMessage) Sender() (int32, string, bool) {
	return message.SenderId, message.IpAddress, false
}

func (message *PeerMessage) Respond(client *Client, messageType MsgType, contents interface{}) error {
	if peer := client.GetPeer(message.SenderId); peer != nil {
		response, err := NewPeerMessage(client, messageType, contents)
		if err != nil {
			return err
		}
		peer.OutChannel <- response
		return nil
	} else {
		return fmt.Errorf("PeerMessage: No peer with id %d.", message.SenderId)
	}
}

func (message *PeerMessage) ReadValue(client *Client, into interface{}, secure bool) error {
	if secure {
		if peer := client.GetPeer(message.SenderId); peer != nil {
			hash := sha256.Sum256(message.Payload)
			err := rsa.VerifyPKCS1v15(peer.PublicKey, crypto.SHA256, hash[:], message.Signature)
			if err != nil {
				return err
			}
		} else {
			return fmt.Errorf("PeerMessage: No peer with id %d.", message.SenderId)
		}
	}
	decoder := gob.NewDecoder(bytes.NewBuffer(message.Payload))
	return decoder.Decode(into)
}
