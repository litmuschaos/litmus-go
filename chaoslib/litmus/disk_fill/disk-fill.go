package disk_fill

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/disk-fill/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/openebs/maya/pkg/util/retry"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/remotecommand"
)

var err error

// GetEphemeralStorageSize return the ephemeral storage size for the target pod
func GetEphemeralStorageSize(experimentsDetails *experimentTypes.ExperimentDetails, containerID string, clients clients.ClientSets) (int, error) {

	// it will return the ephemeral storage size for the target container
	command := fmt.Sprintf("du /diskfill/" + containerID)

	req := clients.KubeClient.CoreV1().RESTClient().Post().
		Resource("pods").
		Name("disk-fill-" + experimentsDetails.RunID).
		Namespace(experimentsDetails.ChaosNamespace).
		SubResource("exec")
	scheme := runtime.NewScheme()
	if err := apiv1.AddToScheme(scheme); err != nil {
		return 0, fmt.Errorf("error adding to scheme: %v", err)
	}

	parameterCodec := runtime.NewParameterCodec(scheme)
	req.VersionedParams(&apiv1.PodExecOptions{
		Command:   strings.Fields(command),
		Container: "disk-fill",
		Stdin:     false,
		Stdout:    true,
		Stderr:    true,
		TTY:       false,
	}, parameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(clients.KubeConfig, "POST", req.URL())
	if err != nil {
		return 0, fmt.Errorf("error while creating Executor: %v", err)
	}

	var out bytes.Buffer
	stdout := &out
	stderr := os.Stderr

	err = exec.Stream(remotecommand.StreamOptions{
		Stdin:  nil,
		Stdout: stdout,
		Stderr: stderr,
		Tty:    false,
	})

	if err != nil {
		errorCode := strings.Contains(err.Error(), "143")
		if errorCode != true {
			log.Infof("[Prepare}: Unable to get the ephemeral storage size due to, err : %v", err.Error())
			return 0, err
		}
	}

	// Filtering out the ephemeral storage size from the output of du command
	// It contains details of all subdirectories of target container
	ephemeralStorageAll := strings.Split(out.String(), "\n")
	// It will return the details of main directory
	ephemeralStorageAllDiskFill := strings.Split(ephemeralStorageAll[len(ephemeralStorageAll)-2], "\t")[0]
	// type casting string to interger
	ephemeralStorageSize, err := strconv.Atoi(ephemeralStorageAllDiskFill)
	return ephemeralStorageSize, nil
}

//PrepareDiskFill contains the prepration steps before chaos injection
func PrepareDiskFill(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	//Select application pod & node for the disk fill
	appName, appNodeName, err := GetApplicationPod(experimentsDetails, clients)
	if err != nil {
		return errors.Errorf("Unable to get the application name and application nodename due to, err: %v", err)
	}

	//Get the target container name of the application pod
	if experimentsDetails.TargetContainer == "" {
		experimentsDetails.TargetContainer, err = GetTargetContainer(experimentsDetails, appName, clients)
		if err != nil {
			return errors.Errorf("Unable to get the target container name due to, err: %v", err)
		}
	}

	// GetEmhemeralStorageAttributes derive the emhemeral storage attributes
	ephemeralStorageLimit, ephemeralStorageRequest, err := GetEmhemeralStorageAttributes(experimentsDetails, clients, appName)
	if err != nil {
		return err
	}

	// Getting the container id of target container
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

	// generate the runid, a unique string
	experimentsDetails.RunID = GetRunID()

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", strconv.Itoa(experimentsDetails.RampTime))
		waitForDuration(experimentsDetails.RampTime)
	}

	if experimentsDetails.EngineName != "" {
		msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on " + appName + " pod"
		types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, chaosDetails)
		events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
	}

	// Creating the helper pod to perform network chaos
	err = CreateHelperPod(experimentsDetails, clients, appName, appNodeName)
	if err != nil {
		errors.Errorf("Unable to create the helper pod, err: %v", err)
	}

	//Checking the status of helper pod
	log.Info("[Status]: Checking the status of the helper pod")
	err = status.CheckApplicationStatus(experimentsDetails.ChaosNamespace, "name=disk-fill-"+experimentsDetails.RunID, clients)
	if err != nil {
		return errors.Errorf("helper pod is not in running state, err: %v", err)
	}

	// Derive the used ephemeral storage size from the target container
	usedEphemeralStorageSize, err := GetEphemeralStorageSize(experimentsDetails, containerID, clients)
	log.Infof("used ephemeral storage space: %v", strconv.Itoa(usedEphemeralStorageSize))

	// deriving the ephemeral storage size to be filled
	sizeTobeFilled := GetSizeToBeFilled(experimentsDetails, usedEphemeralStorageSize, int(ephemeralStorageLimit))

	log.Infof("ephemeral storage size to be filled: %v", strconv.Itoa(sizeTobeFilled))

	if sizeTobeFilled > 0 {
		// Creating files to fill those size
		FileCreation(experimentsDetails, clients, containerID, sizeTobeFilled)
	} else {
		log.Warn("jagha nahi hai, housefull")
	}

	// waiting for the chaos duration
	log.Infof("[Wait]: Waiting for the %vs after injecting chaos", strconv.Itoa(experimentsDetails.ChaosDuration))
	waitForDuration(experimentsDetails.ChaosDuration)

	// It will delete the target pod if it is evicted or delete the files, which was created during chaos execution
	err = Remedy(experimentsDetails, clients, containerID, appName)

	//Deleting the helper pod
	log.Info("[Cleanup]: Deleting the helper pod")
	err = DeleteHelperPod(experimentsDetails, clients, experimentsDetails.RunID)
	if err != nil {
		errors.Errorf("Unable to delete the helper pod, err: %v", err)
	}

	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", strconv.Itoa(experimentsDetails.RampTime))
		waitForDuration(experimentsDetails.RampTime)
	}
	return nil
}

//GetApplicationPod will select a random replica of application pod for chaos
//It will also get the node name of the application pod
func GetApplicationPod(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) (string, string, error) {
	podList, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).List(v1.ListOptions{LabelSelector: experimentsDetails.AppLabel})
	if err != nil || len(podList.Items) == 0 {
		return "", "", errors.Wrapf(err, "Fail to get the application pod in %v namespace", experimentsDetails.AppNS)
	}

	rand.Seed(time.Now().Unix())
	randomIndex := rand.Intn(len(podList.Items))
	applicationName := podList.Items[randomIndex].Name
	nodeName := podList.Items[randomIndex].Spec.NodeName

	return applicationName, nodeName, nil
}

//GetTargetContainer will fetch the conatiner name from application pod
//This container will be used as target container
func GetTargetContainer(experimentsDetails *experimentTypes.ExperimentDetails, appName string, clients clients.ClientSets) (string, error) {
	pod, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).Get(appName, v1.GetOptions{})
	if err != nil {
		return "", errors.Wrapf(err, "Fail to get the application pod status, due to:%v", err)
	}

	return pod.Spec.Containers[0].Name, nil
}

//waitForDuration waits for the given time duration (in seconds)
func waitForDuration(duration int) {
	time.Sleep(time.Duration(duration) * time.Second)
}

// GetRunID generate a random string
func GetRunID() string {
	var letterRunes = []rune("abcdefghijklmnopqrstuvwxyz")
	runID := make([]rune, 6)
	for i := range runID {
		runID[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(runID)
}

// CreateHelperPod derive the attributes for helper pod and create the helper pod
func CreateHelperPod(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, appName, appNodeName string) error {

	mountPropagationMode := apiv1.MountPropagationHostToContainer
	privileged := true

	helperPod := &apiv1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:      "disk-fill-" + experimentsDetails.RunID,
			Namespace: experimentsDetails.ChaosNamespace,
			Labels: map[string]string{
				"app":      "disk-fill",
				"name":     "disk-fill-" + experimentsDetails.RunID,
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
					Name:            "disk-fill",
					Image:           "ubuntu",
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
					SecurityContext: &apiv1.SecurityContext{
						Privileged: &privileged,
					},
				},
			},
		},
	}

	_, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.ChaosNamespace).Create(helperPod)
	return err
}

//DeleteHelperPod delete the helper pod
func DeleteHelperPod(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, runID string) error {

	err := clients.KubeClient.CoreV1().Pods(experimentsDetails.ChaosNamespace).Delete("disk-fill-"+runID, &v1.DeleteOptions{})

	if err != nil {
		return err
	}

	err = retry.
		Times(90).
		Wait(1 * time.Second).
		Try(func(attempt uint) error {
			podSpec, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.ChaosNamespace).List(v1.ListOptions{LabelSelector: "name=disk-fill-" + runID})
			if err != nil || len(podSpec.Items) != 0 {
				return errors.Errorf("Helper Pod is not deleted yet, err: %v", err)
			}
			return nil
		})

	return err
}

// GetEmhemeralStorageAttributes derive the emhemeral storage attributes from target pod
func GetEmhemeralStorageAttributes(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, podName string) (int64, int64, error) {

	pod, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).Get(podName, v1.GetOptions{})

	if err != nil {
		return 0, 0, err
	}

	var ephemeralStorageLimit, ephemeralStorageRequest int64
	containers := pod.Spec.Containers

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

	fmt.Printf("values %v   %v ", ephemeralStorageLimit, ephemeralStorageRequest)

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

	for _, container := range containers {
		if container.Name == experimentsDetails.TargetContainer {
			containerID = strings.Split(container.ContainerID, "//")[1]
			break
		}
	}

	return containerID, nil

}

// FileCreation creates the file to filled the desired space in the ephemeral storage
func FileCreation(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, containerID string, sizeTobeFilled int) error {

	command := fmt.Sprintf("dd if=/dev/urandom of=/diskfill/" + containerID + "/diskfill bs=4K count=" + strconv.Itoa(sizeTobeFilled/4))

	req := clients.KubeClient.CoreV1().RESTClient().Post().
		Resource("pods").
		Name("disk-fill-" + experimentsDetails.RunID).
		Namespace(experimentsDetails.ChaosNamespace).
		SubResource("exec")
	scheme := runtime.NewScheme()
	if err := apiv1.AddToScheme(scheme); err != nil {
		return fmt.Errorf("error adding to scheme: %v", err)
	}

	parameterCodec := runtime.NewParameterCodec(scheme)
	req.VersionedParams(&apiv1.PodExecOptions{
		Command:   strings.Fields(command),
		Container: "disk-fill",
		Stdin:     false,
		Stdout:    true,
		Stderr:    true,
		TTY:       false,
	}, parameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(clients.KubeConfig, "POST", req.URL())
	if err != nil {
		return fmt.Errorf("error while creating Executor: %v", err)
	}

	var out bytes.Buffer
	stdout := &out
	stderr := os.Stderr

	err = exec.Stream(remotecommand.StreamOptions{
		Stdin:  nil,
		Stdout: stdout,
		Stderr: stderr,
		Tty:    false,
	})

	if err != nil {
		errorCode := strings.Contains(err.Error(), "143")
		if errorCode != true {
			log.Infof("[Chaos]:Unable to create the files due to err: %v", err.Error())
			return err
		}
	}

	return nil
}

// FileDeletion deletes the files, which was created during chaos execution
func FileDeletion(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, containerID string) error {

	command := fmt.Sprintf("rm -rf /diskfill/" + containerID + "/diskfill")

	req := clients.KubeClient.CoreV1().RESTClient().Post().
		Resource("pods").
		Name("disk-fill-" + experimentsDetails.RunID).
		Namespace(experimentsDetails.ChaosNamespace).
		SubResource("exec")
	scheme := runtime.NewScheme()
	if err := apiv1.AddToScheme(scheme); err != nil {
		return fmt.Errorf("error adding to scheme: %v", err)
	}

	parameterCodec := runtime.NewParameterCodec(scheme)
	req.VersionedParams(&apiv1.PodExecOptions{
		Command:   strings.Fields(command),
		Container: "disk-fill",
		Stdin:     false,
		Stdout:    true,
		Stderr:    true,
		TTY:       false,
	}, parameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(clients.KubeConfig, "POST", req.URL())
	if err != nil {
		return fmt.Errorf("error while creating Executor: %v", err)
	}

	var out bytes.Buffer
	stdout := &out
	stderr := os.Stderr

	err = exec.Stream(remotecommand.StreamOptions{
		Stdin:  nil,
		Stdout: stdout,
		Stderr: stderr,
		Tty:    false,
	})

	if err != nil {
		errorCode := strings.Contains(err.Error(), "143")
		if errorCode != true {
			log.Infof("[Chaos]:Unable to delete the files due to, err: %v", err.Error())
			return err
		}
	}

	return nil
}

// GetSizeToBeFilled generate the emhemeral storage size need to be filled
func GetSizeToBeFilled(experimentsDetails *experimentTypes.ExperimentDetails, usedEphemeralStorageSize int, ephemeralStorageLimit int) int {

	requirementToFill := (ephemeralStorageLimit * experimentsDetails.FillPercentage) / 100
	needToFilled := requirementToFill - usedEphemeralStorageSize
	return needToFilled
}

// Remedy will delete the target pod if it is evicted or delete the files, which was created during chaos execution
func Remedy(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, containerID string, podName string) error {
	pod, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).Get(podName, v1.GetOptions{})
	if err != nil {
		return err
	}

	podReason := pod.Status.Reason
	if podReason == "Evicted" {
		if err := clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).Delete(podName, &v1.DeleteOptions{}); err != nil {
			return err
		}
	} else {

		// deleting the files after chaos execution
		FileDeletion(experimentsDetails, clients, containerID)
	}
	return nil
}
