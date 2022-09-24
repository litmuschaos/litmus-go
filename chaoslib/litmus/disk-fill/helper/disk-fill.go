package helper

import (
	"context"
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

var err error

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
	types.InitialiseChaosVariables(&chaosDetails)

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

	targetList, err := common.ParseTargets()
	if err != nil {
		return err
	}

	var targets []targetDetails

	for _, t := range targetList.Target {
		td := targetDetails{
			Name:            t.Name,
			Namespace:       t.Namespace,
			TargetContainer: t.TargetContainer,
		}

		// Derive the container id of the target container
		td.ContainerId, err = common.GetContainerID(td.Namespace, td.Name, td.TargetContainer, clients)
		if err != nil {
			return err
		}

		td.SizeToFill, err = getDiskSizeToFill(td, experimentsDetails, clients)
		if err != nil {
			return err
		}

		log.InfoWithValues("[Info]: Details of application under chaos injection", logrus.Fields{
			"PodName":         td.Name,
			"Namespace":       td.Namespace,
			"SizeToFill(KB)":  td.SizeToFill,
			"TargetContainer": td.TargetContainer,
		})

		targets = append(targets, td)
	}

	// record the event inside chaosengine
	if experimentsDetails.EngineName != "" {
		msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on application pod"
		types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
		events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
	}

	// watching for the abort signal and revert the chaos
	go abortWatcher(targets, experimentsDetails, clients, resultDetails.Name)

	select {
	case <-inject:
		// stopping the chaos execution, if abort signal received
		os.Exit(1)
	default:
	}

	for _, t := range targets {
		if t.SizeToFill > 0 {
			if err := fillDisk(t.ContainerId, t.SizeToFill, experimentsDetails.DataBlockSize); err != nil {
				return err
			}

			if err = result.AnnotateChaosResult(resultDetails.Name, chaosDetails.ChaosNamespace, "injected", "pod", t.Name); err != nil {
				if remedyErr := remedy(t, clients); remedyErr != nil {
					return remedyErr
				}
				return err
			}
		} else {
			log.Warn("No required free space found!")
		}
	}

	log.Infof("[Chaos]: Waiting for %vs", experimentsDetails.ChaosDuration)

	common.WaitForDuration(experimentsDetails.ChaosDuration)

	log.Info("[Chaos]: Stopping the experiment")

	var errList []string

	for _, t := range targets {
		// It will delete the target pod if target pod is evicted
		// if target pod is still running then it will delete all the files, which was created earlier during chaos execution
		err = remedy(t, clients)
		if err != nil {
			errList = append(errList, err.Error())
			continue
		}
		if err = result.AnnotateChaosResult(resultDetails.Name, chaosDetails.ChaosNamespace, "reverted", "pod", t.Name); err != nil {
			errList = append(errList, err.Error())
		}
	}

	if len(errList) != 0 {
		return fmt.Errorf("failed to revert chaos, err: %v", strings.Join(errList, ","))
	}

	return nil
}

// fillDisk fill the ephemeral disk by creating files
func fillDisk(containerID string, sizeTobeFilled, bs int) error {

	// Creating files to fill the required ephemeral storage size of block size of 4K
	log.Infof("[Fill]: Filling ephemeral storage, size: %vKB", sizeTobeFilled)
	dd := fmt.Sprintf("sudo dd if=/dev/urandom of=/diskfill/%v/diskfill bs=%vK count=%v", containerID, bs, strconv.Itoa(sizeTobeFilled/bs))
	log.Infof("dd: {%v}", dd)
	cmd := exec.Command("/bin/bash", "-c", dd)
	_, err := cmd.CombinedOutput()
	return err
}

// getEphemeralStorageAttributes derive the ephemeral storage attributes from the target pod
func getEphemeralStorageAttributes(t targetDetails, clients clients.ClientSets) (int64, error) {

	pod, err := clients.KubeClient.CoreV1().Pods(t.Namespace).Get(context.Background(), t.Name, v1.GetOptions{})
	if err != nil {
		return 0, err
	}

	var ephemeralStorageLimit int64
	containers := pod.Spec.Containers

	// Extracting ephemeral storage limit & requested value from the target container
	// It will be in the form of Kb
	for _, container := range containers {
		if container.Name == t.TargetContainer {
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
		ephemeralStorageMebibytes, _ := strconv.Atoi(experimentsDetails.EphemeralStorageMebibytes)
		requirementToBeFill = ephemeralStorageMebibytes * 1024
	default:
		// deriving size need to be filled from the used size & requirement size to fill
		fillPercentage, _ := strconv.Atoi(experimentsDetails.FillPercentage)
		requirementToBeFill = (ephemeralStorageLimit * fillPercentage) / 100
	}

	needToBeFilled := requirementToBeFill - usedEphemeralStorageSize
	return needToBeFilled
}

// remedy will delete the target pod if target pod is evicted
// if target pod is still running then it will delete the files, which was created during chaos execution
func remedy(t targetDetails, clients clients.ClientSets) error {
	pod, err := clients.KubeClient.CoreV1().Pods(t.Namespace).Get(context.Background(), t.Name, v1.GetOptions{})
	if err != nil {
		return err
	}
	// Deleting the pod as pod is already evicted
	podReason := pod.Status.Reason
	if podReason == "Evicted" {
		log.Warn("Target pod is evicted, deleting the pod")
		if err := clients.KubeClient.CoreV1().Pods(t.Namespace).Delete(context.Background(), t.Name, v1.DeleteOptions{}); err != nil {
			return err
		}
	} else {
		// deleting the files after chaos execution
		rm := fmt.Sprintf("sudo rm -rf /diskfill/%v/diskfill", t.ContainerId)
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
	experimentDetails.ExperimentName = types.Getenv("EXPERIMENT_NAME", "")
	experimentDetails.InstanceID = types.Getenv("INSTANCE_ID", "")
	experimentDetails.ChaosDuration, _ = strconv.Atoi(types.Getenv("TOTAL_CHAOS_DURATION", "30"))
	experimentDetails.ChaosNamespace = types.Getenv("CHAOS_NAMESPACE", "litmus")
	experimentDetails.EngineName = types.Getenv("CHAOSENGINE", "")
	experimentDetails.ChaosUID = clientTypes.UID(types.Getenv("CHAOS_UID", ""))
	experimentDetails.ChaosPodName = types.Getenv("POD_NAME", "")
	experimentDetails.FillPercentage = types.Getenv("FILL_PERCENTAGE", "")
	experimentDetails.EphemeralStorageMebibytes = types.Getenv("EPHEMERAL_STORAGE_MEBIBYTES", "")
	experimentDetails.DataBlockSize, _ = strconv.Atoi(types.Getenv("DATA_BLOCK_SIZE", "256"))
}

// abortWatcher continuously watch for the abort signals
func abortWatcher(targets []targetDetails, experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultName string) {
	// waiting till the abort signal received
	<-abort

	log.Info("[Chaos]: Killing process started because of terminated signal received")
	log.Info("Chaos Revert Started")
	// retry thrice for the chaos revert
	retry := 3
	for retry > 0 {
		for _, t := range targets {
			err := remedy(t, clients)
			if err != nil {
				log.Errorf("unable to kill disk-fill process, err :%v", err)
				continue
			}
			if err = result.AnnotateChaosResult(resultName, experimentsDetails.ChaosNamespace, "reverted", "pod", t.Name); err != nil {
				log.Errorf("unable to annotate the chaosresult, err :%v", err)
			}
		}
		retry--
		time.Sleep(1 * time.Second)
	}
	log.Info("Chaos Revert Completed")
	os.Exit(1)
}

func getDiskSizeToFill(t targetDetails, experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) (int, error) {
	usedEphemeralStorageSize, err := getUsedEphemeralStorage(t.ContainerId)

	// GetEphemeralStorageAttributes derive the ephemeral storage attributes from the target container
	ephemeralStorageLimit, err := getEphemeralStorageAttributes(t, clients)
	if err != nil {
		return 0, err
	}

	if ephemeralStorageLimit == 0 && experimentsDetails.EphemeralStorageMebibytes == "0" {
		return 0, errors.Errorf("either provide ephemeral storage limit inside target container or define EPHEMERAL_STORAGE_MEBIBYTES ENV")
	}

	// deriving the ephemeral storage size to be filled
	sizeTobeFilled := getSizeToBeFilled(experimentsDetails, usedEphemeralStorageSize, int(ephemeralStorageLimit))

	return sizeTobeFilled, nil
}

func getUsedEphemeralStorage(containerId string) (int, error) {
	// derive the used ephemeral storage size from the target container
	du := fmt.Sprintf("sudo du /diskfill/%v", containerId)
	cmd := exec.Command("/bin/bash", "-c", du)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Error(string(out))
		return 0, err
	}
	ephemeralStorageDetails := string(out)

	// filtering out the used ephemeral storage from the output of du command
	usedEphemeralStorageSize, err := filterUsedEphemeralStorage(ephemeralStorageDetails)
	if err != nil {
		return 0, errors.Errorf("unable to filter used ephemeral storage size, err: %v", err)
	}
	log.Infof("used ephemeral storage space: %vKB", strconv.Itoa(usedEphemeralStorageSize))
	return usedEphemeralStorageSize, nil
}

type targetDetails struct {
	Name            string
	Namespace       string
	TargetContainer string
	ContainerId     string
	SizeToFill      int
}
