package cpu

import (
	"time"

	"github.com/gorilla/websocket"
	"github.com/litmuschaos/litmus-go/pkg/machine/common/messages"
	"github.com/pkg/errors"
)

// CheckPrerequisites validates the pre-requisites for the experiment
func CheckPrerequisites(connections []*websocket.Conn) error {

	for _, conn := range connections {
		duration := 60 * time.Second

		feedback, payload, err := messages.SendMessageToAgent(conn, "CHECK_STEADY_STATE", nil, &duration)
		if err != nil {
			return errors.Errorf("failed to send message to agent, err: %v", err)
		}

		// ACTION_SUCCESSFUL feedback is received only if all the processes exist in the target machine
		if feedback != "ACTION_SUCCESSFUL" {
			if feedback == "ERROR" {

				agentError, err := messages.GetErrorMessage(payload)
				if err != nil {
					return errors.Errorf("failed to interpret error message from agent, err: %v", err)
				}

				return errors.Errorf("error during steady-state validation, err: %s", agentError)
			}

			return errors.Errorf("unintelligible feedback received from agent: %s", feedback)
		}
	}

	return nil
}
