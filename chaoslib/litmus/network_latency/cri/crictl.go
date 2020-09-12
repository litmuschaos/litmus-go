package cri

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/pkg/errors"
	coreV1 "k8s.io/api/core/v1"
)

func PIDFromContainer(c coreV1.ContainerStatus) (int, error) {
	containerID := strings.TrimPrefix(c.ContainerID, "cri-o://")
	out, err := exec.Command("crictl", "inspect", containerID).CombinedOutput()
	if err != nil {
		log.Error(fmt.Sprintf("[cri] Failed to run crictl: %s", string(out)))
		return 0, err
	}

	PID, _ := parsePIDFromJson(out)
	if err != nil {
		log.Error(fmt.Sprintf("[cri] Failed to parse json from crictl output: %s", string(out)))
		return 0, err
	}
	log.Info(fmt.Sprintf("[cri] Container ID=%s has process PID=%d", containerID, PID))

	return PID, nil
}

// JSON representation of crictl inspect command output
type InspectResponse struct {
	PID int `json:"pid"`
}

func parsePIDFromJson(j []byte) (int, error) {

	var resp InspectResponse
	err := json.Unmarshal(j, &resp)
	if err != nil {
		return 0, err
	}
	if resp.PID == 0 {
		return 0, errors.Errorf("[cri] Could not find pid field in json: %s", string(j))
	}

	return resp.PID, nil
}
