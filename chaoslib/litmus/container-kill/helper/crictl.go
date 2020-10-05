package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentEnv "github.com/litmuschaos/litmus-go/pkg/generic/container-kill/environment"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/container-kill/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/openebs/maya/pkg/util/retry"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientTypes "k8s.io/apimachinery/pkg/types"
)

func main() {

	experimentsDetails := experimentTypes.ExperimentDetails{}
	clients := clients.ClientSets{}
	eventsDetails := types.EventDetails{}
	chaosDetails := types.ChaosDetails{}

	//Getting kubeConfig and Generate ClientSets
	if err := clients.GenerateClientSetFromKubeConfig(); err != nil {
		log.Fatalf("Unable to Get the kubeconfig, err: %v", err)
	}

	//Fetching all the ENV passed in the helper pod
	log.Info("[PreReq]: Getting the ENV variables")
	GetENV(&experimentsDetails, "container-kill")

	// Intialise the chaos attributes
	experimentEnv.InitialiseChaosVariables(&chaosDetails, &experimentsDetails)

	err := KillContainer(&experimentsDetails, clients, &eventsDetails, &chaosDetails)
	if err != nil {
		log.Fatalf("helper pod failed, err: %v", err)
	}

}

// KillContainer kill the random application container
// it will kill the container till the chaos duration
// the execution will stop after timestamp passes the given chaos duration
func KillContainer(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	// getting the current timestamp, it will help to kepp track the total chaos duration
	ChaosStartTimeStamp := time.Now().Unix()

	for iteration := 0; iteration < experimentsDetails.Iterations; iteration++ {

		//Obtain the pod ID of the application pod
		podID, err := GetPodID(experimentsDetails)
		if err != nil {
			return errors.Errorf("Unable to get the pod id, %v", err)
		}

		//GetRestartCount return the restart count of target container
		restartCountBefore, err := GetRestartCount(experimentsDetails, experimentsDetails.TargetPod, clients)
		if err != nil {
			return err
		}

		//Obtain the container ID through Pod
		// this id will be used to select the container for kill
		containerID, err := GetContainerID(experimentsDetails, podID)
		if err != nil {
			return errors.Errorf("Unable to get the container id, %v", err)
		}

		log.InfoWithValues("[Info]: Details of application under chaos injection", logrus.Fields{
			"PodName":            experimentsDetails.TargetPod,
			"ContainerName":      experimentsDetails.TargetContainer,
			"RestartCountBefore": restartCountBefore,
		})

		// record the event inside chaosengine
		if experimentsDetails.EngineName != "" {
			msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on application pod"
			types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngne")
		}

		// killing the application container
		StopContainer(containerID)

		//Waiting for the chaos interval after chaos injection
		if experimentsDetails.ChaosInterval != 0 {
			log.Infof("[Wait]: Wait for the chaos interval %vs", experimentsDetails.ChaosInterval)
			waitForChaosInterval(experimentsDetails)
		}

		//Check the status of restarted container
		err = CheckContainerStatus(experimentsDetails, clients, experimentsDetails.TargetPod)
		if err != nil {
			return errors.Errorf("Application container is not in running state, %v", err)
		}

		// It will verify that the restart count of container should increase after chaos injection
		err = VerifyRestartCount(experimentsDetails, experimentsDetails.TargetPod, clients, restartCountBefore)
		if err != nil {
			return err
		}

		// generating the total duration of the experiment run
		ChaosCurrentTimeStamp := time.Now().Unix()
		chaosDiffTimeStamp := ChaosCurrentTimeStamp - ChaosStartTimeStamp

		// terminating the execution after the timestamp exceed the total chaos duration
		if int(chaosDiffTimeStamp) >= experimentsDetails.ChaosDuration {
			break
		}

	}
	log.Infof("[Completion]: %v chaos has been completed", experimentsDetails.ExperimentName)
	return nil

}

//GetPodID derive the pod-id of the application pod
func GetPodID(experimentsDetails *experimentTypes.ExperimentDetails) (string, error) {

	cmd := exec.Command("crictl", "pods")
	stdout, _ := cmd.Output()

	pods := RemoveExtraSpaces(stdout)
	for i := 0; i < len(pods)-1; i++ {
		attributes := strings.Split(pods[i], " ")
		if attributes[3] == experimentsDetails.TargetPod {
			return attributes[0], nil
		}

	}

	return "", fmt.Errorf("%v pod is unavailable", experimentsDetails.TargetPod)
}

//GetContainerID  derive the container id of the application container
func GetContainerID(experimentsDetails *experimentTypes.ExperimentDetails, podID string) (string, error) {

	cmd := exec.Command("crictl", "ps")
	stdout, _ := cmd.Output()
	containers := RemoveExtraSpaces(stdout)

	for i := 0; i < len(containers)-1; i++ {
		attributes := strings.Split(containers[i], " ")
		if attributes[4] == experimentsDetails.TargetContainer && attributes[6] == podID {
			return attributes[0], nil
		}

	}

	return "", fmt.Errorf("%v container is unavailable", experimentsDetails.TargetContainer)

}

//StopContainer kill the application container
func StopContainer(containerID string) {

	cmd := exec.Command("crictl", "stop", string(containerID))
	stdout, _ := cmd.Output()
	fmt.Print(string(stdout))
}

// CheckContainerStatus checks the status of the application container
func CheckContainerStatus(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, appName string) error {
	err := retry.
		Times(90).
		Wait(2 * time.Second).
		Try(func(attempt uint) error {
			pod, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).Get(appName, v1.GetOptions{})
			if err != nil {
				return errors.Errorf("Unable to find the pod with name %v, err: %v", appName, err)
			}
			for _, container := range pod.Status.ContainerStatuses {
				if container.Ready != true {
					return errors.Errorf("containers are not yet in running state")
				}
				log.InfoWithValues("The running status of container are as follows", logrus.Fields{
					"container": container.Name, "Pod": pod.Name, "Status": pod.Status.Phase})
			}

			return nil
		})
	if err != nil {
		return err
	}
	return nil
}

// RemoveExtraSpaces remove all the extra spaces present in output of crictl commands
func RemoveExtraSpaces(arr []byte) []string {
	bytesSlice := make([]byte, len(arr))
	index := 0
	count := 0
	for i := 0; i < len(arr); i++ {
		count = 0
		for arr[i] == 32 {
			count++
			i++
			if i >= len(arr) {
				break
			}
		}
		if count > 1 {
			bytesSlice[index] = 32
			index++
		}
		bytesSlice[index] = arr[i]
		index++

	}
	return strings.Split(string(bytesSlice), "\n")
}

//waitForChaosInterval waits for the given ramp time duration (in seconds)
func waitForChaosInterval(experimentsDetails *experimentTypes.ExperimentDetails) {
	time.Sleep(time.Duration(experimentsDetails.ChaosInterval) * time.Second)
}

//GetRestartCount return the restart count of target container
func GetRestartCount(experimentsDetails *experimentTypes.ExperimentDetails, podName string, clients clients.ClientSets) (int, error) {
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

//VerifyRestartCount verify the restart count of target container that it is restarted or not after chaos injection
func VerifyRestartCount(experimentsDetails *experimentTypes.ExperimentDetails, podName string, clients clients.ClientSets, restartCountBefore int) error {

	restartCountAfter := 0
	err := retry.
		Times(90).
		Wait(1 * time.Second).
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
			return nil
		})

	log.Infof("restartCount of target container after chaos injection: %v", strconv.Itoa(restartCountAfter))

	return err

}

//GetENV fetches all the env variables from the runner pod
func GetENV(experimentDetails *experimentTypes.ExperimentDetails, name string) {
	experimentDetails.ExperimentName = name
	experimentDetails.AppNS = Getenv("APP_NS", "")
	experimentDetails.TargetContainer = Getenv("APP_CONTAINER", "")
	experimentDetails.TargetPod = Getenv("APP_POD", "")
	experimentDetails.ChaosDuration, _ = strconv.Atoi(Getenv("TOTAL_CHAOS_DURATION", "30"))
	experimentDetails.ChaosInterval, _ = strconv.Atoi(Getenv("CHAOS_INTERVAL", "10"))
	experimentDetails.Iterations, _ = strconv.Atoi(Getenv("ITERATIONS", "3"))
	experimentDetails.ChaosNamespace = Getenv("CHAOS_NAMESPACE", "litmus")
	experimentDetails.EngineName = Getenv("CHAOS_ENGINE", "")
	experimentDetails.AppLabel = Getenv("APP_LABEL", "")
	experimentDetails.ChaosUID = clientTypes.UID(Getenv("CHAOS_UID", ""))
	experimentDetails.ChaosPodName = Getenv("POD_NAME", "")
}

// Getenv fetch the env and set the default value, if any
func Getenv(key string, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		value = defaultValue
	}
	return value
}
