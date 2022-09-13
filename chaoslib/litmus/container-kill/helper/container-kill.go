package helper

import (
	"bytes"
	"context"
	"fmt"
	"github.com/litmuschaos/litmus-go/pkg/result"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/container-kill/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientTypes "k8s.io/apimachinery/pkg/types"
)

var err error

// Helper injects the container-kill chaos
func Helper(clients clients.ClientSets) {

	experimentsDetails := experimentTypes.ExperimentDetails{}
	eventsDetails := types.EventDetails{}
	chaosDetails := types.ChaosDetails{}
	resultDetails := types.ResultDetails{}

	//Fetching all the ENV passed in the helper pod
	log.Info("[PreReq]: Getting the ENV variables")
	getENV(&experimentsDetails)

	// Intialise the chaos attributes
	types.InitialiseChaosVariables(&chaosDetails)

	// Intialise Chaos Result Parameters
	types.SetResultAttributes(&resultDetails, chaosDetails)

	err := killContainer(&experimentsDetails, clients, &eventsDetails, &chaosDetails, &resultDetails)
	if err != nil {
		log.Fatalf("helper pod failed, err: %v", err)
	}
}

// killContainer kill the random application container
// it will kill the container till the chaos duration
// the execution will stop after timestamp passes the given chaos duration
func killContainer(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails, resultDetails *types.ResultDetails) error {
	targetEnv := os.Getenv("TARGETS")
	if targetEnv == "" {
		return fmt.Errorf("no target found, provide atleast one target")
	}

	var targets []targetDetails

	for _, t := range strings.Split(targetEnv, ";") {
		target := strings.Split(t, ":")
		if len(target) != 2 {
			return fmt.Errorf("unsupported target: '%v', provide target in '<name>:<namespace>", target)
		}
		td := targetDetails{
			Name:            target[0],
			Namespace:       target[1],
			TargetContainer: experimentsDetails.TargetContainer,
		}

		if td.TargetContainer == "" {
			td.TargetContainer, err = common.GetTargetContainer(td.Namespace, td.Name, clients)
			if err != nil {
				return errors.Errorf("unable to get the target container name, err: %v", err)
			}
		}
		targets = append(targets, td)
	}

	if err := loop(targets, experimentsDetails, clients, eventsDetails, chaosDetails, resultDetails); err != nil {
		return err
	}

	log.Infof("[Completion]: %v chaos has been completed", experimentsDetails.ExperimentName)
	return nil
}

func loop(targets []targetDetails, experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails, resultDetails *types.ResultDetails) error {

	//ChaosStartTimeStamp contains the start timestamp, when the chaos injection begin
	ChaosStartTimeStamp := time.Now()
	duration := int(time.Since(ChaosStartTimeStamp).Seconds())

	for duration < experimentsDetails.ChaosDuration {

		for _, t := range targets {
			t.RestartCountBefore, err = getRestartCount(t, clients)
			if err != nil {
				return err
			}

			t.ContainerId, err = common.GetContainerID(t.Namespace, t.Name, t.TargetContainer, clients)
			if err != nil {
				return err
			}
		}

		for _, t := range targets {
			if err := kill(experimentsDetails, t, clients, eventsDetails, chaosDetails); err != nil {
				return err
			}
		}

		//Waiting for the chaos interval after chaos injection
		if experimentsDetails.ChaosInterval != 0 {
			log.Infof("[Wait]: Wait for the chaos interval %vs", experimentsDetails.ChaosInterval)
			common.WaitForDuration(experimentsDetails.ChaosInterval)
		}

		for _, t := range targets {
			if err := validate(t, experimentsDetails.Timeout, experimentsDetails.Delay, clients); err != nil {
				return err
			}
			if err := result.AnnotateChaosResult(resultDetails.Name, chaosDetails.ChaosNamespace, "targeted", "pod", experimentsDetails.TargetPods); err != nil {
				return err
			}
		}

		duration = int(time.Since(ChaosStartTimeStamp).Seconds())
	}
	return nil
}

func kill(experimentsDetails *experimentTypes.ExperimentDetails, t targetDetails, clients clients.ClientSets, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	log.InfoWithValues("[Info]: Details of application under chaos injection", logrus.Fields{
		"PodName":            t.Name,
		"ContainerName":      t.TargetContainer,
		"RestartCountBefore": t.RestartCountBefore,
	})

	// record the event inside chaosengine
	if experimentsDetails.EngineName != "" {
		msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on application pod"
		types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
		events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
	}

	switch experimentsDetails.ContainerRuntime {
	case "docker":
		if err := stopDockerContainer(t.ContainerId, experimentsDetails.SocketPath, experimentsDetails.Signal); err != nil {
			return err
		}
	case "containerd", "crio":
		if err := stopContainerdContainer(t.ContainerId, experimentsDetails.SocketPath, experimentsDetails.Signal); err != nil {
			return err
		}
	default:
		return errors.Errorf("%v container runtime not supported", experimentsDetails.ContainerRuntime)
	}
	return nil
}

func validate(t targetDetails, timeout, delay int, clients clients.ClientSets) error {
	//Check the status of restarted container
	err := common.CheckContainerStatus(t.Namespace, t.Name, timeout, delay, clients)
	if err != nil {
		return err
	}

	// It will verify that the restart count of container should increase after chaos injection
	return verifyRestartCount(t, timeout, delay, clients, t.RestartCountBefore)
}

//stopContainerdContainer kill the application container
func stopContainerdContainer(containerID, socketPath, signal string) error {
	var errOut bytes.Buffer
	var cmd *exec.Cmd
	endpoint := "unix://" + socketPath
	switch signal {
	case "SIGKILL":
		cmd = exec.Command("sudo", "crictl", "-i", endpoint, "-r", endpoint, "stop", "--timeout=0", string(containerID))
	case "SIGTERM":
		cmd = exec.Command("sudo", "crictl", "-i", endpoint, "-r", endpoint, "stop", string(containerID))
	default:
		return errors.Errorf("{%v} signal not supported, use either SIGTERM or SIGKILL", signal)
	}
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		return errors.Errorf("Unable to run command, err: %v; error output: %v", err, errOut.String())
	}
	return nil
}

//stopDockerContainer kill the application container
func stopDockerContainer(containerID, socketPath, signal string) error {
	var errOut bytes.Buffer
	host := "unix://" + socketPath
	cmd := exec.Command("sudo", "docker", "--host", host, "kill", string(containerID), "--signal", signal)
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		return errors.Errorf("Unable to run command, err: %v; error output: %v", err, errOut.String())
	}
	return nil
}

//getRestartCount return the restart count of target container
func getRestartCount(target targetDetails, clients clients.ClientSets) (int, error) {
	pod, err := clients.KubeClient.CoreV1().Pods(target.Namespace).Get(context.Background(), target.Name, v1.GetOptions{})
	if err != nil {
		return 0, err
	}
	restartCount := 0
	for _, container := range pod.Status.ContainerStatuses {
		if container.Name == target.TargetContainer {
			restartCount = int(container.RestartCount)
			break
		}
	}
	return restartCount, nil
}

//verifyRestartCount verify the restart count of target container that it is restarted or not after chaos injection
func verifyRestartCount(t targetDetails, timeout, delay int, clients clients.ClientSets, restartCountBefore int) error {

	restartCountAfter := 0
	return retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {
			pod, err := clients.KubeClient.CoreV1().Pods(t.Namespace).Get(context.Background(), t.Name, v1.GetOptions{})
			if err != nil {
				return errors.Errorf("Unable to find the pod with name %v, err: %v", t.Name, err)
			}
			for _, container := range pod.Status.ContainerStatuses {
				if container.Name == t.TargetContainer {
					restartCountAfter = int(container.RestartCount)
					break
				}
			}
			if restartCountAfter <= restartCountBefore {
				return errors.Errorf("Target container is not restarted")
			}
			log.Infof("restartCount of target container after chaos injection: %v", strconv.Itoa(restartCountAfter))
			return nil
		})
}

//getENV fetches all the env variables from the runner pod
func getENV(experimentDetails *experimentTypes.ExperimentDetails) {
	experimentDetails.ExperimentName = types.Getenv("EXPERIMENT_NAME", "")
	experimentDetails.InstanceID = types.Getenv("INSTANCE_ID", "")
	experimentDetails.TargetContainer = types.Getenv("APP_CONTAINER", "")
	experimentDetails.ChaosDuration, _ = strconv.Atoi(types.Getenv("TOTAL_CHAOS_DURATION", "30"))
	experimentDetails.ChaosInterval, _ = strconv.Atoi(types.Getenv("CHAOS_INTERVAL", "10"))
	experimentDetails.ChaosNamespace = types.Getenv("CHAOS_NAMESPACE", "litmus")
	experimentDetails.EngineName = types.Getenv("CHAOSENGINE", "")
	experimentDetails.ChaosUID = clientTypes.UID(types.Getenv("CHAOS_UID", ""))
	experimentDetails.ChaosPodName = types.Getenv("POD_NAME", "")
	experimentDetails.SocketPath = types.Getenv("SOCKET_PATH", "")
	experimentDetails.ContainerRuntime = types.Getenv("CONTAINER_RUNTIME", "")
	experimentDetails.Signal = types.Getenv("SIGNAL", "SIGKILL")
	experimentDetails.Delay, _ = strconv.Atoi(types.Getenv("STATUS_CHECK_DELAY", "2"))
	experimentDetails.Timeout, _ = strconv.Atoi(types.Getenv("STATUS_CHECK_TIMEOUT", "180"))
}

type targetDetails struct {
	Name               string
	Namespace          string
	TargetContainer    string
	RestartCountBefore int
	ContainerId        string
}
