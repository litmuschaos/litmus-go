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
        "encoding/json"

	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/volume-fill/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/result"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientTypes "k8s.io/apimachinery/pkg/types"
)

var inject, abort chan os.Signal

type MountDocker struct {
        Type        string `json:"Type"`
        Source      string `json:"Source"`
        Destination string `json:"Destination"`
        Mode        string `json:"Mode,omitempty"`
        Rw          bool   `json:"RW"`
        Propagation string `json:"Propagation,omitempty"`
        Name        string `json:"Name,omitempty"`
        Driver      string `json:"Driver,omitempty"`
}

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

	if err := volumeFill(&experimentsDetails, clients, &eventsDetails, &chaosDetails, &resultDetails); err != nil {
		log.Fatalf("helper pod failed, err: %v", err)
	}
}

//diskFill contains steps to inject disk-fill chaos
func volumeFill(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails, resultDetails *types.ResultDetails) error {

	// Derive the container id of the target container
	containerID, err := common.GetContainerID(experimentsDetails.AppNS, experimentsDetails.TargetPods, experimentsDetails.TargetContainer, clients)
	if err != nil {
		return err
	}


	// Derive the volume mount point from the target container
	volumePath, err := getVolumePath(experimentsDetails, containerID, clients)
	if err != nil {
		return err
	}

	fullPath := strings.TrimPrefix(volumePath, experimentsDetails.ContainerPath)

        du := fmt.Sprintf("sudo df /diskfill/%v |tail -n+2 |awk '{print $3}'", fullPath)
        cmd := exec.Command("/bin/bash", "-c", du)
        out, err := cmd.CombinedOutput()
        if err != nil {
                fmt.Print(string(out))
        }
        usedStorageSize, err := strconv.Atoi(strings.TrimSuffix(string(out), "\n"))


        du2 := fmt.Sprintf("sudo df /diskfill/%v |tail -n+2 |awk '{print $4}'", fullPath)
        cmd2 := exec.Command("/bin/bash", "-c", du2)
        out2, err := cmd2.CombinedOutput()
        if err != nil {
                fmt.Print(string(out2))
        }
        availableStorageSize, err := strconv.Atoi(strings.TrimSuffix(string(out2), "\n"))



        totalStorageSize := usedStorageSize + availableStorageSize


	// deriving the ephemeral storage size to be filled
	sizeTobeFilled := getSizeToBeFilled(experimentsDetails, usedStorageSize, totalStorageSize)

	log.InfoWithValues("[Info]: Details of application under chaos injection", logrus.Fields{
		"PodName":                   experimentsDetails.TargetPods,
		"ContainerName":             experimentsDetails.TargetContainer,
                "VolumeName":                experimentsDetails.TargetVolume,
		"totalStorageSize(KB)":      totalStorageSize,
                "usedStorageSize":           usedStorageSize,
                "availableStorageSize":      availableStorageSize,
                "fillStoragePercentage":     experimentsDetails.FillPercentage,
                "volumePath":                volumePath,
                "fullPath":                  fullPath,
                "containerPath":             experimentsDetails.ContainerPath,
		"ContainerID":               containerID,
	})

	log.Infof("storage size to be filled: %vKB", strconv.Itoa(sizeTobeFilled))

	// record the event inside chaosengine
	if experimentsDetails.EngineName != "" {
		msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on application pod"
		types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
		events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
	}

	// watching for the abort signal and revert the chaos
	go abortWatcher(experimentsDetails, clients, containerID, fullPath, resultDetails.Name)

	if sizeTobeFilled > 0 {

		if err := fillDisk(fullPath, sizeTobeFilled, experimentsDetails.DataBlockSize); err != nil {
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
		err = remedy(experimentsDetails, clients, containerID, fullPath)
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

//stopDockerContainer kill the application container
func inspectDockerContainer(containerID string, socketPath string) ([]MountDocker, error) {
        host := "unix://" + socketPath
        var mounts []MountDocker

        cmd := exec.Command("sudo", "docker", "--host", host, "inspect", "-f",  "{{ json .Mounts }}", string(containerID))
        out, err := cmd.CombinedOutput()
        temp := strings.Split(string(out), "\n")
        json.Unmarshal([]byte(temp[0]), &mounts)
        return mounts, err
}


// fillDisk fill the ephemeral disk by creating files
func fillDisk(containerID string, sizeTobeFilled int, bs int) error {

	select {
	case <-inject:
		// stopping the chaos execution, if abort signal received
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
func getVolumePath(experimentsDetails *experimentTypes.ExperimentDetails, containerID string, clients clients.ClientSets) (string, error) {

	pod, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).Get(experimentsDetails.TargetPods, v1.GetOptions{})
	if err != nil {
		return "", err
	}

	var mountPath string
	containers := pod.Spec.Containers

	// Extracting ephemeral storage limit & requested value from the target container
	// It will be in the form of Kb
	for _, container := range containers {
		if container.Name == experimentsDetails.TargetContainer {
		  volumes := container.VolumeMounts
		  for _, volume := range volumes {
			  if volume.Name == experimentsDetails.TargetVolume {
			    mountPath = volume.MountPath
			    break
			  }
			}
		}
	}

        inspect, err := inspectDockerContainer(containerID, experimentsDetails.SocketPath)

        var sourcePath string

        for _, mount := range inspect {
          if mount.Destination == mountPath {
            sourcePath = mount.Source
            break
          }
	}

	return sourcePath, nil
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
func remedy(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, containerID string, fullPath string) error {
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
                log.Info("Deleting the file after chaos execution")
		rm := fmt.Sprintf("sudo rm -rf /diskfill/%v/diskfill", fullPath)
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
	experimentDetails.AppNS = types.Getenv("APP_NAMESPACE", "")
	experimentDetails.TargetContainer = types.Getenv("APP_CONTAINER", "")
	experimentDetails.TargetPods = types.Getenv("APP_POD", "")
	experimentDetails.TargetVolume = types.Getenv("APP_VOLUME", "")
	experimentDetails.ChaosDuration, _ = strconv.Atoi(types.Getenv("TOTAL_CHAOS_DURATION", "30"))
	experimentDetails.ChaosNamespace = types.Getenv("CHAOS_NAMESPACE", "litmus")
	experimentDetails.EngineName = types.Getenv("CHAOSENGINE", "")
	experimentDetails.ChaosUID = clientTypes.UID(types.Getenv("CHAOS_UID", ""))
	experimentDetails.ChaosPodName = types.Getenv("POD_NAME", "")
	experimentDetails.FillPercentage, _ = strconv.Atoi(types.Getenv("FILL_PERCENTAGE", ""))
	experimentDetails.EphemeralStorageMebibytes, _ = strconv.Atoi(types.Getenv("EPHEMERAL_STORAGE_MEBIBYTES", ""))
	experimentDetails.DataBlockSize, _ = strconv.Atoi(types.Getenv("DATA_BLOCK_SIZE", "256"))
	experimentDetails.SocketPath = types.Getenv("SOCKET_PATH", "")
	experimentDetails.ContainerRuntime = types.Getenv("CONTAINER_RUNTIME", "")
        experimentDetails.ContainerPath = types.Getenv("CONTAINER_PATH", "")

}

// abortWatcher continuously watch for the abort signals
func abortWatcher(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, containerID, fullPath, resultName string) {
	// waiting till the abort signal received
	<-abort

	log.Info("[Chaos]: Killing process started because of terminated signal received")
	log.Info("Chaos Revert Started")
	// retry thrice for the chaos revert
	retry := 3
	for retry > 0 {
		if err := remedy(experimentsDetails, clients, containerID, fullPath); err != nil {
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
