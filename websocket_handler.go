package main

import (
	"encoding/json"
	"fmt"
)

// The websocket interface used for the mail program.
type WSInterface struct {
	ws *WS
}

// A standard message sent or recieved from the client.
type WSMessage struct {
	MessageType string      `json:"type"`
	Message     interface{} `json:"msg"`
}

// A general websocket response.
type WSGeneralResp struct {
	Status string `json:"status"`
	Error  string `json:"error"`
}

func (ws *WSInterface) buildMessageJson(msgType string, msg interface{}) (message []byte, err error) {
	// Build a message.
	wsMessage := WSMessage{
		MessageType: msgType,
		Message:     msg,
	}
	// Encode message to json.
	message, err = json.Marshal(wsMessage)
	return
}

// Send a message to the subscribed clients.
func (ws *WSInterface) sendMessage(msgType string, msg interface{}) error {
	// Build json.
	message, err := ws.buildMessageJson(msgType, msg)
	if err != nil {
		return err
	}
	// Send message to subscribed clients.
	ws.ws.message <- message
	return nil
}

// Send a message to a specific client.
func (ws *WSInterface) sendMessageToClient(msgType string, msg interface{}, c *WSClient) error {
	// Build json.
	message, err := ws.buildMessageJson(msgType, msg)
	if err != nil {
		return err
	}
	// Send message to client.
	c.send <- message
	return nil
}

// Handle a message from a client.
func (ws *WSInterface) handleMessage(message []byte, c *WSClient) {
	// Message should be in standard json.
	wsMessage := WSMessage{}
	err := json.Unmarshal(message, &wsMessage)

	// If we could not parse the message, return an error.
	if err != nil {
		resp := WSGeneralResp{
			Status: APIERR,
			Error:  fmt.Sprintf("Unable to decode request %v", err),
		}
		ws.sendMessageToClient("error", resp, c)
		return
	}

	// Depending on the message type, handle the message.
	switch wsMessage.MessageType {
	default:
		// By default, we do nothing but return an error.
		resp := WSGeneralResp{
			Status: APIERR,
			Error:  fmt.Sprintf("No handler of type %v", wsMessage.MessageType),
		}
		ws.sendMessageToClient("error", resp, c)
	}
}
