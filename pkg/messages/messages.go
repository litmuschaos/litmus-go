package messages

import (
	"encoding/json"

	"github.com/gorilla/websocket"
)

type Message struct {
	Action  string      `json:"action"`
	Payload interface{} `json:"body"`
}

// ListenForAgentMessage listens for a message sent by the agent and returns its action and payload
func ListenForAgentMessage(conn *websocket.Conn) (string, []byte, error) {

	var msg Message

	err := conn.ReadJSON(&msg)
	if err != nil {
		return "", []byte{}, err
	}

	payload, err := json.Marshal(msg.Payload)
	if err != nil {
		return "", []byte{}, err
	}

	return msg.Action, payload, nil
}

// SendMessageToAgent sends a message consisting of an action and a payload to the agent
func SendMessageToAgent(conn *websocket.Conn, action string, payload interface{}) error {

	err := conn.WriteJSON(Message{action, payload})
	if err != nil {
		return err
	}

	return nil
}

// GetErrorMessage accepts the error message payload and returns the error message string
func GetErrorMessage(payload []byte) (string, error) {

	var agentError string

	if err := json.Unmarshal(payload, &agentError); err != nil {
		return "", err
	}

	return agentError, nil
}
