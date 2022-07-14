package lib

import (
	"context"
	"strconv"
	"strings"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/disk-fill/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/litmuschaos/litmus-go/pkg/utils/exec"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//PrepareDiskFill contains the prepration steps before chaos injection
func PrepareDiskFill(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	targetPodList := apiv1.PodList{}
	var err error
	var podsAffectedPerc int
	// It will contains all the pod & container details required for exec command
	execCommandDetails := exec.PodDetails{}
	// Get the target pod details for the chaos execution
	// if the target pod is not defined it will derive the random target pod list using pod affected percentage
	if experimentsDetails.TargetPods == "" && chaosDetails.AppDetail.Label == "" {
		return errors.Errorf("please provide one of the appLabel or TARGET_PODS")
	}
	//setup the tunables if provided in range
	setChaosTunables(experimentsDetails)

	log.InfoWithValues("[Info]: The chaos tunables are:", logrus.Fields{
		"FillPercentage":            experimentsDetails.FillPercentage,
		"EphemeralStorageMebibytes": experimentsDetails.EphemeralStorageMebibytes,
		"PodsAffectedPerc":          experimentsDetails.PodsAffectedPerc,
		"Sequence":                  experimentsDetails.Sequence,
	})

	podsAffectedPerc, _ = strconv.Atoi(experimentsDetails.PodsAffectedPerc)
	if experimentsDetails.NodeLabel == "" {
		targetPodList, err = common.GetPodList(experimentsDetails.TargetPods, podsAffectedPerc, clients, chaosDetails)
		if err != nil {
			return err
		}
	} else {
		if experimentsDetails.TargetPods == "" {
			targetPodList, err = common.GetPodListFromSpecifiedNodes(experimentsDetails.TargetPods, podsAffectedPerc, experimentsDetails.NodeLabel, clients, chaosDetails)
			if err != nil {
				return err
			}
		} else {
			log.Infof("TARGET_PODS env is provided, overriding the NODE_LABEL input")
			targetPodList, err = common.GetPodList(experimentsDetails.TargetPods, podsAffectedPerc, clients, chaosDetails)
			if err != nil {
				return err
			}
		}
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

	// Getting the serviceAccountName, need permission inside helper pod to create the events
	if experimentsDetails.ChaosServiceAccount == "" {
		experimentsDetails.ChaosServiceAccount, err = common.GetServiceAccount(experimentsDetails.ChaosNamespace, experimentsDetails.ChaosPodName, clients)
		if err != nil {
			return errors.Errorf("unable to get the serviceAccountName, err: %v", err)
		}
	}

	if experimentsDetails.EngineName != "" {
		if err := common.SetHelperData(chaosDetails, experimentsDetails.SetHelperData, clients); err != nil {
			return err
		}
	}

	experimentsDetails.IsTargetContainerProvided = (experimentsDetails.TargetContainer != "")
	switch strings.ToLower(experimentsDetails.Sequence) {
	case "serial":
		if err = injectChaosInSerialMode(experimentsDetails, targetPodList, clients, chaosDetails, execCommandDetails, resultDetails, eventsDetails); err != nil {
			return err
		}
	case "parallel":
		if err = injectChaosInParallelMode(experimentsDetails, targetPodList, clients, chaosDetails, execCommandDetails, resultDetails, eventsDetails); err != nil {
			return err
		}
	default:
		return errors.Errorf("%v sequence is not supported", experimentsDetails.Sequence)
	}

	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	return nil
}

// injectChaosInSerialMode fill the ephemeral storage of all target application serially (one by one)
func injectChaosInSerialMode(experimentsDetails *experimentTypes.ExperimentDetails, targetPodList apiv1.PodList, clients clients.ClientSets, chaosDetails *types.ChaosDetails, execCommandDetails exec.PodDetails, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails) error {

	labelSuffix := common.GetRunID()
	var err error
	// run the probes during chaos
	if len(resultDetails.ProbeDetails) != 0 {
		if err := probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
			return err
		}
	}

	// creating the helper pod to perform disk-fill chaos
	for _, pod := range targetPodList.Items {

		//Get the target container name of the application pod
		if !experimentsDetails.IsTargetContainerProvided {
			experimentsDetails.TargetContainer, err = common.GetTargetContainer(experimentsDetails.AppNS, pod.Name, clients)
			if err != nil {
				return errors.Errorf("unable to get the target container name, err: %v", err)
			}
		}

		runID := common.GetRunID()
		if err := createHelperPod(experimentsDetails, clients, chaosDetails, pod.Name, pod.Spec.NodeName, runID, labelSuffix); err != nil {
			return errors.Errorf("unable to create the helper pod, err: %v", err)
		}

		appLabel := "name=" + experimentsDetails.ExperimentName + "-helper-" + runID

		//checking the status of the helper pods, wait till the pod comes to running state else fail the experiment
		log.Info("[Status]: Checking the status of the helper pods")
		if err := status.CheckHelperStatus(experimentsDetails.ChaosNamespace, appLabel, experimentsDetails.Timeout, experimentsDetails.Delay, clients); err != nil {
			common.DeleteHelperPodBasedOnJobCleanupPolicy(experimentsDetails.ExperimentName+"-helper-"+runID, appLabel, chaosDetails, clients)
			return errors.Errorf("helper pods are not in running state, err: %v", err)
		}

		// Wait till the completion of the helper pod
		// set an upper limit for the waiting time
		log.Info("[Wait]: waiting till the completion of the helper pod")
		podStatus, err := status.WaitForCompletion(experimentsDetails.ChaosNamespace, appLabel, clients, experimentsDetails.ChaosDuration+experimentsDetails.Timeout, experimentsDetails.ExperimentName)
		if err != nil || podStatus == "Failed" {
			common.DeleteHelperPodBasedOnJobCleanupPolicy(experimentsDetails.ExperimentName+"-helper-"+runID, appLabel, chaosDetails, clients)
			return common.HelperFailedError(err)
		}

		//Deleting all the helper pod for disk-fill chaos
		log.Info("[Cleanup]: Deleting the helper pod")
		if err = common.DeletePod(experimentsDetails.ExperimentName+"-helper-"+runID, appLabel, experimentsDetails.ChaosNamespace, chaosDetails.Timeout, chaosDetails.Delay, clients); err != nil {
			return errors.Errorf("unable to delete the helper pod, %v", err)
		}
	}

	return nil

}

// injectChaosInParallelMode fill the ephemeral storage of of all target application in parallel mode (all at once)
func injectChaosInParallelMode(experimentsDetails *experimentTypes.ExperimentDetails, targetPodList apiv1.PodList, clients clients.ClientSets, chaosDetails *types.ChaosDetails, execCommandDetails exec.PodDetails, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails) error {

	labelSuffix := common.GetRunID()
	var err error
	// run the probes during chaos
	if len(resultDetails.ProbeDetails) != 0 {
		if err := probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
			return err
		}
	}

	// creating the helper pod to perform disk-fill chaos
	for _, pod := range targetPodList.Items {

		//Get the target container name of the application pod
		if !experimentsDetails.IsTargetContainerProvided {
			experimentsDetails.TargetContainer, err = common.GetTargetContainer(experimentsDetails.AppNS, pod.Name, clients)
			if err != nil {
				return errors.Errorf("unable to get the target container name, err: %v", err)
			}
		}

		runID := common.GetRunID()
		if err := createHelperPod(experimentsDetails, clients, chaosDetails, pod.Name, pod.Spec.NodeName, runID, labelSuffix); err != nil {
			return errors.Errorf("unable to create the helper pod, err: %v", err)
		}
	}

	appLabel := "app=" + experimentsDetails.ExperimentName + "-helper-" + labelSuffix

	//checking the status of the helper pods, wait till the pod comes to running state else fail the experiment
	log.Info("[Status]: Checking the status of the helper pods")
	if err := status.CheckHelperStatus(experimentsDetails.ChaosNamespace, appLabel, experimentsDetails.Timeout, experimentsDetails.Delay, clients); err != nil {
		common.DeleteAllHelperPodBasedOnJobCleanupPolicy(appLabel, chaosDetails, clients)
		return errors.Errorf("helper pods are not in running state, err: %v", err)
	}

	// Wait till the completion of the helper pod
	// set an upper limit for the waiting time
	log.Info("[Wait]: waiting till the completion of the helper pod")
	podStatus, err := status.WaitForCompletion(experimentsDetails.ChaosNamespace, appLabel, clients, experimentsDetails.ChaosDuration+experimentsDetails.Timeout, experimentsDetails.ExperimentName)
	if err != nil || podStatus == "Failed" {
		common.DeleteAllHelperPodBasedOnJobCleanupPolicy(appLabel, chaosDetails, clients)
		return common.HelperFailedError(err)
	}

	//Deleting all the helper pod for disk-fill chaos
	log.Info("[Cleanup]: Deleting all the helper pod")
	if err = common.DeleteAllPod(appLabel, experimentsDetails.ChaosNamespace, chaosDetails.Timeout, chaosDetails.Delay, clients); err != nil {
		return errors.Errorf("unable to delete the helper pod, %v", err)
	}

	return nil
}

// createHelperPod derive the attributes for helper pod and create the helper pod
func createHelperPod(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails, appName, appNodeName, runID, labelSuffix string) error {

	mountPropagationMode := apiv1.MountPropagationHostToContainer
	terminationGracePeriodSeconds := int64(experimentsDetails.TerminationGracePeriodSeconds)

	helperPod := &apiv1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:        experimentsDetails.ExperimentName + "-helper-" + runID,
			Namespace:   experimentsDetails.ChaosNamespace,
			Labels:      common.GetHelperLabels(chaosDetails.Labels, runID, labelSuffix, experimentsDetails.ExperimentName),
			Annotations: chaosDetails.Annotations,
		},
		Spec: apiv1.PodSpec{
			RestartPolicy:                 apiv1.RestartPolicyNever,
			ImagePullSecrets:              chaosDetails.ImagePullSecrets,
			NodeName:                      appNodeName,
			ServiceAccountName:            experimentsDetails.ChaosServiceAccount,
			TerminationGracePeriodSeconds: &terminationGracePeriodSeconds,
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
					Command: []string{
						"/bin/bash",
					},
					Args: []string{
						"-c",
						"./helpers -name disk-fill",
					},
					Resources: chaosDetails.Resources,
					Env:       getPodEnv(experimentsDetails, appName),
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

	_, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.ChaosNamespace).Create(context.Background(), helperPod, v1.CreateOptions{})
	return err
}

// getPodEnv derive all the env required for the helper pod
func getPodEnv(experimentsDetails *experimentTypes.ExperimentDetails, podName string) []apiv1.EnvVar {

	var envDetails common.ENVDetails
	envDetails.SetEnv("APP_NAMESPACE", experimentsDetails.AppNS).
		SetEnv("APP_POD", podName).
		SetEnv("APP_CONTAINER", experimentsDetails.TargetContainer).
		SetEnv("TOTAL_CHAOS_DURATION", strconv.Itoa(experimentsDetails.ChaosDuration)).
		SetEnv("CHAOS_NAMESPACE", experimentsDetails.ChaosNamespace).
		SetEnv("CHAOSENGINE", experimentsDetails.EngineName).
		SetEnv("CHAOS_UID", string(experimentsDetails.ChaosUID)).
		SetEnv("EXPERIMENT_NAME", experimentsDetails.ExperimentName).
		SetEnv("FILL_PERCENTAGE", experimentsDetails.FillPercentage).
		SetEnv("EPHEMERAL_STORAGE_MEBIBYTES", experimentsDetails.EphemeralStorageMebibytes).
		SetEnv("DATA_BLOCK_SIZE", strconv.Itoa(experimentsDetails.DataBlockSize)).
		SetEnv("INSTANCE_ID", experimentsDetails.InstanceID).
		SetEnvFromDownwardAPI("v1", "metadata.name")

	return envDetails.ENV
}

//setChaosTunables will setup a random value within a given range of values
//If the value is not provided in range it'll setup the initial provided value.
func setChaosTunables(experimentsDetails *experimentTypes.ExperimentDetails) {
	experimentsDetails.FillPercentage = common.ValidateRange(experimentsDetails.FillPercentage)
	experimentsDetails.EphemeralStorageMebibytes = common.ValidateRange(experimentsDetails.EphemeralStorageMebibytes)
	experimentsDetails.PodsAffectedPerc = common.ValidateRange(experimentsDetails.PodsAffectedPerc)
	experimentsDetails.Sequence = common.GetRandomSequence(experimentsDetails.Sequence)
}
