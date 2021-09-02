package helper

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

// Helper injects the disk-fill chaos
func Helper(clients clients.ClientSets) {

	experimentsDetails := experimentTypes.ExperimentDetails{}
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

	//Fetching all the ENV passed in the helper pod
	log.Info("[PreReq]: Getting the ENV variables")
	getENV(&experimentsDetails)

	// Intialise the chaos attributes
	experimentEnv.InitialiseChaosVariables(&chaosDetails, &experimentsDetails)

	// Intialise Chaos Result Parameters
	types.SetResultAttributes(&resultDetails, chaosDetails)

	// Set the chaos result uid
	result.SetResultUID(&resultDetails, clients, &chaosDetails)

	if err := diskFill(&experimentsDetails, clients, &eventsDetails, &chaosDetails, &resultDetails); err != nil {
		log.Fatalf("helper pod failed, err: %v", err)
	}
}

//diskFill contains steps to inject disk-fill chaos
func diskFill(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails, resultDetails *types.ResultDetails) error {

	// Derive the container id of the target container
	containerID, err := common.GetContainerID(experimentsDetails.AppNS, experimentsDetails.TargetPods, experimentsDetails.TargetContainer, clients)
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
	usedEphemeralStorageSize, err := filterUsedEphemeralStorage(ephemeralStorageDetails)
	if err != nil {
		return errors.Errorf("unable to filter used ephemeral storage size, err: %v", err)
	}
	log.Infof("used ephemeral storage space: %vKB", strconv.Itoa(usedEphemeralStorageSize))

	// GetEphemeralStorageAttributes derive the ephemeral storage attributes from the target container
	ephemeralStorageLimit, err := getEphemeralStorageAttributes(experimentsDetails, clients)
	if err != nil {
		return err
	}

	if ephemeralStorageLimit == 0 && experimentsDetails.EphemeralStorageMebibytes == 0 {
		return errors.Errorf("either provide ephemeral storage limit inside target container or define EPHEMERAL_STORAGE_MEBIBYTES ENV")
	}

	// deriving the ephemeral storage size to be filled
	sizeTobeFilled := getSizeToBeFilled(experimentsDetails, usedEphemeralStorageSize, int(ephemeralStorageLimit))

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
	go abortWatcher(experimentsDetails, clients, containerID, resultDetails.Name)

	if sizeTobeFilled > 0 {

		if err := fillDisk(containerID, sizeTobeFilled, experimentsDetails.DataBlockSize); err != nil {
			log.Error(string(out))
			return err
		}

		if err = result.AnnotateChaosResult(resultDetails.Name, chaosDetails.ChaosNamespace, "injected", "pod", experimentsDetails.TargetPods); err != nil {
			return err
		}

		log.Infof("[Chaos]: Waiting for %vs", experimentsDetails.ChaosDuration)

		common.WaitForDuration(experimentsDetails.ChaosDuration)

		log.Info("[Chaos]: Stopping the experiment")

		// It will delete the target pod if target pod is evicted
		// if target pod is still running then it will delete all the files, which was created earlier during chaos execution
		err = remedy(experimentsDetails, clients, containerID)
		if err != nil {
			return errors.Errorf("unable to perform remedy operation, err: %v", err)
		}
		if err = result.AnnotateChaosResult(resultDetails.Name, chaosDetails.ChaosNamespace, "reverted", "pod", experimentsDetails.TargetPods); err != nil {
			return err
		}
	} else {
		log.Warn("No required free space found!, It's Housefull")
	}
	return nil
}

// fillDisk fill the ephemeral disk by creating files
func fillDisk(containerID string, sizeTobeFilled, bs int) error {

	select {
	case <-inject:
		// stopping the chaos execution, if abort signal recieved
		os.Exit(1)
	default:
		// Creating files to fill the required ephemeral storage size of block size of 4K
		log.Infof("[Fill]: Filling ephemeral storage, size: %vKB", sizeTobeFilled)
		dd := fmt.Sprintf("sudo dd if=/dev/urandom of=/diskfill/%v/diskfill bs=%vK count=%v", containerID, bs, strconv.Itoa(sizeTobeFilled/bs))
		log.Infof("dd: {%v}", dd)
		cmd := exec.Command("/bin/bash", "-c", dd)
		_, err := cmd.CombinedOutput()
		return err
	}
	return nil
}

// getEphemeralStorageAttributes derive the ephemeral storage attributes from the target pod
func getEphemeralStorageAttributes(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) (int64, error) {

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

// filterUsedEphemeralStorage filter out the used ephemeral storage from the given string
func filterUsedEphemeralStorage(ephemeralStorageDetails string) (int, error) {

	// Filtering out the ephemeral storage size from the output of du command
	// It contains details of all subdirectories of target container
	ephemeralStorageAll := strings.Split(ephemeralStorageDetails, "\n")
	// It will return the details of main directory
	ephemeralStorageAllDiskFill := strings.Split(ephemeralStorageAll[len(ephemeralStorageAll)-2], "\t")[0]
	// type casting string to interger
	ephemeralStorageSize, err := strconv.Atoi(ephemeralStorageAllDiskFill)
	return ephemeralStorageSize, err
}

// getSizeToBeFilled generate the ephemeral storage size need to be filled
func getSizeToBeFilled(experimentsDetails *experimentTypes.ExperimentDetails, usedEphemeralStorageSize int, ephemeralStorageLimit int) int {
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

// remedy will delete the target pod if target pod is evicted
// if target pod is still running then it will delete the files, which was created during chaos execution
func remedy(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, containerID string) error {
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

//getENV fetches all the env variables from the runner pod
func getENV(experimentDetails *experimentTypes.ExperimentDetails) {
	experimentDetails.ExperimentName = common.Getenv("EXPERIMENT_NAME", "")
	experimentDetails.InstanceID = common.Getenv("INSTANCE_ID", "")
	experimentDetails.AppNS = common.Getenv("APP_NS", "")
	experimentDetails.TargetContainer = common.Getenv("APP_CONTAINER", "")
	experimentDetails.TargetPods = common.Getenv("APP_POD", "")
	experimentDetails.ChaosDuration, _ = strconv.Atoi(common.Getenv("TOTAL_CHAOS_DURATION", "30"))
	experimentDetails.ChaosNamespace = common.Getenv("CHAOS_NAMESPACE", "litmus")
	experimentDetails.EngineName = common.Getenv("CHAOS_ENGINE", "")
	experimentDetails.ChaosUID = clientTypes.UID(common.Getenv("CHAOS_UID", ""))
	experimentDetails.ChaosPodName = common.Getenv("POD_NAME", "")
	experimentDetails.FillPercentage, _ = strconv.Atoi(common.Getenv("FILL_PERCENTAGE", ""))
	experimentDetails.EphemeralStorageMebibytes, _ = strconv.Atoi(common.Getenv("EPHEMERAL_STORAGE_MEBIBYTES", ""))
	experimentDetails.DataBlockSize, _ = strconv.Atoi(common.Getenv("DATA_BLOCK_SIZE", "256"))
}

// abortWatcher continuosly watch for the abort signals
func abortWatcher(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, containerID, resultName string) {
	// waiting till the abort signal recieved
	<-abort

	log.Info("[Chaos]: Killing process started because of terminated signal received")
	log.Info("Chaos Revert Started")
	// retry thrice for the chaos revert
	retry := 3
	for retry > 0 {
		if err := remedy(experimentsDetails, clients, containerID); err != nil {
			log.Errorf("unable to perform remedy operation, err: %v", err)
		}
		retry--
		time.Sleep(1 * time.Second)
	}
	if err := result.AnnotateChaosResult(resultName, experimentsDetails.ChaosNamespace, "reverted", "pod", experimentsDetails.TargetPods); err != nil {
		log.Errorf("unable to annotate the chaosresult, err :%v", err)
	}
	log.Info("Chaos Revert Completed")
	os.Exit(1)
}
