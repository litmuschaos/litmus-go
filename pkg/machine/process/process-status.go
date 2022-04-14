package process

import (
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/litmuschaos/litmus-go/pkg/machine/common/messages"
	"github.com/pkg/errors"
)

// ProcessStateCheck validates that all the processes are running in the target machine
func ProcessStateCheck(conn *websocket.Conn, processIds string) error {

	duration := 60 * time.Second

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

	feedback, payload, err := messages.SendMessageToAgent(conn, "CHECK_STEADY_STATE", pids, &duration)
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

	return nil
}
