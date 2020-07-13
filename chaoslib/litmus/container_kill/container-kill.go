package container_kill

import (
	"math/rand"
	"strconv"
	"time"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/container-kill/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/math"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/openebs/maya/pkg/util/retry"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//PrepareContainerKill contains the prepration steps before chaos injection
func PrepareContainerKill(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails) error {

	//Select application pod and node name for the container-kill
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

	log.InfoWithValues("[Info]: Details of application under chaos injection", logrus.Fields{
		"PodName":       appName,
		"NodeName":      appNodeName,
		"ContainerName": experimentsDetails.TargetContainer,
	})

	//Getting the iteration count for the container-kill
	GetIterations(experimentsDetails)

	// generating a unique string which can be appended with the helper pod name & labels for the uniquely identification
	experimentsDetails.RunID = GetRunID()

	// Getting the serviceAccountName, need permission inside helper pod to create the events
	if experimentsDetails.ChaosServiceAccount == "" {
		err = GetServiceAccount(experimentsDetails, clients)
		if err != nil {
			return errors.Errorf("Unable to get the serviceAccountName, err: %v", err)
		}
	}

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", strconv.Itoa(experimentsDetails.RampTime))
		waitForDuration(experimentsDetails.RampTime)
	}

	//GetRestartCount return the restart count of target container
	restartCountBefore, err := GetRestartCount(experimentsDetails, appName, clients)
	if err != nil {
		return err
	}

	log.Infof("restartCount of target container before chaos injection: %v", strconv.Itoa(restartCountBefore))

	// creating the helper pod to perform container kill chaos
	err = CreateHelperPod(experimentsDetails, clients, appName, appNodeName)
	if err != nil {
		return errors.Errorf("Unable to create the helper pod, err: %v", err)
	}

	//checking the status of the helper pod, wait till the helper pod comes to running state else fail the experiment
	log.Info("[Status]: Checking the status of the helper pod")
	err = status.CheckApplicationStatus(experimentsDetails.ChaosNamespace, "name=container-kill-"+experimentsDetails.RunID, clients)
	if err != nil {
		return errors.Errorf("helper pod is not in running state, err: %v", err)
	}

	// Recording the chaos start timestamp
	ChaosStartTimeStamp := time.Now().Unix()

	// Wait till the completion of the helper pod
	// set an upper limit for the waiting time
	log.Info("[Wait]: waiting till the completion of the helper pod")
	err = status.WaitForCompletion(experimentsDetails.ChaosNamespace, "name=container-kill-"+experimentsDetails.RunID, clients, experimentsDetails.ChaosDuration+experimentsDetails.ChaosInterval+60)
	if err != nil {
		return err
	}

	//ChaosCurrentTimeStamp contains the current timestamp
	ChaosCurrentTimeStamp := time.Now().Unix()
	//ChaosDiffTimeStamp contains the difference of current timestamp and start timestamp
	//It will helpful to track the total chaos duration
	chaosDiffTimeStamp := ChaosCurrentTimeStamp - ChaosStartTimeStamp

	if int(chaosDiffTimeStamp) < experimentsDetails.ChaosDuration {
		return errors.Errorf("The helper pod failed, check the logs of helper pod for more details")
	}

	//Deleting the helper pod for container-kill chaos
	log.Info("[Cleanup]: Deleting the helper pod")
	err = DeleteHelperPod(experimentsDetails, clients)
	if err != nil {
		return errors.Errorf("Unable to delete the helper pod, err: %v", err)
	}

	// It will verify that the restart count of container should increase after chaos injection
	err = VerifyRestartCount(experimentsDetails, appName, clients, restartCountBefore)
	if err != nil {
		return errors.Errorf("Target container is not restarted , err: %v", err)
	}

	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", strconv.Itoa(experimentsDetails.RampTime))
		waitForDuration(experimentsDetails.RampTime)
	}
	return nil
}

//GetIterations derive the iterations value from given parameters
func GetIterations(experimentsDetails *experimentTypes.ExperimentDetails) {
	var Iterations int
	if experimentsDetails.ChaosInterval != 0 {
		Iterations = experimentsDetails.ChaosDuration / experimentsDetails.ChaosInterval
	}
	experimentsDetails.Iterations = math.Maximum(Iterations, 1)

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

// GetServiceAccount find the serviceAccountName for the helper pod
func GetServiceAccount(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) error {
	pod, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.ChaosNamespace).Get(experimentsDetails.ChaosPodName, v1.GetOptions{})
	if err != nil {
		return err
	}
	experimentsDetails.ChaosServiceAccount = pod.Spec.ServiceAccountName
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
				return errors.Errorf("Unable to get the application pod, err: %v", err)
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

//GetTargetContainer will fetch the conatiner name from application pod
//This container will be used as target container
func GetTargetContainer(experimentsDetails *experimentTypes.ExperimentDetails, appName string, clients clients.ClientSets) (string, error) {
	pod, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).Get(appName, v1.GetOptions{})
	if err != nil {
		return "", errors.Wrapf(err, "Fail to get the application pod status, due to:%v", err)
	}

	return pod.Spec.Containers[0].Name, nil
}

// CreateHelperPod derive the attributes for helper pod and create the helper pod
func CreateHelperPod(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, podName, nodeName string) error {

	privilegedEnable := true

	helperPod := &apiv1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:      "container-kill-" + experimentsDetails.RunID,
			Namespace: experimentsDetails.ChaosNamespace,
			Labels: map[string]string{
				"app":      "container-kill",
				"name":     "container-kill-" + experimentsDetails.RunID,
				"chaosUID": string(experimentsDetails.ChaosUID),
			},
		},
		Spec: apiv1.PodSpec{
			ServiceAccountName: experimentsDetails.ChaosServiceAccount,
			RestartPolicy:      apiv1.RestartPolicyNever,
			NodeName:           nodeName,
			Volumes: []apiv1.Volume{
				{
					Name: "cri-socket",
					VolumeSource: apiv1.VolumeSource{
						HostPath: &apiv1.HostPathVolumeSource{
							Path: experimentsDetails.ContainerPath,
						},
					},
				},
				{
					Name: "cri-config",
					VolumeSource: apiv1.VolumeSource{
						HostPath: &apiv1.HostPathVolumeSource{
							Path: "/etc/crictl.yaml",
						},
					},
				},
			},
			Containers: []apiv1.Container{
				{
					Name:            "container-kill",
					Image:           experimentsDetails.LIBImage,
					ImagePullPolicy: apiv1.PullAlways,
					Command: []string{
						"bin/bash",
					},
					Args: []string{
						"-c",
						"./experiments/container-kill",
					},
					Env: GetPodEnv(experimentsDetails, podName),
					VolumeMounts: []apiv1.VolumeMount{
						{
							Name:      "cri-socket",
							MountPath: experimentsDetails.ContainerPath,
						},
						{
							Name:      "cri-config",
							MountPath: "/etc/crictl.yaml",
						},
					},
					SecurityContext: &apiv1.SecurityContext{
						Privileged: &privilegedEnable,
					},
				},
			},
		},
	}

	_, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.ChaosNamespace).Create(helperPod)
	return err

}

//DeleteHelperPod deletes the helper pod and wait until it got terminated
func DeleteHelperPod(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) error {

	err := clients.KubeClient.CoreV1().Pods(experimentsDetails.ChaosNamespace).Delete("container-kill-"+experimentsDetails.RunID, &v1.DeleteOptions{})

	if err != nil {
		return err
	}

	// waiting for the termination of the pod
	err = retry.
		Times(90).
		Wait(1 * time.Second).
		Try(func(attempt uint) error {
			podSpec, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.ChaosNamespace).List(v1.ListOptions{LabelSelector: "name=container-kill-" + experimentsDetails.RunID})
			if err != nil || len(podSpec.Items) != 0 {
				return errors.Errorf("Pod is not deleted yet, err: %v", err)
			}
			return nil
		})

	return err
}

// GetPodEnv derive all the env required for the helper pod
func GetPodEnv(experimentsDetails *experimentTypes.ExperimentDetails, podName string) []apiv1.EnvVar {

	var envVar []apiv1.EnvVar
	ENVList := map[string]string{
		"APP_NS":               experimentsDetails.AppNS,
		"APP_POD":              podName,
		"APP_CONTAINER":        experimentsDetails.TargetContainer,
		"TOTAL_CHAOS_DURATION": strconv.Itoa(experimentsDetails.ChaosDuration),
		"CHAOS_NAMESPACE":      experimentsDetails.ChaosNamespace,
		"CHAOS_ENGINE":         experimentsDetails.EngineName,
		"CHAOS_UID":            string(experimentsDetails.ChaosUID),
		"CHAOS_INTERVAL":       strconv.Itoa(experimentsDetails.ChaosInterval),
		"ITERATIONS":           strconv.Itoa(experimentsDetails.Iterations),
	}
	for key, value := range ENVList {
		var perEnv apiv1.EnvVar
		perEnv.Name = key
		perEnv.Value = value
		envVar = append(envVar, perEnv)
	}
	// Getting experiment pod name from downward API
	experimentPodName := GetValueFromDownwardAPI("v1", "metadata.name")

	var downwardEnv apiv1.EnvVar
	downwardEnv.Name = "POD_NAME"
	downwardEnv.ValueFrom = &experimentPodName
	envVar = append(envVar, downwardEnv)

	return envVar
}

// GetValueFromDownwardAPI returns the value from downwardApi
func GetValueFromDownwardAPI(apiVersion string, fieldPath string) apiv1.EnvVarSource {
	downwardENV := apiv1.EnvVarSource{
		FieldRef: &apiv1.ObjectFieldSelector{
			APIVersion: apiVersion,
			FieldPath:  fieldPath,
		},
	}
	return downwardENV
}
