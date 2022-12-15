package helper

import (
	"bytes"
	"context"
	"fmt"
	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/litmuschaos/litmus-go/pkg/result"
	"github.com/palantir/stacktrace"
	"github.com/sirupsen/logrus"
	"os/exec"
	"strconv"
	"time"

	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/container-kill/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
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

	// Initialise the chaos attributes
	types.InitialiseChaosVariables(&chaosDetails)
	chaosDetails.Phase = types.ChaosInjectPhase

	// Initialise Chaos Result Parameters
	types.SetResultAttributes(&resultDetails, chaosDetails)

	if err := killContainer(&experimentsDetails, clients, &eventsDetails, &chaosDetails, &resultDetails); err != nil {
		// update failstep inside chaosresult
		if resultErr := result.UpdateFailedStepFromHelper(&resultDetails, &chaosDetails, clients, err); resultErr != nil {
			log.Fatalf("helper pod failed, err: %v, resultErr: %v", err, resultErr)
		}
		log.Fatalf("helper pod failed, err: %v", err)
	}
}

// killContainer kill the random application container
// it will kill the container till the chaos duration
// the execution will stop after timestamp passes the given chaos duration
func killContainer(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails, resultDetails *types.ResultDetails) error {
	targetList, err := common.ParseTargets(chaosDetails.ChaosPodName)
	if err != nil {
		return stacktrace.Propagate(err, "could not parse targets")
	}

	var targets []targetDetails

	for _, t := range targetList.Target {
		td := targetDetails{
			Name:            t.Name,
			Namespace:       t.Namespace,
			TargetContainer: t.TargetContainer,
			Source:          chaosDetails.ChaosPodName,
		}
		targets = append(targets, td)
		log.Infof("Injecting chaos on target: {name: %s, namespace: %v, container: %v}", t.Name, t.Namespace, t.TargetContainer)
	}

	if err := killIterations(targets, experimentsDetails, clients, eventsDetails, chaosDetails, resultDetails); err != nil {
		return err
	}

	log.Infof("[Completion]: %v chaos has been completed", experimentsDetails.ExperimentName)
	return nil
}

func killIterations(targets []targetDetails, experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails, resultDetails *types.ResultDetails) error {

	//ChaosStartTimeStamp contains the start timestamp, when the chaos injection begin
	ChaosStartTimeStamp := time.Now()
	duration := int(time.Since(ChaosStartTimeStamp).Seconds())

	for duration < experimentsDetails.ChaosDuration {

		var containerIds []string

		for _, t := range targets {
			t.RestartCountBefore, err = getRestartCount(t, clients)
			if err != nil {
				return stacktrace.Propagate(err, "could get container restart count")
			}

			containerId, err := common.GetContainerID(t.Namespace, t.Name, t.TargetContainer, clients, t.Source)
			if err != nil {
				return stacktrace.Propagate(err, "could not get container id")
			}

			log.InfoWithValues("[Info]: Details of application under chaos injection", logrus.Fields{
				"PodName":            t.Name,
				"ContainerName":      t.TargetContainer,
				"RestartCountBefore": t.RestartCountBefore,
			})

			containerIds = append(containerIds, containerId)
		}

		if err := kill(experimentsDetails, containerIds, clients, eventsDetails, chaosDetails); err != nil {
			return stacktrace.Propagate(err, "could not kill target container")
		}

		//Waiting for the chaos interval after chaos injection
		if experimentsDetails.ChaosInterval != 0 {
			log.Infof("[Wait]: Wait for the chaos interval %vs", experimentsDetails.ChaosInterval)
			common.WaitForDuration(experimentsDetails.ChaosInterval)
		}

		for _, t := range targets {
			if err := validate(t, experimentsDetails.Timeout, experimentsDetails.Delay, clients); err != nil {
				return stacktrace.Propagate(err, "could not verify restart count")
			}
			if err := result.AnnotateChaosResult(resultDetails.Name, chaosDetails.ChaosNamespace, "targeted", "pod", t.Name); err != nil {
				return stacktrace.Propagate(err, "could not annotate chaosresult")
			}
		}

		duration = int(time.Since(ChaosStartTimeStamp).Seconds())
	}
	return nil
}

func kill(experimentsDetails *experimentTypes.ExperimentDetails, containerIds []string, clients clients.ClientSets, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	// record the event inside chaosengine
	if experimentsDetails.EngineName != "" {
		msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on application pod"
		types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
		events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
	}

	switch experimentsDetails.ContainerRuntime {
	case "docker":
		if err := stopDockerContainer(containerIds, experimentsDetails.SocketPath, experimentsDetails.Signal, experimentsDetails.ChaosPodName); err != nil {
			return stacktrace.Propagate(err, "could not stop container")
		}
	case "containerd", "crio":
		if err := stopContainerdContainer(containerIds, experimentsDetails.SocketPath, experimentsDetails.Signal, experimentsDetails.ChaosPodName); err != nil {
			return stacktrace.Propagate(err, "could not stop container")
		}
	default:
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeHelper, Source: chaosDetails.ChaosPodName, Reason: fmt.Sprintf("unsupported container runtime %s", experimentsDetails.ContainerRuntime)}
	}
	return nil
}

func validate(t targetDetails, timeout, delay int, clients clients.ClientSets) error {
	//Check the status of restarted container
	if err := common.CheckContainerStatus(t.Namespace, t.Name, timeout, delay, clients, t.Source); err != nil {
		return err
	}

	// It will verify that the restart count of container should increase after chaos injection
	return verifyRestartCount(t, timeout, delay, clients, t.RestartCountBefore)
}

//stopContainerdContainer kill the application container
func stopContainerdContainer(containerIDs []string, socketPath, signal, source string) error {
	if signal != "SIGKILL" && signal != "SIGTERM" {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeHelper, Source: source, Reason: fmt.Sprintf("unsupported signal %s, use either SIGTERM or SIGKILL", signal)}
	}

	cmd := exec.Command("sudo", "crictl", "-i", fmt.Sprintf("unix://%s", socketPath), "-r", fmt.Sprintf("unix://%s", socketPath), "stop")
	if signal == "SIGKILL" {
		cmd.Args = append(cmd.Args, "--timeout=0")
	}
	cmd.Args = append(cmd.Args, containerIDs...)

	var errOut, out bytes.Buffer
	cmd.Stderr = &errOut
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosInject, Source: source, Reason: fmt.Sprintf("failed to stop container :%s", out.String())}
	}
	return nil
}

//stopDockerContainer kill the application container
func stopDockerContainer(containerIDs []string, socketPath, signal, source string) error {
	var errOut, out bytes.Buffer
	cmd := exec.Command("sudo", "docker", "--host", fmt.Sprintf("unix://%s", socketPath), "kill", "--signal", signal)
	cmd.Args = append(cmd.Args, containerIDs...)
	cmd.Stderr = &errOut
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosInject, Source: source, Reason: fmt.Sprintf("failed to stop container :%s", out.String())}
	}
	return nil
}

//getRestartCount return the restart count of target container
func getRestartCount(target targetDetails, clients clients.ClientSets) (int, error) {
	pod, err := clients.KubeClient.CoreV1().Pods(target.Namespace).Get(context.Background(), target.Name, v1.GetOptions{})
	if err != nil {
		return 0, cerrors.Error{ErrorCode: cerrors.ErrorTypeHelper, Source: target.Source, Target: fmt.Sprintf("{podName: %s, namespace: %s}", target.Name, target.Namespace), Reason: err.Error()}
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
				return cerrors.Error{ErrorCode: cerrors.ErrorTypeHelper, Source: t.Source, Target: fmt.Sprintf("{podName: %s, namespace: %s}", t.Name, t.Namespace), Reason: err.Error()}
			}
			for _, container := range pod.Status.ContainerStatuses {
				if container.Name == t.TargetContainer {
					restartCountAfter = int(container.RestartCount)
					break
				}
			}
			if restartCountAfter <= restartCountBefore {
				return cerrors.Error{ErrorCode: cerrors.ErrorTypeHelper, Source: t.Source, Target: fmt.Sprintf("{podName: %s, namespace: %s, container: %s}", t.Name, t.Namespace, t.TargetContainer), Reason: "target container is not restarted after kill"}
			}
			log.Infof("restartCount of target container after chaos injection: %v", strconv.Itoa(restartCountAfter))
			return nil
		})
}

//getENV fetches all the env variables from the runner pod
func getENV(experimentDetails *experimentTypes.ExperimentDetails) {
	experimentDetails.ExperimentName = types.Getenv("EXPERIMENT_NAME", "")
	experimentDetails.InstanceID = types.Getenv("INSTANCE_ID", "")
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
	Source             string
}
