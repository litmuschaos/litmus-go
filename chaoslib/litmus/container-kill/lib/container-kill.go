package lib

import (
	"strconv"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/container-kill/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/math"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/pkg/errors"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//PrepareContainerKill contains the prepration steps before chaos injection
func PrepareContainerKill(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	// Get the target pod details for the chaos execution
	// if the target pod is not defined it will derive the random target pod list using pod affected percentage
	targetPodList, err := common.GetPodList(experimentsDetails.AppNS, experimentsDetails.TargetPod, experimentsDetails.AppLabel, experimentsDetails.PodsAffectedPerc, clients)
	if err != nil {
		return errors.Errorf("Unable to get the target pod list, err: %v", err)
	}

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	// Getting the serviceAccountName, need permission inside helper pod to create the events
	if experimentsDetails.ChaosServiceAccount == "" {
		err = GetServiceAccount(experimentsDetails, clients)
		if err != nil {
			return errors.Errorf("Unable to get the serviceAccountName, err: %v", err)
		}
	}

	//Get the target container name of the application pod
	if experimentsDetails.TargetContainer == "" {
		experimentsDetails.TargetContainer, err = GetTargetContainer(experimentsDetails, targetPodList.Items[0].Name, clients)
		if err != nil {
			return errors.Errorf("Unable to get the target container name, err: %v", err)
		}
	}

	//Getting the iteration count for the container-kill
	GetIterations(experimentsDetails)

	// Get Chaos Pod Annotation
	experimentsDetails.Annotations, err = common.GetChaosPodAnnotation(experimentsDetails.ChaosPodName, experimentsDetails.ChaosNamespace, clients)
	if err != nil {
		return errors.Errorf("Unable to get annotations, err: %v", err)
	}

	// creating the helper pod to perform container kill chaos
	for _, pod := range targetPodList.Items {
		runID := common.GetRunID()
		err = CreateHelperPod(experimentsDetails, clients, pod.Name, pod.Spec.NodeName, runID)
		if err != nil {
			return errors.Errorf("Unable to create the helper pod, err: %v", err)
		}
	}

	//checking the status of the helper pods, wait till the pod comes to running state else fail the experiment
	log.Info("[Status]: Checking the status of the helper pods")
	err = status.CheckApplicationStatus(experimentsDetails.ChaosNamespace, "app="+experimentsDetails.ExperimentName+"-helper", experimentsDetails.Timeout, experimentsDetails.Delay, clients)
	if err != nil {
		return errors.Errorf("helper pods are not in running state, err: %v", err)
	}

	// Wait till the completion of the helper pod
	// set an upper limit for the waiting time
	log.Info("[Wait]: waiting till the completion of the helper pod")
	podStatus, err := status.WaitForCompletion(experimentsDetails.ChaosNamespace, "app="+experimentsDetails.ExperimentName+"-helper", clients, experimentsDetails.ChaosDuration+experimentsDetails.ChaosInterval+60, experimentsDetails.ExperimentName)
	if err != nil || podStatus == "Failed" {
		return errors.Errorf("helper pod failed, err: %v", err)
	}

	//Deleting all the helper pod for container-kill chaos
	log.Info("[Cleanup]: Deleting all the helper pods")
	err = common.DeleteAllPod("app="+experimentsDetails.ExperimentName+"-helper", experimentsDetails.ChaosNamespace, chaosDetails.Timeout, chaosDetails.Delay, clients)
	if err != nil {
		return errors.Errorf("Unable to delete the helper pod, err: %v", err)
	}

	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
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

//GetTargetContainer will fetch the container name from application pod
//This container will be used as target container
func GetTargetContainer(experimentsDetails *experimentTypes.ExperimentDetails, appName string, clients clients.ClientSets) (string, error) {
	pod, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).Get(appName, v1.GetOptions{})
	if err != nil {
		return "", err
	}

	return pod.Spec.Containers[0].Name, nil
}

// CreateHelperPod derive the attributes for helper pod and create the helper pod
func CreateHelperPod(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, podName, nodeName, runID string) error {

	privilegedEnable := false
	if experimentsDetails.ContainerRuntime == "crio" {
		privilegedEnable = true
	}

	helperPod := &apiv1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:      experimentsDetails.ExperimentName + "-" + runID,
			Namespace: experimentsDetails.ChaosNamespace,
			Labels: map[string]string{
				"app":      experimentsDetails.ExperimentName + "-helper",
				"name":     experimentsDetails.ExperimentName + "-" + runID,
				"chaosUID": string(experimentsDetails.ChaosUID),
			},
			Annotations: experimentsDetails.Annotations,
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
							Path: experimentsDetails.SocketPath,
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
						"/bin/bash",
					},
					Args: []string{
						"-c",
						"./helper/container-killer",
					},
					Env: GetPodEnv(experimentsDetails, podName),
					VolumeMounts: []apiv1.VolumeMount{
						{
							Name:      "cri-socket",
							MountPath: experimentsDetails.SocketPath,
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
