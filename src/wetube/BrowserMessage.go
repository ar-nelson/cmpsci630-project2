package wetube

import (
	"encoding/json"
)

type BrowserMessage struct {
	Type    MsgType
	Message string
}

func NewBrowserMessage(messageType MsgType, contents interface{}) (*BrowserMessage, error) {
	b, err := json.Marshal(contents)
	if err != nil {
		return nil, err
	}
	return &BrowserMessage{messageType, string(b)}, nil
}

func (message *BrowserMessage) MsgType() MsgType {
	return message.Type
}

func (message *BrowserMessage) Sender() (int, string, bool) {
	return 0, "", true
}

func (message *BrowserMessage) Respond(client *Client, messageType MsgType, contents interface{}) error {
	response, err := NewBrowserMessage(messageType, contents)
	if err != nil {
		return err
	}
	client.ToBrowser <- response
	return nil
}

func (message *BrowserMessage) ReadValue(client *Client, into interface{}) error {
	return json.Unmarshal([]byte(message.Message), into)
}
