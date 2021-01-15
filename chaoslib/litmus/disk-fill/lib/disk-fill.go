package lib

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

	// Get the target pod details for the chaos execution
	// if the target pod is not defined it will derive the random target pod list using pod affected percentage
	targetPodList, err := common.GetPodList(experimentsDetails.TargetPods, experimentsDetails.PodsAffectedPerc, clients, chaosDetails)
	if err != nil {
		return err
	}

	podNames := []string{}
	for _, pod := range targetPodList.Items {
		podNames = append(podNames, pod.Name)
	}
	log.Infof("Target pods list for chaos, %v", podNames)

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	if experimentsDetails.EngineName != "" {
		// Get Chaos Pod Annotation
		experimentsDetails.Annotations, err = common.GetChaosPodAnnotation(experimentsDetails.ChaosPodName, experimentsDetails.ChaosNamespace, clients)
		if err != nil {
			return errors.Errorf("unable to get annotations, err: %v", err)
		}
		// Get Resource Requirements
		experimentsDetails.Resources, err = common.GetChaosPodResourceRequirements(experimentsDetails.ChaosPodName, experimentsDetails.ExperimentName, experimentsDetails.ChaosNamespace, clients)
		if err != nil {
			return errors.Errorf("Unable to get resource requirements, err: %v", err)
		}
	}

	// generating the chaos inject event in the chaosengine
	if experimentsDetails.EngineName != "" {
		msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on application pod"
		types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
		events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
	}

	if experimentsDetails.Sequence == "serial" {
		if err = InjectChaosInSerialMode(experimentsDetails, targetPodList, clients, chaosDetails, execCommandDetails); err != nil {
			return err
		}
	} else {
		if err = InjectChaosInParallelMode(experimentsDetails, targetPodList, clients, chaosDetails, execCommandDetails); err != nil {
			return err
		}
	}

	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	return nil
}

// InjectChaosInSerialMode fill the ephemeral storage of all target application serially (one by one)
func InjectChaosInSerialMode(experimentsDetails *experimentTypes.ExperimentDetails, targetPodList apiv1.PodList, clients clients.ClientSets, chaosDetails *types.ChaosDetails, execCommandDetails exec.PodDetails) error {

	// creating the helper pod to perform disk-fill chaos
	for _, pod := range targetPodList.Items {
		runID := common.GetRunID()
		err := CreateHelperPod(experimentsDetails, clients, pod.Name, pod.Spec.NodeName, runID)
		if err != nil {
			return errors.Errorf("Unable to create the helper pod, err: %v", err)
		}

		//checking the status of the helper pods, wait till the pod comes to running state else fail the experiment
		log.Info("[Status]: Checking the status of the helper pods")
		err = status.CheckApplicationStatus(experimentsDetails.ChaosNamespace, "app="+experimentsDetails.ExperimentName+"-helper", experimentsDetails.Timeout, experimentsDetails.Delay, clients)
		if err != nil {
			common.DeleteHelperPodBasedOnJobCleanupPolicy(experimentsDetails.ExperimentName+"-"+runID, "app="+experimentsDetails.ExperimentName+"-helper", chaosDetails, clients)
			return errors.Errorf("helper pods are not in running state, err: %v", err)
		}

		//Get the target container name of the application pod
		if experimentsDetails.TargetContainer == "" {
			experimentsDetails.TargetContainer, err = GetTargetContainer(experimentsDetails, pod.Name, clients)
			if err != nil {
				common.DeleteHelperPodBasedOnJobCleanupPolicy(experimentsDetails.ExperimentName+"-"+runID, "app="+experimentsDetails.ExperimentName+"-helper", chaosDetails, clients)
				return errors.Errorf("Unable to get the target container name, err: %v", err)
			}
		}

		// GetEphemeralStorageAttributes derive the ephemeral storage attributes from the target container
		ephemeralStorageLimit, ephemeralStorageRequest, err := GetEphemeralStorageAttributes(experimentsDetails, clients, pod.Name)
		if err != nil {
			common.DeleteHelperPodBasedOnJobCleanupPolicy(experimentsDetails.ExperimentName+"-"+runID, "app="+experimentsDetails.ExperimentName+"-helper", chaosDetails, clients)
			return err
		}

		// Derive the container id of the target container
		containerID, err := GetContainerID(experimentsDetails, clients, pod.Name)
		if err != nil {
			common.DeleteHelperPodBasedOnJobCleanupPolicy(experimentsDetails.ExperimentName+"-"+runID, "app="+experimentsDetails.ExperimentName+"-helper", chaosDetails, clients)
			return err
		}

		log.InfoWithValues("[Info]: Details of application under chaos injection", logrus.Fields{
			"PodName":                 pod.Name,
			"NodeName":                pod.Spec.NodeName,
			"ContainerName":           experimentsDetails.TargetContainer,
			"ephemeralStorageLimit":   ephemeralStorageLimit,
			"ephemeralStorageRequest": ephemeralStorageRequest,
			"ContainerID":             containerID,
		})

		// getting the helper pod name, scheduled on the target node
		podName, err := GetHelperPodName(pod, clients, experimentsDetails.ChaosNamespace, "app="+experimentsDetails.ExperimentName+"-helper")
		if err != nil {
			common.DeleteHelperPodBasedOnJobCleanupPolicy(experimentsDetails.ExperimentName+"-"+runID, "app="+experimentsDetails.ExperimentName+"-helper", chaosDetails, clients)
			return err
		}

		// Derive the used ephemeral storage size from the target container
		// It will exec inside disk-fill helper pod & derive the used ephemeral storage space
		command := "sudo du /diskfill/" + containerID
		exec.SetExecCommandAttributes(&execCommandDetails, podName, "disk-fill", experimentsDetails.ChaosNamespace)
		ephemeralStorageDetails, err := exec.Exec(&execCommandDetails, clients, strings.Fields(command))
		if err != nil {
			common.DeleteHelperPodBasedOnJobCleanupPolicy(experimentsDetails.ExperimentName+"-"+runID, "app="+experimentsDetails.ExperimentName+"-helper", chaosDetails, clients)
			return errors.Errorf("Unable to get ephemeral storage details, err: %v", err)
		}

		// filtering out the used ephemeral storage from the output of du command
		usedEphemeralStorageSize, err := FilterUsedEphemeralStorage(ephemeralStorageDetails)
		if err != nil {
			common.DeleteHelperPodBasedOnJobCleanupPolicy(experimentsDetails.ExperimentName+"-"+runID, "app="+experimentsDetails.ExperimentName+"-helper", chaosDetails, clients)
			return errors.Errorf("Unable to filter used ephemeral storage size, err: %v", err)
		}
		log.Infof("used ephemeral storage space: %v", strconv.Itoa(usedEphemeralStorageSize))

		// deriving the ephemeral storage size to be filled
		sizeTobeFilled := GetSizeToBeFilled(experimentsDetails, usedEphemeralStorageSize, int(ephemeralStorageLimit))

		log.Infof("ephemeral storage size to be filled: %v", strconv.Itoa(sizeTobeFilled))

		if sizeTobeFilled > 0 {
			// Creating files to fill the required ephemeral storage size of block size of 4K
			command := "sudo dd if=/dev/urandom of=/diskfill/" + containerID + "/diskfill bs=4K count=" + strconv.Itoa(sizeTobeFilled/4)
			_, err = exec.Exec(&execCommandDetails, clients, strings.Fields(command))
			if err != nil {
				common.DeleteHelperPodBasedOnJobCleanupPolicy(experimentsDetails.ExperimentName+"-"+runID, "app="+experimentsDetails.ExperimentName+"-helper", chaosDetails, clients)
				return errors.Errorf("Unable to fill the ephemeral storage, err: %v", err)
			}
		} else {
			log.Warn("No required free space found!, It's Housefull")
		}

		// waiting for the chaos duration
		log.Infof("[Wait]: Waiting for the %vs after injecting chaos", experimentsDetails.ChaosDuration)
		common.WaitForDuration(experimentsDetails.ChaosDuration)

		// It will delete the target pod if target pod is evicted
		// if target pod is still running then it will delete all the files, which was created earlier during chaos execution
		exec.SetExecCommandAttributes(&execCommandDetails, podName, "disk-fill", experimentsDetails.ChaosNamespace)
		err = Remedy(experimentsDetails, clients, containerID, pod.Name, &execCommandDetails)
		if err != nil {
			common.DeleteHelperPodBasedOnJobCleanupPolicy(experimentsDetails.ExperimentName+"-"+runID, "app="+experimentsDetails.ExperimentName+"-helper", chaosDetails, clients)
			return errors.Errorf("Unable to perform remedy operation due to %v", err)
		}

		//Deleting all the helper pod for disk-fill chaos
		log.Info("[Cleanup]: Deleting the helper pod")
		err = common.DeletePod(experimentsDetails.ExperimentName+"-"+runID, "app="+experimentsDetails.ExperimentName+"-helper", experimentsDetails.ChaosNamespace, chaosDetails.Timeout, chaosDetails.Delay, clients)
		if err != nil {
			return errors.Errorf("Unable to delete the helper pod, %v", err)
		}
	}

	return nil

}

// InjectChaosInParallelMode fill the ephemeral storage of of all target application in parallel mode (all at once)
func InjectChaosInParallelMode(experimentsDetails *experimentTypes.ExperimentDetails, targetPodList apiv1.PodList, clients clients.ClientSets, chaosDetails *types.ChaosDetails, execCommandDetails exec.PodDetails) error {

	// creating the helper pod to perform disk-fill chaos
	for _, pod := range targetPodList.Items {
		runID := common.GetRunID()
		err := CreateHelperPod(experimentsDetails, clients, pod.Name, pod.Spec.NodeName, runID)
		if err != nil {
			return errors.Errorf("Unable to create the helper pod, err: %v", err)
		}
	}

	//checking the status of the helper pods, wait till the pod comes to running state else fail the experiment
	log.Info("[Status]: Checking the status of the helper pods")
	err := status.CheckApplicationStatus(experimentsDetails.ChaosNamespace, "app="+experimentsDetails.ExperimentName+"-helper", experimentsDetails.Timeout, experimentsDetails.Delay, clients)
	if err != nil {
		common.DeleteAllHelperPodBasedOnJobCleanupPolicy("app="+experimentsDetails.ExperimentName+"-helper", chaosDetails, clients)
		return errors.Errorf("helper pods are not in running state, err: %v", err)
	}

	for _, pod := range targetPodList.Items {

		//Get the target container name of the application pod
		if experimentsDetails.TargetContainer == "" {
			experimentsDetails.TargetContainer, err = GetTargetContainer(experimentsDetails, pod.Name, clients)
			if err != nil {
				common.DeleteAllHelperPodBasedOnJobCleanupPolicy("app="+experimentsDetails.ExperimentName+"-helper", chaosDetails, clients)
				return errors.Errorf("Unable to get the target container name, err: %v", err)
			}
		}

		// GetEphemeralStorageAttributes derive the ephemeral storage attributes from the target container
		ephemeralStorageLimit, ephemeralStorageRequest, err := GetEphemeralStorageAttributes(experimentsDetails, clients, pod.Name)
		if err != nil {
			common.DeleteAllHelperPodBasedOnJobCleanupPolicy("app="+experimentsDetails.ExperimentName+"-helper", chaosDetails, clients)
			return err
		}

		// Derive the container id of the target container
		containerID, err := GetContainerID(experimentsDetails, clients, pod.Name)
		if err != nil {
			common.DeleteAllHelperPodBasedOnJobCleanupPolicy("app="+experimentsDetails.ExperimentName+"-helper", chaosDetails, clients)
			return err
		}

		log.InfoWithValues("[Info]: Details of application under chaos injection", logrus.Fields{
			"PodName":                 pod.Name,
			"NodeName":                pod.Spec.NodeName,
			"ContainerName":           experimentsDetails.TargetContainer,
			"ephemeralStorageLimit":   ephemeralStorageLimit,
			"ephemeralStorageRequest": ephemeralStorageRequest,
			"ContainerID":             containerID,
		})

		// getting the helper pod name, scheduled on the target node
		podName, err := GetHelperPodName(pod, clients, experimentsDetails.ChaosNamespace, "app="+experimentsDetails.ExperimentName+"-helper")
		if err != nil {
			common.DeleteAllHelperPodBasedOnJobCleanupPolicy("app="+experimentsDetails.ExperimentName+"-helper", chaosDetails, clients)
			return err
		}

		// Derive the used ephemeral storage size from the target container
		// It will exec inside disk-fill helper pod & derive the used ephemeral storage space
		command := "sudo du /diskfill/" + containerID
		exec.SetExecCommandAttributes(&execCommandDetails, podName, "disk-fill", experimentsDetails.ChaosNamespace)
		ephemeralStorageDetails, err := exec.Exec(&execCommandDetails, clients, strings.Fields(command))
		if err != nil {
			common.DeleteAllHelperPodBasedOnJobCleanupPolicy("app="+experimentsDetails.ExperimentName+"-helper", chaosDetails, clients)
			return errors.Errorf("Unable to get ephemeral storage details, err: %v", err)
		}

		// filtering out the used ephemeral storage from the output of du command
		usedEphemeralStorageSize, err := FilterUsedEphemeralStorage(ephemeralStorageDetails)
		if err != nil {
			common.DeleteAllHelperPodBasedOnJobCleanupPolicy("app="+experimentsDetails.ExperimentName+"-helper", chaosDetails, clients)
			return errors.Errorf("Unable to filter used ephemeral storage size, err: %v", err)
		}
		log.Infof("used ephemeral storage space: %v", strconv.Itoa(usedEphemeralStorageSize))

		// deriving the ephemeral storage size to be filled
		sizeTobeFilled := GetSizeToBeFilled(experimentsDetails, usedEphemeralStorageSize, int(ephemeralStorageLimit))

		log.Infof("ephemeral storage size to be filled: %v", strconv.Itoa(sizeTobeFilled))

		if sizeTobeFilled > 0 {
			// Creating files to fill the required ephemeral storage size of block size of 4K
			command := "sudo dd if=/dev/urandom of=/diskfill/" + containerID + "/diskfill bs=4K count=" + strconv.Itoa(sizeTobeFilled/4)
			_, err = exec.Exec(&execCommandDetails, clients, strings.Fields(command))
			if err != nil {
				common.DeleteAllHelperPodBasedOnJobCleanupPolicy("app="+experimentsDetails.ExperimentName+"-helper", chaosDetails, clients)
				return errors.Errorf("Unable to fill the ephemeral storage, err: %v", err)
			}
		} else {
			log.Warn("No required free space found!, It's Housefull")
		}
	}

	// waiting for the chaos duration
	log.Infof("[Wait]: Waiting for the %vs after injecting chaos", experimentsDetails.ChaosDuration)
	common.WaitForDuration(experimentsDetails.ChaosDuration)

	for _, pod := range targetPodList.Items {

		// Derive the container id of the target container
		containerID, err := GetContainerID(experimentsDetails, clients, pod.Name)
		if err != nil {
			common.DeleteAllHelperPodBasedOnJobCleanupPolicy("app="+experimentsDetails.ExperimentName+"-helper", chaosDetails, clients)
			return err
		}

		// getting the helper pod name, scheduled on the target node
		podName, err := GetHelperPodName(pod, clients, experimentsDetails.ChaosNamespace, "app="+experimentsDetails.ExperimentName+"-helper")
		if err != nil {
			common.DeleteAllHelperPodBasedOnJobCleanupPolicy("app="+experimentsDetails.ExperimentName+"-helper", chaosDetails, clients)
			return err
		}

		// It will delete the target pod if target pod is evicted
		// if target pod is still running then it will delete all the files, which was created earlier during chaos execution
		exec.SetExecCommandAttributes(&execCommandDetails, podName, "disk-fill", experimentsDetails.ChaosNamespace)
		err = Remedy(experimentsDetails, clients, containerID, pod.Name, &execCommandDetails)
		if err != nil {
			common.DeleteAllHelperPodBasedOnJobCleanupPolicy("app="+experimentsDetails.ExperimentName+"-helper", chaosDetails, clients)
			return errors.Errorf("Unable to perform remedy operation due to %v", err)
		}
	}

	//Deleting all the helper pod for disk-fill chaos
	log.Info("[Cleanup]: Deleting all the helper pod")
	err = common.DeleteAllPod("app="+experimentsDetails.ExperimentName+"-helper", experimentsDetails.ChaosNamespace, chaosDetails.Timeout, chaosDetails.Delay, clients)
	if err != nil {
		return errors.Errorf("Unable to delete the helper pod, %v", err)
	}

	return nil

}

//GetTargetContainer will fetch the container name from application pod
// It will return the first container name from the application pod
func GetTargetContainer(experimentsDetails *experimentTypes.ExperimentDetails, appName string, clients clients.ClientSets) (string, error) {
	pod, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).Get(appName, v1.GetOptions{})
	if err != nil {
		return "", err
	}

	return pod.Spec.Containers[0].Name, nil
}

// CreateHelperPod derive the attributes for helper pod and create the helper pod
func CreateHelperPod(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, appName, appNodeName, runID string) error {

	mountPropagationMode := apiv1.MountPropagationHostToContainer

	helperPod := &apiv1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:      experimentsDetails.ExperimentName + "-" + runID,
			Namespace: experimentsDetails.ChaosNamespace,
			Labels: map[string]string{
				"app":                       experimentsDetails.ExperimentName + "-helper",
				"name":                      experimentsDetails.ExperimentName + "-" + runID,
				"chaosUID":                  string(experimentsDetails.ChaosUID),
				"app.kubernetes.io/part-of": "litmus",
			},
			Annotations: experimentsDetails.Annotations,
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
					ImagePullPolicy: apiv1.PullPolicy(experimentsDetails.LIBImagePullPolicy),
					Args: []string{
						"sleep",
						"10000",
					},
					Resources: experimentsDetails.Resources,
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
		command := "sudo rm -rf /diskfill/" + containerID + "/diskfill"
		_, err = exec.Exec(execCommandDetails, clients, strings.Fields(command))
		if err != nil {
			return errors.Errorf("Unable to delete files to reset ephemeral storage usage due to err: %v", err)
		}
	}
	return nil
}

// GetHelperPodName check for the helper pod, which is scheduled on the target node
func GetHelperPodName(targetPod apiv1.Pod, clients clients.ClientSets, namespace, labels string) (string, error) {

	podList, err := clients.KubeClient.CoreV1().Pods(namespace).List(v1.ListOptions{LabelSelector: labels})

	if err != nil || len(podList.Items) == 0 {
		return "", errors.Errorf("Unable to list the helper pods, %v", err)
	}

	for _, pod := range podList.Items {
		if pod.Spec.NodeName == targetPod.Spec.NodeName {
			return pod.Name, nil
		}
	}
	return "", errors.Errorf("No helper pod is available on %v node", targetPod.Spec.NodeName)
}
