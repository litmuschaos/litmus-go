package common

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/pkg/errors"
)

// CrictlInspectResponse JSON representation of crictl inspect command output
// in crio, pid is present inside pid attribute of inspect output
// in containerd, pid is present inside `info.pid` of inspect output
type CrictlInspectResponse struct {
	Info InfoDetails `json:"info"`
}

// InfoDetails JSON representation of crictl inspect command output
type InfoDetails struct {
	RuntimeSpec RuntimeDetails `json:"runtimeSpec"`
	PID         int            `json:"pid"`
}

// RuntimeDetails contains runtime details
type RuntimeDetails struct {
	Linux LinuxAttributes `json:"linux"`
}

// LinuxAttributes contains all the linux attributes
type LinuxAttributes struct {
	Namespaces []Namespace `json:"namespaces"`
}

// Namespace contains linux namespace details
type Namespace struct {
	Type string `json:"type"`
	Path string `json:"path"`
}

// DockerInspectResponse JSON representation of docker inspect command output
type DockerInspectResponse struct {
	State StateDetails `json:"state"`
}

// StateDetails JSON representation of docker inspect command output
type StateDetails struct {
	PID int `json:"pid"`
}

//GetPID extract out the PID of the target container
func GetPID(runtime, containerID, socketPath string) (int, error) {
	var PID int

	switch runtime {
	case "docker":
		host := "unix://" + socketPath
		// deriving pid from the inspect out of target container
		out, err := exec.Command("sudo", "docker", "--host", host, "inspect", containerID).CombinedOutput()
		if err != nil {
			log.Error(fmt.Sprintf("[docker]: Failed to run docker inspect: %s", string(out)))
			return 0, err
		}
		// parsing data from the json output of inspect command
		PID, err = parsePIDFromJSON(out, runtime)
		if err != nil {
			log.Error(fmt.Sprintf("[docker]: Failed to parse json from docker inspect output: %s", string(out)))
			return 0, err
		}
	case "containerd", "crio":
		// deriving pid from the inspect out of target container
		endpoint := "unix://" + socketPath
		out, err := exec.Command("sudo", "crictl", "-i", endpoint, "-r", endpoint, "inspect", containerID).CombinedOutput()
		if err != nil {
			log.Error(fmt.Sprintf("[cri]: Failed to run crictl: %s", string(out)))
			return 0, err
		}
		// parsing data from the json output of inspect command
		PID, err = parsePIDFromJSON(out, runtime)
		if err != nil {
			log.Errorf(fmt.Sprintf("[cri]: Failed to parse json from crictl output: %s", string(out)))
			return 0, err
		}
	default:
		return 0, errors.Errorf("%v container runtime not suported", runtime)
	}

	log.Info(fmt.Sprintf("[Info]: Container ID=%s has process PID=%d", containerID, PID))

	return PID, nil
}

//parsePIDFromJSON extract the pid from the json output
func parsePIDFromJSON(j []byte, runtime string) (int, error) {
	var pid int
	// namespaces are present inside `info.runtimeSpec.linux.namespaces` of inspect output
	// linux namespace of type network contains pid, in the form of `/proc/<pid>/ns/net`
	switch runtime {
	case "docker":
		// in docker, pid is present inside state.pid attribute of inspect output
		var resp []DockerInspectResponse
		if err := json.Unmarshal(j, &resp); err != nil {
			return 0, err
		}
		pid = resp[0].State.PID
	case "containerd":
		var resp CrictlInspectResponse
		if err := json.Unmarshal(j, &resp); err != nil {
			return 0, err
		}
		for _, namespace := range resp.Info.RuntimeSpec.Linux.Namespaces {
			if namespace.Type == "network" {
				value := strings.Split(namespace.Path, "/")[2]
				pid, _ = strconv.Atoi(value)
			}
		}
	case "crio":
		var info InfoDetails
		if err := json.Unmarshal(j, &info); err != nil {
			return 0, err
		}
		pid = info.PID
		if pid == 0 {
			var resp CrictlInspectResponse
			if err := json.Unmarshal(j, &resp); err != nil {
				return 0, err
			}
			pid = resp.Info.PID
		}
	default:
		return 0, errors.Errorf("[cri]: unsupported container runtime, runtime: %v", runtime)
	}
	if pid == 0 {
		return 0, errors.Errorf("[cri]: No running target container found, pid: %d", pid)
	}

	return pid, nil
}
