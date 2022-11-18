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

func getDockerPID(containerID, socketPath string) (int, error) {
	host := "unix://" + socketPath
	// deriving pid from the inspect out of target container
	out, err := exec.Command("sudo", "docker", "--host", host, "inspect", containerID).CombinedOutput()
	if err != nil {
		log.Error(fmt.Sprintf("[docker]: Failed to run docker inspect: %s", string(out)))
		return 0, err
	}
	// in docker, pid is present inside state.pid attribute of inspect output
	var resp []DockerInspectResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return 0, err
	}
	pid := resp[0].State.PID
	return pid, nil
}

func getContainerdSandboxPID(containerID, socketPath string) (int, error) {
	var pid int
	endpoint := "unix://" + socketPath
	out, err := exec.Command("sudo", "crictl", "-i", endpoint, "-r", endpoint, "inspect", containerID).CombinedOutput()
	if err != nil {
		log.Error(fmt.Sprintf("[cri]: Failed to run crictl: %s", string(out)))
		return 0, err
	}
	var resp CrictlInspectResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return 0, err
	}
	for _, namespace := range resp.Info.RuntimeSpec.Linux.Namespaces {
		if namespace.Type == "network" {
			value := strings.Split(namespace.Path, "/")[2]
			pid, _ = strconv.Atoi(value)
		}
	}
	return pid, nil
}

func getContainerdPID(containerID, socketPath string) (int, error) {
	var pid int
	endpoint := "unix://" + socketPath
	out, err := exec.Command("sudo", "crictl", "-i", endpoint, "-r", endpoint, "inspect", containerID).CombinedOutput()
	if err != nil {
		log.Error(fmt.Sprintf("[cri]: Failed to run crictl: %s", string(out)))
		return 0, err
	}
	var resp CrictlInspectResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return 0, err
	}
	pid = resp.Info.PID
	if pid == 0 {
		return 0, errors.Errorf("[cri]: No running target container found, pid: %d", pid)
	}
	return pid, nil
}

func getCRIOPID(containerID, socketPath string) (int, error) {
	var pid int
	endpoint := "unix://" + socketPath
	out, err := exec.Command("sudo", "crictl", "-i", endpoint, "-r", endpoint, "inspect", containerID).CombinedOutput()
	if err != nil {
		log.Error(fmt.Sprintf("[cri]: Failed to run crictl: %s", string(out)))
		return 0, err
	}
	var info InfoDetails
	if err := json.Unmarshal(out, &info); err != nil {
		return 0, err
	}
	pid = info.PID
	if pid == 0 {
		var resp CrictlInspectResponse
		if err := json.Unmarshal(out, &resp); err != nil {
			return 0, err
		}
		pid = resp.Info.PID
	}
	return pid, nil
}

//GetPauseAndSandboxPID extract out the PID of the target container
func GetPauseAndSandboxPID(runtime, containerID, socketPath string) (int, error) {
	var pid int

	switch runtime {
	case "docker":
		pid, err = getDockerPID(containerID, socketPath)
	case "containerd":
		pid, err = getContainerdSandboxPID(containerID, socketPath)
	case "crio":
		pid, err = getCRIOPID(containerID, socketPath)
	default:
		return 0, errors.Errorf("%v container runtime not suported", runtime)
	}
	if err != nil {
		return 0, err
	}

	if pid == 0 {
		return 0, errors.Errorf("[cri]: No running target container found, pid: %d", pid)
	}

	log.Info(fmt.Sprintf("[Info]: Container ID=%s has process PID=%d", containerID, pid))
	return pid, nil
}

func GetPID(runtime, containerID, socketPath string) (int, error) {
	if runtime == "containerd" {
		return getContainerdPID(containerID, socketPath)
	}
	return GetPauseAndSandboxPID(runtime, containerID, socketPath)
}
