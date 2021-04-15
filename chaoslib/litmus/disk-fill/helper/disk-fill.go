package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentEnv "github.com/litmuschaos/litmus-go/pkg/generic/disk-fill/environment"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/disk-fill/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/result"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientTypes "k8s.io/apimachinery/pkg/types"
)

var inject, abort chan os.Signal

func main() {

	experimentsDetails := experimentTypes.ExperimentDetails{}
	clients := clients.ClientSets{}
	eventsDetails := types.EventDetails{}
	chaosDetails := types.ChaosDetails{}
	resultDetails := types.ResultDetails{}

	// inject channel is used to transmit signal notifications.
	inject = make(chan os.Signal, 1)
	// Catch and relay certain signal(s) to inject channel.
	signal.Notify(inject, os.Interrupt, syscall.SIGTERM)

	// abort channel is used to transmit signal notifications.
	abort = make(chan os.Signal, 1)
	// Catch and relay certain signal(s) to abort channel.
	signal.Notify(abort, os.Interrupt, syscall.SIGTERM)

	//Getting kubeConfig and Generate ClientSets
	if err := clients.GenerateClientSetFromKubeConfig(); err != nil {
		log.Fatalf("Unable to Get the kubeconfig, err: %v", err)
	}

	//Fetching all the ENV passed in the helper pod
	log.Info("[PreReq]: Getting the ENV variables")
	GetENV(&experimentsDetails, "disk-fill")

	// Intialise the chaos attributes
	experimentEnv.InitialiseChaosVariables(&chaosDetails, &experimentsDetails)

	// Intialise Chaos Result Parameters
	types.SetResultAttributes(&resultDetails, chaosDetails)

	// Set the chaos result uid
	result.SetResultUID(&resultDetails, clients, &chaosDetails)

	err := DiskFill(&experimentsDetails, clients, &eventsDetails, &chaosDetails, &resultDetails)
	if err != nil {
		log.Fatalf("helper pod failed, err: %v", err)
	}
}

//DiskFill contains steps to inject disk-fill chaos
func DiskFill(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails, resultDetails *types.ResultDetails) error {

	// Derive the container id of the target container
	containerID, err := GetContainerID(experimentsDetails, clients)
	if err != nil {
		return err
	}

	// derive the used ephemeral storage size from the target container
	du := fmt.Sprintf("sudo du /diskfill/%v", containerID)
	cmd := exec.Command("/bin/bash", "-c", du)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Error(string(out))
		return err
	}
	ephemeralStorageDetails := string(out)

	// filtering out the used ephemeral storage from the output of du command
	usedEphemeralStorageSize, err := FilterUsedEphemeralStorage(ephemeralStorageDetails)
	if err != nil {
		return errors.Errorf("Unable to filter used ephemeral storage size, err: %v", err)
	}
	log.Infof("used ephemeral storage space: %vKB", strconv.Itoa(usedEphemeralStorageSize))

	// GetEphemeralStorageAttributes derive the ephemeral storage attributes from the target container
	ephemeralStorageLimit, err := GetEphemeralStorageAttributes(experimentsDetails, clients)
	if err != nil {
		return err
	}

	if ephemeralStorageLimit == 0 && experimentsDetails.EphemeralStorageMebibytes == 0 {
		return errors.Errorf("Either provide ephemeral storage limit inside target container or define EPHEMERAL_STORAGE_MEBIBYTES ENV")
	}

	// deriving the ephemeral storage size to be filled
	sizeTobeFilled := GetSizeToBeFilled(experimentsDetails, usedEphemeralStorageSize, int(ephemeralStorageLimit))

	log.InfoWithValues("[Info]: Details of application under chaos injection", logrus.Fields{
		"PodName":                   experimentsDetails.TargetPods,
		"ContainerName":             experimentsDetails.TargetContainer,
		"ephemeralStorageLimit(KB)": ephemeralStorageLimit,
		"ContainerID":               containerID,
	})

	log.Infof("ephemeral storage size to be filled: %vKB", strconv.Itoa(sizeTobeFilled))

	// record the event inside chaosengine
	if experimentsDetails.EngineName != "" {
		msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on application pod"
		types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
		events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
	}

	// watching for the abort signal and revert the chaos
	go abortWatcher(experimentsDetails, clients, containerID)

	if sizeTobeFilled > 0 {

		if err := fillDisk(containerID, sizeTobeFilled); err != nil {
			log.Error(string(out))
			return err
		}

		log.Infof("[Chaos]: Waiting for %vs", experimentsDetails.ChaosDuration)

		common.WaitForDuration(experimentsDetails.ChaosDuration)

		log.Info("[Chaos]: Stopping the experiment")

		// It will delete the target pod if target pod is evicted
		// if target pod is still running then it will delete all the files, which was created earlier during chaos execution
		err = Remedy(experimentsDetails, clients, containerID)
		if err != nil {
			return errors.Errorf("Unable to perform remedy operation, err: %v", err)
		}
	} else {
		log.Warn("No required free space found!, It's Housefull")
	}
	return nil
}

// fillDisk fill the ephemeral disk by creating files
func fillDisk(containerID string, sizeTobeFilled int) error {

	select {
	case <-inject:
		// stopping the chaos execution, if abort signal recieved
		os.Exit(1)
	default:
		// Creating files to fill the required ephemeral storage size of block size of 4K
		log.Infof("[Fill]: Filling ephemeral storage, size: %vKB", sizeTobeFilled)
		dd := fmt.Sprintf("sudo dd if=/dev/urandom of=/diskfill/%v/diskfill bs=4K count=%v", containerID, strconv.Itoa(sizeTobeFilled/4))
		cmd := exec.Command("/bin/bash", "-c", dd)
		_, err := cmd.CombinedOutput()
		return err
	}
	return nil
}

// GetEphemeralStorageAttributes derive the ephemeral storage attributes from the target pod
func GetEphemeralStorageAttributes(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) (int64, error) {

	pod, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).Get(experimentsDetails.TargetPods, v1.GetOptions{})

	if err != nil {
		return 0, err
	}

	var ephemeralStorageLimit int64
	containers := pod.Spec.Containers

	// Extracting ephemeral storage limit & requested value from the target container
	// It will be in the form of Kb
	for _, container := range containers {
		if container.Name == experimentsDetails.TargetContainer {
			ephemeralStorageLimit = container.Resources.Limits.StorageEphemeral().ToDec().ScaledValue(resource.Kilo)
			break
		}
	}

	return ephemeralStorageLimit, nil
}

// GetContainerID derive the container id of the target container
func GetContainerID(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) (string, error) {
	pod, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).Get(experimentsDetails.TargetPods, v1.GetOptions{})

	if err != nil {
		return "", err
	}

	var containerID string
	containers := pod.Status.ContainerStatuses

	// filtering out the container id from the details of containers inside containerStatuses of the given pod
	// container id is present in the form of <runtime>://<container-id>
	for _, container := range containers {
		if container.Name == experimentsDetails.TargetContainer {
			containerID = strings.Split(container.ContainerID, "//")[1]
			break
		}
	}

	return containerID, nil

}

// FilterUsedEphemeralStorage filter out the used ephemeral storage from the given string
func FilterUsedEphemeralStorage(ephemeralStorageDetails string) (int, error) {

	// Filtering out the ephemeral storage size from the output of du command
	// It contains details of all subdirectories of target container
	ephemeralStorageAll := strings.Split(ephemeralStorageDetails, "\n")
	// It will return the details of main directory
	ephemeralStorageAllDiskFill := strings.Split(ephemeralStorageAll[len(ephemeralStorageAll)-2], "\t")[0]
	// type casting string to interger
	ephemeralStorageSize, err := strconv.Atoi(ephemeralStorageAllDiskFill)
	return ephemeralStorageSize, err

}

// GetSizeToBeFilled generate the ephemeral storage size need to be filled
func GetSizeToBeFilled(experimentsDetails *experimentTypes.ExperimentDetails, usedEphemeralStorageSize int, ephemeralStorageLimit int) int {
	var requirementToBeFill int

	switch ephemeralStorageLimit {
	case 0:
		requirementToBeFill = experimentsDetails.EphemeralStorageMebibytes * 1024
	default:
		// deriving size need to be filled from the used size & requirement size to fill
		requirementToBeFill = (ephemeralStorageLimit * experimentsDetails.FillPercentage) / 100
	}

	needToBeFilled := requirementToBeFill - usedEphemeralStorageSize
	return needToBeFilled
}

// Remedy will delete the target pod if target pod is evicted
// if target pod is still running then it will delete the files, which was created during chaos execution
func Remedy(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, containerID string) error {
	pod, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).Get(experimentsDetails.TargetPods, v1.GetOptions{})
	if err != nil {
		return err
	}
	// Deleting the pod as pod is already evicted
	podReason := pod.Status.Reason
	if podReason == "Evicted" {
		log.Warn("Target pod is evicted, deleting the pod")
		if err := clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).Delete(experimentsDetails.TargetPods, &v1.DeleteOptions{}); err != nil {
			return err
		}
	} else {

		// deleting the files after chaos execution
		rm := fmt.Sprintf("sudo rm -rf /diskfill/%v/diskfill", containerID)
		cmd := exec.Command("/bin/bash", "-c", rm)
		out, err := cmd.CombinedOutput()
		if err != nil {
			log.Error(string(out))
			return err
		}
	}
	return nil
}

//GetENV fetches all the env variables from the runner pod
func GetENV(experimentDetails *experimentTypes.ExperimentDetails, name string) {
	experimentDetails.ExperimentName = name
	experimentDetails.AppNS = Getenv("APP_NS", "")
	experimentDetails.TargetContainer = Getenv("APP_CONTAINER", "")
	experimentDetails.TargetPods = Getenv("APP_POD", "")
	experimentDetails.ChaosDuration, _ = strconv.Atoi(Getenv("TOTAL_CHAOS_DURATION", "30"))
	experimentDetails.ChaosNamespace = Getenv("CHAOS_NAMESPACE", "litmus")
	experimentDetails.EngineName = Getenv("CHAOS_ENGINE", "")
	experimentDetails.ChaosUID = clientTypes.UID(Getenv("CHAOS_UID", ""))
	experimentDetails.ChaosPodName = Getenv("POD_NAME", "")
	experimentDetails.FillPercentage, _ = strconv.Atoi(Getenv("FILL_PERCENTAGE", ""))
	experimentDetails.EphemeralStorageMebibytes, _ = strconv.Atoi(Getenv("EPHEMERAL_STORAGE_MEBIBYTES", ""))
}

// Getenv fetch the env and set the default value, if any
func Getenv(key string, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		value = defaultValue
	}
	return value
}

// abortWatcher continuosly watch for the abort signals
func abortWatcher(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, containerID string) {

	for {
		select {
		case <-abort:
			log.Info("[Chaos]: Killing process started because of terminated signal received")
			log.Info("Chaos Revert Started")
			// retry thrice for the chaos revert
			retry := 3
			for retry > 0 {
				if err := Remedy(experimentsDetails, clients, containerID); err != nil {
					log.Errorf("Unable to perform remedy operation, err: %v", err)
				}
				retry--
				time.Sleep(1 * time.Second)
			}
			log.Info("Chaos Revert Completed")
			os.Exit(1)
		}
	}
}
