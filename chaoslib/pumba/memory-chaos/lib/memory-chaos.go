package lib

import (
	"strconv"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/pod-memory-hog/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var err error

// PreparePodMemoryHog contains prepration steps before chaos injection
func PreparePodMemoryHog(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	// Get the target pod details for the chaos execution
	// if the target pod is not defined it will derive the random target pod list using pod affected percentage
	if experimentsDetails.TargetPods == "" && chaosDetails.AppDetail.Label == "" {
		return errors.Errorf("Please provide one of the appLabel or TARGET_PODS")
	}
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
		// Get Resource Requirements
		experimentsDetails.Resources, err = common.GetChaosPodResourceRequirements(experimentsDetails.ChaosPodName, experimentsDetails.ExperimentName, experimentsDetails.ChaosNamespace, clients)
		if err != nil {
			return errors.Errorf("Unable to get resource requirements, err: %v", err)
		}
		// Get ImagePullSecrets
		experimentsDetails.ImagePullSecrets, err = common.GetImagePullSecrets(experimentsDetails.ChaosPodName, experimentsDetails.ChaosNamespace, clients)
		if err != nil {
			return errors.Errorf("Unable to get imagePullSecrets, err: %v", err)
		}
	}

	if experimentsDetails.Sequence == "serial" {
		if err = InjectChaosInSerialMode(experimentsDetails, targetPodList, clients, chaosDetails, resultDetails, eventsDetails); err != nil {
			return err
		}
	} else {
		if err = InjectChaosInParallelMode(experimentsDetails, targetPodList, clients, chaosDetails, resultDetails, eventsDetails); err != nil {
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
func InjectChaosInSerialMode(experimentsDetails *experimentTypes.ExperimentDetails, targetPodList apiv1.PodList, clients clients.ClientSets, chaosDetails *types.ChaosDetails, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails) error {

	labelSuffix := common.GetRunID()

	// run the probes during chaos
	if len(resultDetails.ProbeDetails) != 0 {
		if err := probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
			return err
		}
	}

	// creating the helper pod to perform memory chaos
	for _, pod := range targetPodList.Items {

		runID := common.GetRunID()

		log.InfoWithValues("[Info]: Details of application under chaos injection", logrus.Fields{
			"Target Pod":  pod.Name,
			"NodeName":    pod.Spec.NodeName,
			"MemoryBytes": experimentsDetails.MemoryConsumption,
		})

		err = CreateHelperPod(experimentsDetails, clients, pod.Name, pod.Spec.NodeName, runID, labelSuffix)
		if err != nil {
			return errors.Errorf("Unable to create the helper pod, err: %v", err)
		}

		appLabel := "name=" + experimentsDetails.ExperimentName + "-helper-" + runID

		//checking the status of the helper pod, wait till the pod comes to running state else fail the experiment
		log.Info("[Status]: Checking the status of the helper pod")
		err = status.CheckHelperStatus(experimentsDetails.ChaosNamespace, appLabel, experimentsDetails.Timeout, experimentsDetails.Delay, clients)
		if err != nil {
			common.DeleteHelperPodBasedOnJobCleanupPolicy(experimentsDetails.ExperimentName+"-helper-"+runID, appLabel, chaosDetails, clients)
			return errors.Errorf("helper pod is not in running state, err: %v", err)
		}

		// Wait till the completion of helper pod
		log.Infof("[Wait]: Waiting for %vs till the completion of the helper pod", strconv.Itoa(experimentsDetails.ChaosDuration+30))
		podStatus, err := status.WaitForCompletion(experimentsDetails.ChaosNamespace, appLabel, clients, experimentsDetails.ChaosDuration+30, "pumba-stress")
		if err != nil || podStatus == "Failed" {
			common.DeleteHelperPodBasedOnJobCleanupPolicy(experimentsDetails.ExperimentName+"-helper-"+runID, appLabel, chaosDetails, clients)
			return errors.Errorf("helper pod failed due to, err: %v", err)
		}

		//Deleting the helper pod
		log.Info("[Cleanup]: Deleting the helper pod")
		err = common.DeletePod(experimentsDetails.ExperimentName+"-helper-"+runID, appLabel, experimentsDetails.ChaosNamespace, chaosDetails.Timeout, chaosDetails.Delay, clients)
		if err != nil {
			return errors.Errorf("Unable to delete the helper pod, err: %v", err)
		}
	}

	return nil
}

// InjectChaosInParallelMode kill the container of all target application in parallel mode (all at once)
func InjectChaosInParallelMode(experimentsDetails *experimentTypes.ExperimentDetails, targetPodList apiv1.PodList, clients clients.ClientSets, chaosDetails *types.ChaosDetails, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails) error {

	labelSuffix := common.GetRunID()

	// run the probes during chaos
	if len(resultDetails.ProbeDetails) != 0 {
		if err := probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
			return err
		}
	}

	// creating the helper pod to perform memory chaos
	for _, pod := range targetPodList.Items {

		runID := common.GetRunID()

		log.InfoWithValues("[Info]: Details of application under chaos injection", logrus.Fields{
			"Target Pod":  pod.Name,
			"NodeName":    pod.Spec.NodeName,
			"MemoryBytes": experimentsDetails.MemoryConsumption,
		})

		err = CreateHelperPod(experimentsDetails, clients, pod.Name, pod.Spec.NodeName, runID, labelSuffix)
		if err != nil {
			return errors.Errorf("Unable to create the helper pod, err: %v", err)
		}
	}

	appLabel := "app=" + experimentsDetails.ExperimentName + "-helper-" + labelSuffix

	//checking the status of the helper pod, wait till the pod comes to running state else fail the experiment
	log.Info("[Status]: Checking the status of the helper pod")
	err = status.CheckHelperStatus(experimentsDetails.ChaosNamespace, appLabel, experimentsDetails.Timeout, experimentsDetails.Delay, clients)
	if err != nil {
		common.DeleteAllHelperPodBasedOnJobCleanupPolicy(appLabel, chaosDetails, clients)
		return errors.Errorf("helper pod is not in running state, err: %v", err)
	}

	// Wait till the completion of helper pod
	log.Infof("[Wait]: Waiting for %vs till the completion of the helper pod", strconv.Itoa(experimentsDetails.ChaosDuration+30))
	podStatus, err := status.WaitForCompletion(experimentsDetails.ChaosNamespace, appLabel, clients, experimentsDetails.ChaosDuration+30, "pumba-stress")
	if err != nil || podStatus == "Failed" {
		common.DeleteAllHelperPodBasedOnJobCleanupPolicy(appLabel, chaosDetails, clients)
		return errors.Errorf("helper pod failed due to, err: %v", err)
	}

	//Deleting the helper pod
	log.Info("[Cleanup]: Deleting the helper pod")
	err = common.DeleteAllPod(appLabel, experimentsDetails.ChaosNamespace, chaosDetails.Timeout, chaosDetails.Delay, clients)
	if err != nil {
		return errors.Errorf("Unable to delete the helper pod, err: %v", err)
	}

	return nil
}

// CreateHelperPod derive the attributes for helper pod and create the helper pod
func CreateHelperPod(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, appName, appNodeName, runID, labelSuffix string) error {

	helperPod := &apiv1.Pod{
		TypeMeta: v1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      experimentsDetails.ExperimentName + "-helper-" + runID,
			Namespace: experimentsDetails.ChaosNamespace,
			Labels: map[string]string{
				"app":                       experimentsDetails.ExperimentName + "-helper-" + labelSuffix,
				"name":                      experimentsDetails.ExperimentName + "-helper-" + runID,
				"chaosUID":                  string(experimentsDetails.ChaosUID),
				"app.kubernetes.io/part-of": "litmus",
				// prevent pumba from killing itself
				"com.gaiaadm.pumba": "true",
			},
			Annotations: experimentsDetails.Annotations,
		},
		Spec: apiv1.PodSpec{
			RestartPolicy:    apiv1.RestartPolicyNever,
			ImagePullSecrets: experimentsDetails.ImagePullSecrets,
			NodeName:         appNodeName,
			Volumes: []apiv1.Volume{
				{
					Name: "dockersocket",
					VolumeSource: apiv1.VolumeSource{
						HostPath: &apiv1.HostPathVolumeSource{
							Path: experimentsDetails.SocketPath,
						},
					},
				},
			},
			InitContainers: []apiv1.Container{
				{
					Name:            "setup-pumba-stress",
					Image:           experimentsDetails.LIBImage,
					ImagePullPolicy: apiv1.PullPolicy(experimentsDetails.LIBImagePullPolicy),
					Command: []string{
						"/bin/bash",
						"-c",
						"sudo chmod 777 " + experimentsDetails.SocketPath,
					},
					VolumeMounts: []apiv1.VolumeMount{
						{
							Name:      "dockersocket",
							MountPath: experimentsDetails.SocketPath,
						},
					},
				},
			},
			Containers: []apiv1.Container{
				{
					Name:  "pumba-stress",
					Image: experimentsDetails.LIBImage,
					Command: []string{
						"pumba",
					},
					Args:      GetContainerArguments(experimentsDetails, appName),
					Resources: experimentsDetails.Resources,
					VolumeMounts: []apiv1.VolumeMount{
						{
							Name:      "dockersocket",
							MountPath: experimentsDetails.SocketPath,
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
		"--stress-image",
		experimentsDetails.StressImage,
		"--stressors",
		"--cpu 1 --vm 1 --vm-bytes " + strconv.Itoa(experimentsDetails.MemoryConsumption) + "M --timeout " + strconv.Itoa(experimentsDetails.ChaosDuration) + "s",
	}
	return stressArgs
}
