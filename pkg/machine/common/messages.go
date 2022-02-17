package common

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/pkg/errors"
)

var reqIDMap = make(map[string]chan *Message)
var mutex sync.Mutex

type Message struct {
	Action    string      `json:"action"`
	Payload   interface{} `json:"body"`
	RequestID string      `json:"reqid"`
}

// TODO: Add mutex locks while writing to the map
// TODO: Check if a reqID already exists

// ListenForAgentMessage listens for a message sent by the agent and returns its action and payload
func ListenForAgentMessage(conn *websocket.Conn) {

	for {

		var msg Message

		if err := conn.ReadJSON(&msg); err != nil {
			log.Errorf("An error occured while listening for agent message, err: %v", err)
		}

		reqIDMap[msg.RequestID] <- &msg
	}
}

// SendMessageToAgent sends a message consisting of an action and a payload to the agent
func SendMessageToAgent(conn *websocket.Conn, action string, payload interface{}, responseTimeout *time.Duration) (string, []byte, error) {

	reqID := uuid.New()
	_, keyExists := reqIDMap[reqID.String()]

	for keyExists {
		reqID = uuid.New()
		_, keyExists = reqIDMap[reqID.String()]
	}

	// if responseTimeout is nil, we won't look after the response sent by the agent
	if responseTimeout != nil {

		mutex.Lock()

		reqIDMap[reqID.String()] = make(chan *Message)

		mutex.Unlock()

		defer func() {
			close(reqIDMap[reqID.String()])
			delete(reqIDMap, reqID.String())
		}()
	}

	if err := conn.WriteJSON(Message{action, payload, reqID.String()}); err != nil {
		return "", nil, err
	}

	if responseTimeout == nil {
		return "", nil, nil
	}

	select {

	case <-time.After(*responseTimeout):
		return "", nil, errors.Errorf("failed to receive a response within specified timeout duration")

	case resp := <-reqIDMap[reqID.String()]:

		payload, err := json.Marshal(resp.Payload)
		if err != nil {
			return "", nil, err
		}

		return resp.Action, payload, nil
	}
}

// GetErrorMessage accepts the error message payload and returns the error message string
func GetErrorMessage(payload []byte) (string, error) {

	var agentError string

	if err := json.Unmarshal(payload, &agentError); err != nil {
		return "", err
	}

	return agentError, nil
}
