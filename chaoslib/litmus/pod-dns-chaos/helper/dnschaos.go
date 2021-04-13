package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentEnv "github.com/litmuschaos/litmus-go/pkg/generic/pod-dns-chaos/environment"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/pod-dns-chaos/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/result"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/pkg/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientTypes "k8s.io/apimachinery/pkg/types"
)

var abort, injectAbort chan os.Signal

func main() {

	experimentsDetails := experimentTypes.ExperimentDetails{}
	client := clients.ClientSets{}
	eventsDetails := types.EventDetails{}
	chaosDetails := types.ChaosDetails{}
	resultDetails := types.ResultDetails{}

	// abort channel is used to transmit signal notifications.
	abort = make(chan os.Signal, 1)
	// abort channel is used to transmit signal notifications.
	injectAbort = make(chan os.Signal, 1)

	// Catch and relay certain signal(s) to abort channel.
	signal.Notify(abort, os.Interrupt, syscall.SIGTERM, syscall.SIGKILL)

	//Getting kubeConfig and Generate ClientSets
	if err := client.GenerateClientSetFromKubeConfig(); err != nil {
		log.Fatalf("Unable to Get the kubeconfig, err: %v", err)
	}

	//Fetching all the ENV passed for the helper pod
	log.Info("[PreReq]: Getting the ENV variables")
	GetENV(&experimentsDetails)

	// Initialise the chaos attributes
	experimentEnv.InitialiseChaosVariables(&chaosDetails, &experimentsDetails)

	// Initialise Chaos Result Parameters
	types.SetResultAttributes(&resultDetails, chaosDetails)

	// Set the chaos result uid
	result.SetResultUID(&resultDetails, client, &chaosDetails)

	err := PreparePodDNSChaos(&experimentsDetails, client, &eventsDetails, &chaosDetails, &resultDetails)
	if err != nil {
		log.Fatalf("helper pod failed, err: %v", err)
	}

}

//PreparePodDNSChaos contains the preparation steps before chaos injection
func PreparePodDNSChaos(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails, resultDetails *types.ResultDetails) error {

	containerID, err := GetContainerID(experimentsDetails, clients)
	if err != nil {
		return err
	}
	// extract out the pid of the target container
	pid, err := GetPID(experimentsDetails, containerID)
	if err != nil {
		return err
	}

	// record the event inside chaosengine
	if experimentsDetails.EngineName != "" {
		msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on application pod"
		types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
		events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
	}

	// prepare dns interceptor
	commandTemplate := fmt.Sprintf("sudo TARGET_PID=%d CHAOS_TYPE=%s TARGET_HOSTNAMES='%s' CHAOS_DURATION=%d MATCH_SCHEME=%s nsutil -p -n -t %d -- dns_interceptor", pid, experimentsDetails.ChaosType, experimentsDetails.TargetHostNames, experimentsDetails.ChaosDuration, experimentsDetails.MatchScheme, pid)
	cmd := exec.Command("/bin/bash", "-c", commandTemplate)
	log.Info(cmd.String())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// injecting dns chaos inside target container
	go func() {
		select {
		case <-injectAbort:
			log.Info("[Chaos]: Abort received, skipping chaos injection")
		default:
			err = cmd.Run()
			if err != nil {
				log.Fatalf("dns interceptor failed : %v", err)
			}
		}
	}()

	timeChan := time.Tick(time.Duration(experimentsDetails.ChaosDuration) * time.Second)
	log.Infof("[Chaos]: Waiting for %vs", experimentsDetails.ChaosDuration)

	// either wait for abort signal or chaos duration
	select {
	case <-abort:
		log.Info("[Chaos]: Killing process started because of terminated signal received")
	case <-timeChan:
		log.Info("[Chaos]: Stopping the experiment, chaos duration over")
	}

	log.Info("Chaos Revert Started")
	// retry thrice for the chaos revert

	retry := 3
	for retry > 0 {
		if cmd.Process == nil {
			log.Infof("cannot kill dns interceptor, process not started. Retrying in 1sec...")
		} else {
			log.Infof("killing dns interceptor with pid %v", cmd.Process.Pid)
			// kill command
			killTemplate := fmt.Sprintf("sudo kill %d", cmd.Process.Pid)
			kill := exec.Command("/bin/bash", "-c", killTemplate)
			if err = kill.Run(); err != nil {
				log.Errorf("unable to kill dns interceptor process cry, err :%v", err)
			} else {
				log.Errorf("dns interceptor process stopped")
				break
			}
		}
		retry--
		time.Sleep(1 * time.Second)
	}
	log.Info("Chaos Revert Completed")
	return nil
}

//GetContainerID extract out the container id of the target container
func GetContainerID(experimentDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) (string, error) {

	var containerID string
	switch experimentDetails.ContainerRuntime {
	case "docker":
		host := "unix://" + experimentDetails.SocketPath
		// deriving the container id of the pause container
		cmd := "sudo docker --host " + host + " ps | grep k8s_POD_" + experimentDetails.TargetPods + "_" + experimentDetails.AppNS + " | awk '{print $1}'"
		out, err := exec.Command("/bin/sh", "-c", cmd).CombinedOutput()
		if err != nil {
			log.Error(fmt.Sprintf("[docker]: Failed to run docker ps command: %s", string(out)))
			return "", err
		}
		containerID = strings.TrimSpace(string(out))
	case "containerd", "crio":
		pod, err := clients.KubeClient.CoreV1().Pods(experimentDetails.AppNS).Get(experimentDetails.TargetPods, v1.GetOptions{})
		if err != nil {
			return "", err
		}
		// filtering out the container id from the details of containers inside containerStatuses of the given pod
		// container id is present in the form of <runtime>://<container-id>
		for _, container := range pod.Status.ContainerStatuses {
			if container.Name == experimentDetails.TargetContainer {
				containerID = strings.Split(container.ContainerID, "//")[1]
				break
			}
		}
	default:
		return "", errors.Errorf("%v container runtime not suported", experimentDetails.ContainerRuntime)
	}
	log.Infof("containerid: %v", containerID)

	return containerID, nil
}

//GetPID extract out the PID of the target container
func GetPID(experimentDetails *experimentTypes.ExperimentDetails, containerID string) (int, error) {
	var PID int

	switch experimentDetails.ContainerRuntime {
	case "docker":
		host := "unix://" + experimentDetails.SocketPath
		// deriving pid from the inspect out of target container
		out, err := exec.Command("sudo", "docker", "--host", host, "inspect", containerID).CombinedOutput()
		if err != nil {
			log.Error(fmt.Sprintf("[docker]: Failed to run docker inspect: %s", string(out)))
			return 0, err
		}
		// parsing data from the json output of inspect command
		PID, err = parsePIDFromJSON(out, experimentDetails.ContainerRuntime)
		if err != nil {
			log.Error(fmt.Sprintf("[docker]: Failed to parse json from docker inspect output: %s", string(out)))
			return 0, err
		}
	case "containerd", "crio":
		// deriving pid from the inspect out of target container
		endpoint := "unix://" + experimentDetails.SocketPath
		out, err := exec.Command("sudo", "crictl", "-i", endpoint, "-r", endpoint, "inspect", containerID).CombinedOutput()
		if err != nil {
			log.Error(fmt.Sprintf("[cri]: Failed to run crictl: %s", string(out)))
			return 0, err
		}
		// parsing data from the json output of inspect command
		PID, err = parsePIDFromJSON(out, experimentDetails.ContainerRuntime)
		if err != nil {
			log.Errorf(fmt.Sprintf("[cri]: Failed to parse json from crictl output: %s", string(out)))
			return 0, err
		}
	default:
		return 0, errors.Errorf("%v container runtime not suported", experimentDetails.ContainerRuntime)
	}

	log.Info(fmt.Sprintf("[cri]: Container ID=%s has process PID=%d", containerID, PID))

	return PID, nil
}

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
		return 0, errors.Errorf("[cri]: No supported container runtime, runtime: %v", runtime)
	}
	if pid == 0 {
		return 0, errors.Errorf("[cri]: No running target container found, pid: %d", pid)
	}

	return pid, nil
}

//GetENV fetches all the env variables from the runner pod
func GetENV(experimentDetails *experimentTypes.ExperimentDetails) {
	experimentDetails.ExperimentName = Getenv("EXPERIMENT_NAME", "")
	experimentDetails.AppNS = Getenv("APP_NS", "")
	experimentDetails.TargetContainer = Getenv("APP_CONTAINER", "")
	experimentDetails.TargetPods = Getenv("APP_POD", "")
	experimentDetails.AppLabel = Getenv("APP_LABEL", "")
	experimentDetails.ChaosDuration, _ = strconv.Atoi(Getenv("CHAOS_DURATION", "30"))
	experimentDetails.ChaosNamespace = Getenv("CHAOS_NAMESPACE", "litmus")
	experimentDetails.EngineName = Getenv("CHAOS_ENGINE", "")
	experimentDetails.ChaosUID = clientTypes.UID(Getenv("CHAOS_UID", ""))
	experimentDetails.ChaosPodName = Getenv("POD_NAME", "")
	experimentDetails.ContainerRuntime = Getenv("CONTAINER_RUNTIME", "")
	experimentDetails.TargetHostNames = Getenv("TARGET_HOSTNAMES", "")
	experimentDetails.MatchScheme = Getenv("MATCH_SCHEME", "exact")
	experimentDetails.ChaosType = Getenv("CHAOS_TYPE", "error")
	experimentDetails.SocketPath = Getenv("SOCKET_PATH", "")
}

// Getenv fetch the env and set the default value, if any
func Getenv(key string, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		value = defaultValue
	}
	return value
}
