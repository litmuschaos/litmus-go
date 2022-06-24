package cpu

import (
	"strconv"
	"time"

	"github.com/gorilla/websocket"
	"github.com/litmuschaos/litmus-go/pkg/machine/common/messages"
	"github.com/pkg/errors"
)

// CheckPrerequisites validates the pre-requisites for the experiment
func CheckPrerequisites(cpus, loadPercentage string, connections []*websocket.Conn) error {

	if _, err := strconv.Atoi(cpus); err != nil {
		return errors.Errorf("invalid number of CPUs, err: %v", err)
	}

	load, err := strconv.Atoi(loadPercentage)
	if err != nil {
		return errors.Errorf("invalid load percentage value, err: %v", err)
	}

	if load < 0 || load > 100 {
		return errors.Errorf("invalid load percentage value, err: the value must lie inclusively between 0 to 100")
	}

	for _, conn := range connections {
		duration := 60 * time.Second

		feedback, payload, err := messages.SendMessageToAgent(conn, "CHECK_STEADY_STATE", nil, &duration)
		if err != nil {
			return errors.Errorf("failed to send message to agent, err: %v", err)
		}

		if err := messages.ValidateAgentFeedback(feedback, payload); err != nil {
			return errors.Errorf("error during steady-state validation, err: %v", err)
		}
	}

	return nil
}
