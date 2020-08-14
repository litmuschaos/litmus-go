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
	"github.com/litmuschaos/litmus-go/pkg/utils/regular"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//PrepareContainerKill contains the prepration steps before chaos injection
func PrepareContainerKill(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	//Select application pod and node name for the container-kill
	appName, appNodeName, err := GetApplicationPod(experimentsDetails, clients)
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

	log.InfoWithValues("[Info]: Details of application under chaos injection", logrus.Fields{
		"PodName":       appName,
		"NodeName":      appNodeName,
		"ContainerName": experimentsDetails.TargetContainer,
	})

	//Getting the iteration count for the container-kill
	GetIterations(experimentsDetails)

	// generating a unique string which can be appended with the helper pod name & labels for the uniquely identification
	experimentsDetails.RunID = regular.GetRunID()

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
		regular.WaitForDuration(experimentsDetails.RampTime)
	}

	// creating the helper pod to perform container kill chaos
	err = CreateHelperPod(experimentsDetails, clients, appName, appNodeName)
	if err != nil {
		return errors.Errorf("Unable to create the helper pod, err: %v", err)
	}

	//checking the status of the helper pod, wait till the helper pod comes to running state else fail the experiment
	log.Info("[Status]: Checking the status of the helper pod")
	err = status.CheckApplicationStatus(experimentsDetails.ChaosNamespace, "name="+experimentsDetails.ExperimentName+"-"+experimentsDetails.RunID, experimentsDetails.Timeout, experimentsDetails.Delay, clients)
	if err != nil {
		return errors.Errorf("helper pod is not in running state, err: %v", err)
	}

	// Wait till the completion of the helper pod
	// set an upper limit for the waiting time
	log.Info("[Wait]: waiting till the completion of the helper pod")
	podStatus, err := status.WaitForCompletion(experimentsDetails.ChaosNamespace, "name="+experimentsDetails.ExperimentName+"-"+experimentsDetails.RunID, clients, experimentsDetails.ChaosDuration+experimentsDetails.ChaosInterval+60, "container-kill")
	if err != nil || podStatus == "Failed" {
		return errors.Errorf("helper pod failed due to, err: %v", err)
	}

	//Deleting the helper pod for container-kill chaos
	log.Info("[Cleanup]: Deleting the helper pod")
	err = regular.DeletePod(experimentsDetails.ExperimentName+"-"+experimentsDetails.RunID, "name="+experimentsDetails.ExperimentName+"-"+experimentsDetails.RunID, experimentsDetails.ChaosNamespace, chaosDetails.Timeout, chaosDetails.Delay, clients)
	if err != nil {
		return errors.Errorf("Unable to delete the helper pod, err: %v", err)
	}

	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", strconv.Itoa(experimentsDetails.RampTime))
		regular.WaitForDuration(experimentsDetails.RampTime)
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

//GetTargetContainer will fetch the container name from application pod
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
			Name:      experimentsDetails.ExperimentName + "-" + experimentsDetails.RunID,
			Namespace: experimentsDetails.ChaosNamespace,
			Labels: map[string]string{
				"app":      experimentsDetails.ExperimentName,
				"name":     experimentsDetails.ExperimentName + "-" + experimentsDetails.RunID,
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
					Name:            experimentsDetails.ExperimentName,
					Image:           experimentsDetails.LIBImage,
					ImagePullPolicy: apiv1.PullAlways,
					Command: []string{
						"bin/bash",
					},
					Args: []string{
						"-c",
						"./experiments/container-killer",
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
