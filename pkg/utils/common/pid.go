package common

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/palantir/stacktrace"
	"os/exec"
	"strconv"
	"strings"

	"github.com/litmuschaos/litmus-go/pkg/log"
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

func getDockerPID(containerID, socketPath, source string) (int, error) {
	cmd := exec.Command("sudo", "docker", "--host", fmt.Sprintf("unix://%s", socketPath), "inspect", containerID)
	out, err := inspect(cmd, containerID, source)
	if err != nil {
		return 0, stacktrace.Propagate(err, "could not inspect container id")
	}

	// in docker, pid is present inside state.pid attribute of inspect output
	var resp []DockerInspectResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return 0, cerrors.Error{ErrorCode: cerrors.ErrorTypeContainerRuntime, Source: source, Target: fmt.Sprintf("containerID: %s", containerID), Reason: fmt.Sprintf("failed to parse pid: %s", err.Error())}
	}
	pid := resp[0].State.PID
	return pid, nil
}

func getContainerdSandboxPID(containerID, socketPath, source string) (int, error) {
	var pid int
	cmd := exec.Command("sudo", "crictl", "-i", fmt.Sprintf("unix://%s", socketPath), "-r", fmt.Sprintf("unix://%s", socketPath), "inspect", containerID)
	out, err := inspect(cmd, containerID, source)
	if err != nil {
		return 0, stacktrace.Propagate(err, "could not inspect container id")
	}

	var resp CrictlInspectResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return 0, cerrors.Error{ErrorCode: cerrors.ErrorTypeContainerRuntime, Source: source, Target: fmt.Sprintf("containerID: %s", containerID), Reason: fmt.Sprintf("failed to parse pid: %s", err.Error())}
	}
	for _, namespace := range resp.Info.RuntimeSpec.Linux.Namespaces {
		if namespace.Type == "network" {
			value := strings.Split(namespace.Path, "/")[2]
			pid, _ = strconv.Atoi(value)
		}
	}
	return pid, nil
}

func getContainerdPID(containerID, socketPath, source string) (int, error) {
	var pid int
	cmd := exec.Command("sudo", "crictl", "-i", fmt.Sprintf("unix://%s", socketPath), "-r", fmt.Sprintf("unix://%s", socketPath), "inspect", containerID)
	out, err := inspect(cmd, containerID, source)
	if err != nil {
		return 0, stacktrace.Propagate(err, "could not inspect container id")
	}

	var resp CrictlInspectResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return 0, cerrors.Error{ErrorCode: cerrors.ErrorTypeContainerRuntime, Source: source, Target: fmt.Sprintf("{containerID: %s}", containerID), Reason: fmt.Sprintf("failed to parse pid: %s", err.Error())}
	}
	pid = resp.Info.PID
	if pid == 0 {
		return 0, cerrors.Error{ErrorCode: cerrors.ErrorTypeContainerRuntime, Source: source, Target: fmt.Sprintf("{containerID: %s}", containerID), Reason: fmt.Sprintf("no running target container found")}
	}
	return pid, nil
}

func getCRIOPID(containerID, socketPath, source string) (int, error) {
	var pid int
	cmd := exec.Command("sudo", "crictl", "-i", fmt.Sprintf("unix://%s", socketPath), "-r", fmt.Sprintf("unix://%s", socketPath), "inspect", containerID)
	out, err := inspect(cmd, containerID, source)
	if err != nil {
		return 0, stacktrace.Propagate(err, "could not inspect container id")
	}

	var info InfoDetails
	if err := json.Unmarshal(out, &info); err != nil {
		return 0, cerrors.Error{ErrorCode: cerrors.ErrorTypeContainerRuntime, Source: source, Target: fmt.Sprintf("containerID: %s", containerID), Reason: fmt.Sprintf("failed to parse pid: %s", err.Error())}
	}
	pid = info.PID
	if pid == 0 {
		var resp CrictlInspectResponse
		if err := json.Unmarshal(out, &resp); err != nil {
			return 0, cerrors.Error{ErrorCode: cerrors.ErrorTypeContainerRuntime, Source: source, Target: fmt.Sprintf("containerID: %s", containerID), Reason: fmt.Sprintf("failed to parse pid: %s", err.Error())}
		}
		pid = resp.Info.PID
	}
	return pid, nil
}

//GetPauseAndSandboxPID extract out the PID of the target container
func GetPauseAndSandboxPID(runtime, containerID, socketPath, source string) (int, error) {
	var pid int

	switch runtime {
	case "docker":
		pid, err = getDockerPID(containerID, socketPath, source)
	case "containerd":
		pid, err = getContainerdSandboxPID(containerID, socketPath, source)
	case "crio":
		pid, err = getCRIOPID(containerID, socketPath, source)
	default:
		return 0, cerrors.Error{ErrorCode: cerrors.ErrorTypeHelper, Source: source, Reason: fmt.Sprintf("unsupported container runtime: %s", runtime)}
	}
	if err != nil {
		return 0, err
	}

	if pid == 0 {
		return 0, cerrors.Error{ErrorCode: cerrors.ErrorTypeHelper, Source: source, Target: fmt.Sprintf("containerID: %s", containerID), Reason: fmt.Sprintf("no running target container found")}
	}

	log.Info(fmt.Sprintf("[Info]: Container ID=%s has process PID=%d", containerID, pid))
	return pid, nil
}

func GetPID(runtime, containerID, socketPath, source string) (int, error) {
	if runtime == "containerd" {
		return getContainerdPID(containerID, socketPath, source)
	}
	return GetPauseAndSandboxPID(runtime, containerID, socketPath, source)
}

func inspect(cmd *exec.Cmd, containerID, source string) ([]byte, error) {
	var out, stdErr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stdErr
	if err := cmd.Run(); err != nil {
		return nil, cerrors.Error{ErrorCode: cerrors.ErrorTypeContainerRuntime, Source: source, Target: fmt.Sprintf("{containerID: %s}", containerID), Reason: fmt.Sprintf("failed to get container pid: %s", stdErr.String())}
	}
	return out.Bytes(), nil
}
