package lib

import (
	"strconv"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/pod-cpu-hog/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var err error

// PreparePodCPUHog contains prepration steps before chaos injection
func PreparePodCPUHog(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	targetPodList, err := common.GetPodList(experimentsDetails.TargetPods, experimentsDetails.PodsAffectedPerc, clients, chaosDetails)
	if err != nil {
		return err
	}

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	if experimentsDetails.EngineName != "" {
		msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on target pod"
		types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
		events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
	}

	if experimentsDetails.EngineName != "" {
		// Get Chaos Pod Annotation
		experimentsDetails.Annotations, err = common.GetChaosPodAnnotation(experimentsDetails.ChaosPodName, experimentsDetails.ChaosNamespace, clients)
		if err != nil {
			return errors.Errorf("unable to get annotations, err: %v", err)
		}
	}

	if experimentsDetails.Sequence == "serial" {
		if err = InjectChaosInSerialMode(experimentsDetails, targetPodList, clients, chaosDetails); err != nil {
			return err
		}
	} else {
		if err = InjectChaosInParallelMode(experimentsDetails, targetPodList, clients, chaosDetails); err != nil {
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

// InjectChaosInSerialMode stress the cpu of all target application serially (one by one)
func InjectChaosInSerialMode(experimentsDetails *experimentTypes.ExperimentDetails, targetPodList apiv1.PodList, clients clients.ClientSets, chaosDetails *types.ChaosDetails) error {

	// creating the helper pod to perform cpu chaos
	for _, pod := range targetPodList.Items {

		runID := common.GetRunID()

		log.InfoWithValues("[Info]: Details of application under chaos injection", logrus.Fields{
			"PodName":  pod.Name,
			"NodeName": pod.Spec.NodeName,
			"CPUcores": experimentsDetails.CPUcores,
		})

		err = CreateHelperPod(experimentsDetails, clients, pod.Name, pod.Spec.NodeName, runID)
		if err != nil {
			return errors.Errorf("Unable to create the helper pod, err: %v", err)
		}

		//checking the status of the helper pod, wait till the pod comes to running state else fail the experiment
		log.Info("[Status]: Checking the status of the helper pod")
		err = status.CheckApplicationStatus(experimentsDetails.ChaosNamespace, "app="+experimentsDetails.ExperimentName+"-helper", experimentsDetails.Timeout, experimentsDetails.Delay, clients)
		if err != nil {
			return errors.Errorf("helper pod is not in running state, err: %v", err)
		}

		// Wait till the completion of helper pod
		log.Infof("[Wait]: Waiting for %vs till the completion of the helper pod", strconv.Itoa(experimentsDetails.ChaosDuration+30))
		podStatus, err := status.WaitForCompletion(experimentsDetails.ChaosNamespace, "app="+experimentsDetails.ExperimentName+"-helper", clients, experimentsDetails.ChaosDuration+30, "pumba-stress")
		if err != nil || podStatus == "Failed" {
			return errors.Errorf("helper pod failed due to, err: %v", err)
		}

		//Deleting the helper pod
		log.Info("[Cleanup]: Deleting the helper pod")
		err = common.DeletePod(experimentsDetails.ExperimentName+"-"+runID, "app="+experimentsDetails.ExperimentName+"-helper", experimentsDetails.ChaosNamespace, chaosDetails.Timeout, chaosDetails.Delay, clients)
		if err != nil {
			return errors.Errorf("Unable to delete the helper pod, err: %v", err)
		}
	}

	return nil
}

// InjectChaosInParallelMode kill the container of all target application in parallel mode (all at once)
func InjectChaosInParallelMode(experimentsDetails *experimentTypes.ExperimentDetails, targetPodList apiv1.PodList, clients clients.ClientSets, chaosDetails *types.ChaosDetails) error {

	// creating the helper pod to perform cpu chaos
	for _, pod := range targetPodList.Items {

		runID := common.GetRunID()

		log.InfoWithValues("[Info]: Details of application under chaos injection", logrus.Fields{
			"PodName":  pod.Name,
			"NodeName": pod.Spec.NodeName,
			"CPUcores": experimentsDetails.CPUcores,
		})

		err = CreateHelperPod(experimentsDetails, clients, pod.Name, pod.Spec.NodeName, runID)
		if err != nil {
			return errors.Errorf("Unable to create the helper pod, err: %v", err)
		}
	}

	//checking the status of the helper pod, wait till the pod comes to running state else fail the experiment
	log.Info("[Status]: Checking the status of the helper pod")
	err = status.CheckApplicationStatus(experimentsDetails.ChaosNamespace, "app="+experimentsDetails.ExperimentName+"-helper", experimentsDetails.Timeout, experimentsDetails.Delay, clients)
	if err != nil {
		return errors.Errorf("helper pod is not in running state, err: %v", err)
	}

	// Wait till the completion of helper pod
	log.Infof("[Wait]: Waiting for %vs till the completion of the helper pod", strconv.Itoa(experimentsDetails.ChaosDuration+30))
	podStatus, err := status.WaitForCompletion(experimentsDetails.ChaosNamespace, "app="+experimentsDetails.ExperimentName+"-helper", clients, experimentsDetails.ChaosDuration+30, "pumba-stress")
	if err != nil || podStatus == "Failed" {
		return errors.Errorf("helper pod failed due to, err: %v", err)
	}

	//Deleting the helper pod
	log.Info("[Cleanup]: Deleting the helper pod")
	err = common.DeleteAllPod("app="+experimentsDetails.ExperimentName+"-helper", experimentsDetails.ChaosNamespace, chaosDetails.Timeout, chaosDetails.Delay, clients)
	if err != nil {
		return errors.Errorf("Unable to delete the helper pod, err: %v", err)
	}

	return nil
}

// CreateHelperPod derive the attributes for helper pod and create the helper pod
func CreateHelperPod(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, appName, appNodeName, runID string) error {

	helperPod := &apiv1.Pod{
		TypeMeta: v1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      experimentsDetails.ExperimentName + "-" + runID,
			Namespace: experimentsDetails.ChaosNamespace,
			Labels: map[string]string{
				"app":                       experimentsDetails.ExperimentName + "-helper",
				"name":                      experimentsDetails.ExperimentName + "-" + runID,
				"chaosUID":                  string(experimentsDetails.ChaosUID),
				"app.kubernetes.io/part-of": "litmus",
				// prevent pumba from killing itself
				"com.gaiaadm.pumba": "true",
			},
			Annotations: experimentsDetails.Annotations,
		},
		Spec: apiv1.PodSpec{
			RestartPolicy: apiv1.RestartPolicyNever,
			NodeName:      appNodeName,
			Volumes: []apiv1.Volume{
				{
					Name: "dockersocket",
					VolumeSource: apiv1.VolumeSource{
						HostPath: &apiv1.HostPathVolumeSource{
							Path: "/var/run/docker.sock",
						},
					},
				},
			},
			Containers: []apiv1.Container{
				{
					Name:  "pumba-stress",
					Image: experimentsDetails.LIBImage,
					Args:  GetContainerArguments(experimentsDetails, appName),
					VolumeMounts: []apiv1.VolumeMount{
						{
							Name:      "dockersocket",
							MountPath: "/var/run/docker.sock",
						},
					},
					ImagePullPolicy: apiv1.PullPolicy(experimentsDetails.LIBImagePullPolicy),
					SecurityContext: &apiv1.SecurityContext{
						Capabilities: &apiv1.Capabilities{
							Add: []apiv1.Capability{
								"SYS_ADMIN",
							},
						},
					},
				},
			},
		},
	}

	_, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.ChaosNamespace).Create(helperPod)
	return err
}

// GetContainerArguments derives the args for the pumba stress helper pod
func GetContainerArguments(experimentsDetails *experimentTypes.ExperimentDetails, appName string) []string {
	stressArgs := []string{

		"--log-level",
		"debug",
		"--label",
		"io.kubernetes.pod.name=" + appName,
		"stress",
		"--duration",
		strconv.Itoa(experimentsDetails.ChaosDuration) + "s",
		"--stressors",
		"--cpu " + strconv.Itoa(experimentsDetails.CPUcores) + " --timeout " + strconv.Itoa(experimentsDetails.ChaosDuration) + "s",
	}
	return stressArgs
}
