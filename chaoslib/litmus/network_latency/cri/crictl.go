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

// PIDFromContainer extract out the pids from the target containers
func PIDFromContainer(c coreV1.ContainerStatus) (int, error) {
	containerID := strings.Split(c.ContainerID, "//")[1]
	out, err := exec.Command("crictl", "inspect", containerID).CombinedOutput()
	if err != nil {
		log.Error(fmt.Sprintf("[cri] Failed to run crictl: %s", string(out)))
		return 0, err
	}

	runtime := strings.Split(c.ContainerID, "://")[0]
	PID, _ := parsePIDFromJSON(out, runtime)
	if err != nil {
		log.Error(fmt.Sprintf("[cri] Failed to parse json from crictl output: %s", string(out)))
		return 0, err
	}
	log.Info(fmt.Sprintf("[cri] Container ID=%s has process PID=%d", containerID, PID))

	return PID, nil
}

// InspectResponse JSON representation of crictl inspect command output
// in crio, pid is present inside pid attribute of inspect output
// in containerd, pid is present inside `info.pid` of inspect output
type InspectResponse struct {
	Info InfoDetails `json:"info"`
}

// InfoDetails JSON representation of crictl inspect command output
// in crio, pid is present inside pid attribute of inspect output
// in containerd, pid is present inside `info.pid` of inspect output
type InfoDetails struct {
	PID int `json:"pid"`
}

//parsePIDFromJSON extract the pid from the json output
func parsePIDFromJSON(j []byte, runtime string) (int, error) {

	var pid int

	// in crio, pid is present inside pid attribute of inspect output
	// in containerd, pid is present inside `info.pid` of inspect output
	if runtime == "containerd" {
		var resp InspectResponse
		if err := json.Unmarshal(j, &resp); err != nil {
			return 0, err
		}
		pid = resp.Info.PID
	} else {
		var resp InfoDetails
		if err := json.Unmarshal(j, &resp); err != nil {
			return 0, err
		}
		pid = resp.PID
	}

	if pid == 0 {
		return 0, errors.Errorf("[cri] Could not find pid field in json: %s", string(j))
	}

	return pid, nil
}
