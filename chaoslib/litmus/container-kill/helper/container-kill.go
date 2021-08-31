package helper

import (
	"bytes"
	"os/exec"
	"strconv"
	"time"

	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentEnv "github.com/litmuschaos/litmus-go/pkg/generic/container-kill/environment"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/container-kill/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/result"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/openebs/maya/pkg/util/retry"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientTypes "k8s.io/apimachinery/pkg/types"
)

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
	experimentEnv.InitialiseChaosVariables(&chaosDetails, &experimentsDetails)

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

	//ChaosStartTimeStamp contains the start timestamp, when the chaos injection begin
	ChaosStartTimeStamp := time.Now()
	duration := int(time.Since(ChaosStartTimeStamp).Seconds())

	for duration < experimentsDetails.ChaosDuration {

		//getRestartCount return the restart count of target container
		restartCountBefore, err := getRestartCount(experimentsDetails, experimentsDetails.TargetPods, clients)
		if err != nil {
			return err
		}

		//Obtain the container ID through Pod
		// this id will be used to select the container for the kill
		containerID, err := common.GetContainerID(experimentsDetails.AppNS, experimentsDetails.TargetPods, experimentsDetails.TargetContainer, clients)
		if err != nil {
			return errors.Errorf("Unable to get the container id, %v", err)
		}

		log.InfoWithValues("[Info]: Details of application under chaos injection", logrus.Fields{
			"PodName":            experimentsDetails.TargetPods,
			"ContainerName":      experimentsDetails.TargetContainer,
			"RestartCountBefore": restartCountBefore,
		})

		// record the event inside chaosengine
		if experimentsDetails.EngineName != "" {
			msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on application pod"
			types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngne")
		}

		switch experimentsDetails.ContainerRuntime {
		case "docker":
			if err := stopDockerContainer(containerID, experimentsDetails.SocketPath, experimentsDetails.Signal); err != nil {
				return err
			}
		case "containerd", "crio":
			if err := stopContainerdContainer(containerID, experimentsDetails.SocketPath, experimentsDetails.Signal); err != nil {
				return err
			}
		default:
			return errors.Errorf("%v container runtime not supported", experimentsDetails.ContainerRuntime)
		}

		//Waiting for the chaos interval after chaos injection
		if experimentsDetails.ChaosInterval != 0 {
			log.Infof("[Wait]: Wait for the chaos interval %vs", experimentsDetails.ChaosInterval)
			common.WaitForDuration(experimentsDetails.ChaosInterval)
		}

		//Check the status of restarted container
		err = common.CheckContainerStatus(experimentsDetails.AppNS, experimentsDetails.TargetPods, clients)
		if err != nil {
			return errors.Errorf("application container is not in running state, %v", err)
		}

		// It will verify that the restart count of container should increase after chaos injection
		err = verifyRestartCount(experimentsDetails, experimentsDetails.TargetPods, clients, restartCountBefore)
		if err != nil {
			return err
		}
		duration = int(time.Since(ChaosStartTimeStamp).Seconds())
	}
	if err := result.AnnotateChaosResult(resultDetails.Name, chaosDetails.ChaosNamespace, "targeted", "pod", experimentsDetails.TargetPods); err != nil {
		return err
	}
	log.Infof("[Completion]: %v chaos has been completed", experimentsDetails.ExperimentName)
	return nil
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
func getRestartCount(experimentsDetails *experimentTypes.ExperimentDetails, podName string, clients clients.ClientSets) (int, error) {
	pod, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).Get(podName, v1.GetOptions{})
	if err != nil {
		return 0, err
	}
	restartCount := 0
	for _, container := range pod.Status.ContainerStatuses {
		if container.Name == experimentsDetails.TargetContainer {
			restartCount = int(container.RestartCount)
			break
		}
	}
	return restartCount, nil
}

//verifyRestartCount verify the restart count of target container that it is restarted or not after chaos injection
func verifyRestartCount(experimentsDetails *experimentTypes.ExperimentDetails, podName string, clients clients.ClientSets, restartCountBefore int) error {

	restartCountAfter := 0
	return retry.
		Times(uint(experimentsDetails.Timeout / experimentsDetails.Delay)).
		Wait(time.Duration(experimentsDetails.Delay) * time.Second).
		Try(func(attempt uint) error {
			pod, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).Get(podName, v1.GetOptions{})
			if err != nil {
				return errors.Errorf("Unable to find the pod with name %v, err: %v", podName, err)
			}
			for _, container := range pod.Status.ContainerStatuses {
				if container.Name == experimentsDetails.TargetContainer {
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
	experimentDetails.ExperimentName = common.Getenv("EXPERIMENT_NAME", "")
	experimentDetails.InstanceID = common.Getenv("INSTANCE_ID", "")
	experimentDetails.AppNS = common.Getenv("APP_NS", "")
	experimentDetails.TargetContainer = common.Getenv("APP_CONTAINER", "")
	experimentDetails.TargetPods = common.Getenv("APP_POD", "")
	experimentDetails.ChaosDuration, _ = strconv.Atoi(common.Getenv("TOTAL_CHAOS_DURATION", "30"))
	experimentDetails.ChaosInterval, _ = strconv.Atoi(common.Getenv("CHAOS_INTERVAL", "10"))
	experimentDetails.ChaosNamespace = common.Getenv("CHAOS_NAMESPACE", "litmus")
	experimentDetails.EngineName = common.Getenv("CHAOS_ENGINE", "")
	experimentDetails.ChaosUID = clientTypes.UID(common.Getenv("CHAOS_UID", ""))
	experimentDetails.ChaosPodName = common.Getenv("POD_NAME", "")
	experimentDetails.SocketPath = common.Getenv("SOCKET_PATH", "")
	experimentDetails.ContainerRuntime = common.Getenv("CONTAINER_RUNTIME", "")
	experimentDetails.Signal = common.Getenv("SIGNAL", "SIGKILL")
	experimentDetails.Delay, _ = strconv.Atoi(common.Getenv("STATUS_CHECK_DELAY", "2"))
	experimentDetails.Timeout, _ = strconv.Atoi(common.Getenv("STATUS_CHECK_TIMEOUT", "180"))
}
