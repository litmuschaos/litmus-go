package disk_fill

import (
	"fmt"
	"strconv"
	"strings"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/disk-fill/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/litmuschaos/litmus-go/pkg/utils/exec"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//PrepareDiskFill contains the prepration steps before chaos injection
func PrepareDiskFill(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	// It will contains all the pod & container details required for exec command
	execCommandDetails := exec.PodDetails{}

	//Select application pod & node for the disk fill chaos
	appName, appNodeName, err := common.GetPodAndNodeName(experimentsDetails.AppNS, experimentsDetails.TargetPod, experimentsDetails.AppLabel, clients)
	if err != nil {
		return errors.Errorf("Unable to get the application pod and node name due to, err: %v", err)
	}

	//Get the target container name of the application pod
	if experimentsDetails.TargetContainer == "" {
		experimentsDetails.TargetContainer, err = GetTargetContainer(experimentsDetails, appName, clients)
		if err != nil {
			return errors.Errorf("Unable to get the target container name due to, err: %v", err)
		}
	}

	// GetEphemeralStorageAttributes derive the ephemeral storage attributes from the target container
	ephemeralStorageLimit, ephemeralStorageRequest, err := GetEphemeralStorageAttributes(experimentsDetails, clients, appName)
	if err != nil {
		return err
	}

	// Derive the container id of the target container
	containerID, err := GetContainerID(experimentsDetails, clients, appName)
	if err != nil {
		return err
	}

	log.InfoWithValues("[Info]: Details of application under chaos injection", logrus.Fields{
		"PodName":                 appName,
		"NodeName":                appNodeName,
		"ContainerName":           experimentsDetails.TargetContainer,
		"ephemeralStorageLimit":   ephemeralStorageLimit,
		"ephemeralStorageRequest": ephemeralStorageRequest,
		"ContainerID":             containerID,
	})

	// generating a unique string which can be appended with the helper pod name & labels for the uniquely identification
	experimentsDetails.RunID = common.GetRunID()

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", strconv.Itoa(experimentsDetails.RampTime))
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	// generating the chaos inject event in the chaosengine
	if experimentsDetails.EngineName != "" {
		msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on " + appName + " pod"
		types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
		events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
	}

	// creating the helper pod to perform disk fill chaos
	err = CreateHelperPod(experimentsDetails, clients, appName, appNodeName)
	if err != nil {
		errors.Errorf("Unable to create the helper pod, err: %v", err)
	}

	//checking the status of the helper pod, wait till the helper pod comes to running state else fail the experiment
	log.Info("[Status]: Checking the status of the helper pod")
	err = status.CheckApplicationStatus(experimentsDetails.ChaosNamespace, "name="+experimentsDetails.ExperimentName+"-"+experimentsDetails.RunID, experimentsDetails.Timeout, experimentsDetails.Delay, clients)
	if err != nil {
		return errors.Errorf("helper pod is not in running state, err: %v", err)
	}

	// Derive the used ephemeral storage size from the target container
	// It will exec inside disk-fill helper pod & derive the used ephemeral storage space
	command := "du /diskfill/" + containerID
	exec.SetExecCommandAttributes(&execCommandDetails, "disk-fill-"+experimentsDetails.RunID, "disk-fill", experimentsDetails.ChaosNamespace)
	ephemeralStorageDetails, err := exec.Exec(&execCommandDetails, clients, strings.Fields(command))
	if err != nil {
		return errors.Errorf("Unable to get ephemeral storage details due to err: %v", err)
	}

	// filtering out the used ephemeral storage from the output of du command
	usedEphemeralStorageSize, err := FilterUsedEphemeralStorage(ephemeralStorageDetails)
	if err != nil {
		return errors.Errorf("Unable to filter used ephemeral storage size due to err: %v", err)
	}
	log.Infof("used ephemeral storage space: %v", strconv.Itoa(usedEphemeralStorageSize))

	// deriving the ephemeral storage size to be filled
	sizeTobeFilled := GetSizeToBeFilled(experimentsDetails, usedEphemeralStorageSize, int(ephemeralStorageLimit))

	log.Infof("ephemeral storage size to be filled: %v", strconv.Itoa(sizeTobeFilled))

	if sizeTobeFilled > 0 {
		// Creating files to fill the required ephemeral storage size of block size of 4K
		command := "dd if=/dev/urandom of=/diskfill/" + containerID + "/diskfill bs=4K count=" + strconv.Itoa(sizeTobeFilled/4)
		_, err = exec.Exec(&execCommandDetails, clients, strings.Fields(command))
		if err != nil {
			return errors.Errorf("Unable to to create the files to fill the ephemeral storage due to err: %v", err)
		}
	} else {
		log.Warn("No required free space found!, It's Housefull")
	}

	// waiting for the chaos duration
	log.Infof("[Wait]: Waiting for the %vs after injecting chaos", strconv.Itoa(experimentsDetails.ChaosDuration))
	common.WaitForDuration(experimentsDetails.ChaosDuration)

	// It will delete the target pod if target pod is evicted
	// if target pod is still running then it will delete all the files, which was created earlier during chaos execution
	err = Remedy(experimentsDetails, clients, containerID, appName, &execCommandDetails)
	if err != nil {
		return errors.Errorf("Unable to perform remedy operation due to err: %v", err)
	}

	//Deleting the helper pod
	log.Info("[Cleanup]: Deleting the helper pod")
	err = common.DeletePod(experimentsDetails.ExperimentName+"-"+experimentsDetails.RunID, "name="+experimentsDetails.ExperimentName+"-"+experimentsDetails.RunID, experimentsDetails.ChaosNamespace, chaosDetails.Timeout, chaosDetails.Delay, clients)
	if err != nil {
		return errors.Errorf("Unable to delete the helper pod, err: %v", err)
	}

	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", strconv.Itoa(experimentsDetails.RampTime))
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	return nil
}

//GetTargetContainer will fetch the container name from application pod
// It will return the first container name from the application pod
func GetTargetContainer(experimentsDetails *experimentTypes.ExperimentDetails, appName string, clients clients.ClientSets) (string, error) {
	pod, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).Get(appName, v1.GetOptions{})
	if err != nil {
		return "", errors.Wrapf(err, "Fail to get the application pod status, due to:%v", err)
	}

	return pod.Spec.Containers[0].Name, nil
}

// CreateHelperPod derive the attributes for helper pod and create the helper pod
func CreateHelperPod(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, appName, appNodeName string) error {

	mountPropagationMode := apiv1.MountPropagationHostToContainer

	helperPod := &apiv1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:      experimentsDetails.ExperimentName + "-" + experimentsDetails.RunID,
			Namespace: experimentsDetails.ChaosNamespace,
			Labels: map[string]string{
				"app":      experimentsDetails.ExperimentName,
				"name":     experimentsDetails.ExperimentName + "-" + experimentsDetails.RunID,
				"chaosUID": string(experimentsDetails.ChaosUID),
			},
		},
		Spec: apiv1.PodSpec{
			RestartPolicy: apiv1.RestartPolicyNever,
			NodeName:      appNodeName,
			Volumes: []apiv1.Volume{
				{
					Name: "udev",
					VolumeSource: apiv1.VolumeSource{
						HostPath: &apiv1.HostPathVolumeSource{
							Path: experimentsDetails.ContainerPath,
						},
					},
				},
			},
			Containers: []apiv1.Container{
				{
					Name:            experimentsDetails.ExperimentName,
					Image:           experimentsDetails.LIBImage,
					ImagePullPolicy: apiv1.PullAlways,
					Args: []string{
						"sleep",
						"10000",
					},
					VolumeMounts: []apiv1.VolumeMount{
						{
							Name:             "udev",
							MountPath:        "/diskfill",
							MountPropagation: &mountPropagationMode,
						},
					},
				},
			},
		},
	}

	_, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.ChaosNamespace).Create(helperPod)
	return err
}

// GetEphemeralStorageAttributes derive the ephemeral storage attributes from the target pod
func GetEphemeralStorageAttributes(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, podName string) (int64, int64, error) {

	pod, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).Get(podName, v1.GetOptions{})

	if err != nil {
		return 0, 0, err
	}

	var ephemeralStorageLimit, ephemeralStorageRequest int64
	containers := pod.Spec.Containers

	// Extracting ephemeral storage limit & requested value from the target container
	// It will be in the form of Kb
	for _, container := range containers {
		if container.Name == experimentsDetails.TargetContainer {
			ephemeralStorageLimit = container.Resources.Limits.StorageEphemeral().ToDec().ScaledValue(resource.Kilo)
			ephemeralStorageRequest = container.Resources.Requests.StorageEphemeral().ToDec().ScaledValue(resource.Kilo)
			break
		}
	}

	if ephemeralStorageRequest == 0 || ephemeralStorageLimit == 0 {
		return 0, 0, fmt.Errorf("No Ephemeral storage details found inside %v container", experimentsDetails.TargetContainer)
	}

	return ephemeralStorageLimit, ephemeralStorageRequest, nil
}

// GetContainerID derive the container id of the target container
func GetContainerID(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, podName string) (string, error) {
	pod, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).Get(podName, v1.GetOptions{})

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

	// deriving size need to be filled from the used size & requirement size to fill
	requirementToBeFill := (ephemeralStorageLimit * experimentsDetails.FillPercentage) / 100
	needToBeFilled := requirementToBeFill - usedEphemeralStorageSize
	return needToBeFilled
}

// Remedy will delete the target pod if target pod is evicted
// if target pod is still running then it will delete the files, which was created during chaos execution
func Remedy(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, containerID string, podName string, execCommandDetails *exec.PodDetails) error {
	pod, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).Get(podName, v1.GetOptions{})
	if err != nil {
		return err
	}
	// Deleting the pod as pod is already evicted
	podReason := pod.Status.Reason
	if podReason == "Evicted" {
		if err := clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).Delete(podName, &v1.DeleteOptions{}); err != nil {
			return err
		}
	} else {

		// deleting the files after chaos execution
		command := "rm -rf /diskfill/" + containerID + "/diskfill"
		_, err = exec.Exec(execCommandDetails, clients, strings.Fields(command))
		if err != nil {
			return errors.Errorf("Unable to delete files to clean ephemeral storage due to err: %v", err)
		}
	}
	return nil
}
