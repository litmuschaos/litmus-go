package process

import (
	"strconv"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/litmuschaos/litmus-go/pkg/guest-os/process-kill/types"
	"github.com/litmuschaos/litmus-go/pkg/messages"
	"github.com/pkg/errors"
)

// ProcessStateCheck validates that all the processes are running in the target node
func ProcessStateCheck(conn *websocket.Conn, processIds string) error {

	processIdList := strings.Split(processIds, ",")
	if len(processIdList) == 0 {
		return errors.Errorf("no process ID found")
	}

	var pids []int

	for _, pid := range processIdList {

		p, err := strconv.Atoi(pid)
		if err != nil {
			return errors.Errorf("unable to convert process id %s to integer, err: %v", pid, err)
		}

		pids = append(pids, p)
	}

	if err := messages.SendMessageToAgent(conn, "CHECK_STEADY_STATE", types.Processes{PIDs: pids}); err != nil {
		return errors.Errorf("failed to send message to agent, %v", err)
	}

	feedback, payload, err := messages.ListenForAgentMessage(conn)
	if err != nil {
		return errors.Errorf("error during reception of message from agent, %v", err)
	}

	if feedback != "ACTION_SUCCESSFUL" {
		if feedback == "ERROR" {

			agentError, err := messages.GetErrorMessage(payload)
			if err != nil {
				return errors.Errorf("failed to interpret error message from agent, ", err)
			}

			return errors.Errorf("error during steady state, ", agentError)
		}

		return errors.Errorf("unintelligible feedback received from agent: %s", feedback)
	}

	return nil
}
