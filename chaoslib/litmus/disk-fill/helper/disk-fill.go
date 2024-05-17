package helper

import (
	"context"
	"fmt"
	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/palantir/stacktrace"
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
	types.InitialiseChaosVariables(&chaosDetails)
	chaosDetails.Phase = types.ChaosInjectPhase

	// Intialise Chaos Result Parameters
	types.SetResultAttributes(&resultDetails, chaosDetails)

	// Set the chaos result uid
	result.SetResultUID(&resultDetails, clients, &chaosDetails)

	if err := diskFill(&experimentsDetails, clients, &eventsDetails, &chaosDetails, &resultDetails); err != nil {
		// update failstep inside chaosresult
		if resultErr := result.UpdateFailedStepFromHelper(&resultDetails, &chaosDetails, clients, err); resultErr != nil {
			log.Fatalf("helper pod failed, err: %v, resultErr: %v", err, resultErr)
		}
		log.Fatalf("helper pod failed, err: %v", err)
	}
}

// diskFill contains steps to inject disk-fill chaos
func diskFill(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails, resultDetails *types.ResultDetails) error {

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

		// Derive the container id of the target container
		td.ContainerId, err = common.GetContainerID(td.Namespace, td.Name, td.TargetContainer, clients, chaosDetails.ChaosPodName)
		if err != nil {
			return stacktrace.Propagate(err, "could not get container id")
		}

		// extract out the pid of the target container
		td.TargetPID, err = common.GetPID(experimentsDetails.ContainerRuntime, td.ContainerId, experimentsDetails.SocketPath, td.Source)
		if err != nil {
			return err
		}

		td.SizeToFill, err = getDiskSizeToFill(td, experimentsDetails, clients)
		if err != nil {
			return stacktrace.Propagate(err, "could not get disk size to fill")
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
			if err := fillDisk(t, experimentsDetails.DataBlockSize); err != nil {
				return stacktrace.Propagate(err, "could not fill ephemeral storage")
			}
			log.Infof("successfully injected chaos on target: {name: %s, namespace: %v, container: %v}", t.Name, t.Namespace, t.TargetContainer)
			if err = result.AnnotateChaosResult(resultDetails.Name, chaosDetails.ChaosNamespace, "injected", "pod", t.Name); err != nil {
				if revertErr := revertDiskFill(t, clients); revertErr != nil {
					return cerrors.PreserveError{ErrString: fmt.Sprintf("[%s,%s]", stacktrace.RootCause(err).Error(), stacktrace.RootCause(revertErr).Error())}
				}
				return stacktrace.Propagate(err, "could not annotate chaosresult")
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
		if err = revertDiskFill(t, clients); err != nil {
			errList = append(errList, err.Error())
			continue
		}
		if err = result.AnnotateChaosResult(resultDetails.Name, chaosDetails.ChaosNamespace, "reverted", "pod", t.Name); err != nil {
			errList = append(errList, err.Error())
		}
	}

	if len(errList) != 0 {
		return cerrors.PreserveError{ErrString: fmt.Sprintf("[%s]", strings.Join(errList, ","))}
	}
	return nil
}

// fillDisk fill the ephemeral disk by creating files
func fillDisk(t targetDetails, bs int) error {

	// Creating files to fill the required ephemeral storage size of block size of 4K
	log.Infof("[Fill]: Filling ephemeral storage, size: %vKB", t.SizeToFill)
	dd := fmt.Sprintf("sudo dd if=/dev/urandom of=/proc/%v/root/home/diskfill bs=%vK count=%v", t.TargetPID, bs, strconv.Itoa(t.SizeToFill/bs))
	log.Infof("dd: {%v}", dd)
	cmd := exec.Command("/bin/bash", "-c", dd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Error(err.Error())
	}
	return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosInject, Source: t.Source, Target: fmt.Sprintf("{podName: %s, namespace: %s, container: %s}", t.Name, t.Namespace, t.TargetContainer), Reason: string(out)}
}

// getEphemeralStorageAttributes derive the ephemeral storage attributes from the target pod
func getEphemeralStorageAttributes(t targetDetails, clients clients.ClientSets) (int64, error) {

	pod, err := clients.KubeClient.CoreV1().Pods(t.Namespace).Get(context.Background(), t.Name, v1.GetOptions{})
	if err != nil {
		return 0, cerrors.Error{ErrorCode: cerrors.ErrorTypeHelper, Source: t.Source, Target: fmt.Sprintf("{podName: %s, namespace: %s}", t.Name, t.Namespace), Reason: err.Error()}
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
	// type casting string to integer
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

// revertDiskFill will delete the target pod if target pod is evicted
// if target pod is still running then it will delete the files, which was created during chaos execution
func revertDiskFill(t targetDetails, clients clients.ClientSets) error {
	pod, err := clients.KubeClient.CoreV1().Pods(t.Namespace).Get(context.Background(), t.Name, v1.GetOptions{})
	if err != nil {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosRevert, Source: t.Source, Target: fmt.Sprintf("{podName: %s,namespace: %s}", t.Name, t.Namespace), Reason: err.Error()}
	}
	podReason := pod.Status.Reason
	if podReason == "Evicted" {
		// Deleting the pod as pod is already evicted
		log.Warn("Target pod is evicted, deleting the pod")
		if err := clients.KubeClient.CoreV1().Pods(t.Namespace).Delete(context.Background(), t.Name, v1.DeleteOptions{}); err != nil {
			return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosRevert, Source: t.Source, Target: fmt.Sprintf("{podName: %s,namespace: %s}", t.Name, t.Namespace), Reason: fmt.Sprintf("failed to delete target pod after eviction :%s", err.Error())}
		}
	} else {
		// deleting the files after chaos execution
		rm := fmt.Sprintf("sudo rm -rf /proc/%v/root/home/diskfill", t.TargetPID)
		cmd := exec.Command("/bin/bash", "-c", rm)
		out, err := cmd.CombinedOutput()
		if err != nil {
			log.Error(err.Error())
			return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosRevert, Source: t.Source, Target: fmt.Sprintf("{podName: %s,namespace: %s}", t.Name, t.Namespace), Reason: fmt.Sprintf("failed to cleanup ephemeral storage: %s", string(out))}
		}
	}
	log.Infof("successfully reverted chaos on target: {name: %s, namespace: %v, container: %v}", t.Name, t.Namespace, t.TargetContainer)
	return nil
}

// getENV fetches all the env variables from the runner pod
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
	experimentDetails.ContainerRuntime = types.Getenv("CONTAINER_RUNTIME", "")
	experimentDetails.SocketPath = types.Getenv("SOCKET_PATH", "")
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
			err := revertDiskFill(t, clients)
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

	usedEphemeralStorageSize, err := getUsedEphemeralStorage(t)
	if err != nil {
		return 0, stacktrace.Propagate(err, "could not get used ephemeral storage")
	}

	// GetEphemeralStorageAttributes derive the ephemeral storage attributes from the target container
	ephemeralStorageLimit, err := getEphemeralStorageAttributes(t, clients)
	if err != nil {
		return 0, stacktrace.Propagate(err, "could not get ephemeral storage attributes")
	}

	if ephemeralStorageLimit == 0 && experimentsDetails.EphemeralStorageMebibytes == "0" {
		return 0, cerrors.Error{ErrorCode: cerrors.ErrorTypeHelper, Source: t.Source, Target: fmt.Sprintf("{podName: %s, namespace: %s}", t.Name, t.Namespace), Reason: "either provide ephemeral storage limit inside target container or define EPHEMERAL_STORAGE_MEBIBYTES ENV"}
	}

	// deriving the ephemeral storage size to be filled
	sizeTobeFilled := getSizeToBeFilled(experimentsDetails, usedEphemeralStorageSize, int(ephemeralStorageLimit))

	return sizeTobeFilled, nil
}

func getUsedEphemeralStorage(t targetDetails) (int, error) {
	// derive the used ephemeral storage size from the target container
	du := fmt.Sprintf("sudo du /proc/%v/root", t.TargetPID)
	cmd := exec.Command("/bin/bash", "-c", du)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Error(err.Error())
		return 0, cerrors.Error{ErrorCode: cerrors.ErrorTypeHelper, Source: t.Source, Target: fmt.Sprintf("{podName: %s, namespace: %s, container: %s}", t.Name, t.Namespace, t.TargetContainer), Reason: fmt.Sprintf("failed to get used ephemeral storage size: %s", string(out))}
	}
	ephemeralStorageDetails := string(out)

	// filtering out the used ephemeral storage from the output of du command
	usedEphemeralStorageSize, err := filterUsedEphemeralStorage(ephemeralStorageDetails)
	if err != nil {
		return 0, cerrors.Error{ErrorCode: cerrors.ErrorTypeHelper, Source: t.Source, Target: fmt.Sprintf("{podName: %s, namespace: %s, container: %s}", t.Name, t.Namespace, t.TargetContainer), Reason: fmt.Sprintf("failed to get used ephemeral storage size: %s", err.Error())}
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
	TargetPID       int
	Source          string
}
