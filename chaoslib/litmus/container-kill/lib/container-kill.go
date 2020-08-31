package lib

import (
	"strconv"
	"time"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/container-kill/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/math"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//PrepareContainerKill contains the prepration steps before chaos injection
func PrepareContainerKill(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	// Get the target pod details for the chaos execution
	// if the target pod is not defined it will derive the random target pod list using pod affected percentage
	targetPodList, err := common.GetPodList(experimentsDetails.AppNS, experimentsDetails.TargetPod, experimentsDetails.AppLabel, experimentsDetails.PodsAffectedPerc, clients)
	if err != nil {
		return errors.Errorf("Unable to get the target pod list due to, err: %v", err)
	}

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", strconv.Itoa(experimentsDetails.RampTime))
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	// generating a unique string which can be appended with the helper pod name & labels for the uniquely identification
	experimentsDetails.RunID = common.GetRunID()

	// Getting the serviceAccountName, need permission inside helper pod to create the events
	if experimentsDetails.ChaosServiceAccount == "" {
		err = GetServiceAccount(experimentsDetails, clients)
		if err != nil {
			return errors.Errorf("Unable to get the serviceAccountName, err: %v", err)
		}
	}

	//Getting the iteration count for the container-kill
	GetIterations(experimentsDetails)

	// creating the helper daemonset to perform container kill chaos
	if err = CreateHelperDaemonset(experimentsDetails, clients); err != nil {
		return errors.Errorf("Unable to create the helper daemonset, err: %v", err)
	}

	//checking the status of the helper daemonset, wait till the pod comes to running state else fail the experiment
	log.Info("[Status]: Checking the status of the helper daemonset pods")
	err = status.CheckApplicationStatus(experimentsDetails.ChaosNamespace, "name="+experimentsDetails.ExperimentName+"-"+experimentsDetails.RunID, experimentsDetails.Timeout, experimentsDetails.Delay, clients)
	if err != nil {
		return errors.Errorf("helper daemonset pods is not in running state, err: %v", err)
	}

	var endTime <-chan time.Time
	timeDelay := time.Duration(experimentsDetails.ChaosDuration) * time.Second

	for count := 0; count < experimentsDetails.Iterations; count++ {
		for _, pod := range targetPodList.Items {

			//Get the target container name of the application pod
			if experimentsDetails.TargetContainer == "" {
				experimentsDetails.TargetContainer, err = GetTargetContainer(experimentsDetails, pod.Name, clients)
				if err != nil {
					return errors.Errorf("Unable to get the target container name due to, err: %v", err)
				}
			}

			// kill the container
			if err = KillContainer(experimentsDetails, clients, chaosDetails, pod); err != nil {
				return err
			}

			// create the events
			if experimentsDetails.EngineName != "" {
				msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on " + pod.Name + " pod"
				types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
				events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
			}
		}
		log.Infof("[Wait]: Waiting for the chaos interval of %v seconds", experimentsDetails.ChaosInterval)
		common.WaitForDuration(experimentsDetails.ChaosInterval)
		endTime = time.After(timeDelay)
		select {
		case <-endTime:
			log.Infof("[Chaos]: Time is up for experiment: %v", experimentsDetails.ExperimentName)
			break
		}

	}

	//Deleting the helper daemonset
	log.Info("[Cleanup]: Deleting the helper daemonset")
	err = common.DeleteHelperDaemonset(experimentsDetails.ExperimentName+"-"+experimentsDetails.RunID, "name="+experimentsDetails.ExperimentName+"-"+experimentsDetails.RunID, experimentsDetails.ChaosNamespace, chaosDetails.Timeout, chaosDetails.Delay, clients)
	if err != nil {
		return errors.Errorf("Unable to delete the helper daemonset, err: %v", err)
	}

	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", strconv.Itoa(experimentsDetails.RampTime))
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
		return "", errors.Wrapf(err, "Fail to get the application pod status, due to:%v", err)
	}

	return pod.Spec.Containers[0].Name, nil
}

// CreateHelperDaemonset derive the attributes and create the helper daemonset
func CreateHelperDaemonset(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) error {

	privilegedEnable := false
	if experimentsDetails.ContainerRuntime == "crio" {
		privilegedEnable = true
	}

	helperDaemonset := &appsv1.DaemonSet{
		ObjectMeta: v1.ObjectMeta{
			Name:      experimentsDetails.ExperimentName + "-" + experimentsDetails.RunID,
			Namespace: experimentsDetails.ChaosNamespace,
			Labels: map[string]string{
				"name":     experimentsDetails.ExperimentName + "-" + experimentsDetails.RunID,
				"chaosUID": string(experimentsDetails.ChaosUID),
			},
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{
					"name":     experimentsDetails.ExperimentName + "-" + experimentsDetails.RunID,
					"chaosUID": string(experimentsDetails.ChaosUID),
				},
			},
			Template: apiv1.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{
						"name":     experimentsDetails.ExperimentName + "-" + experimentsDetails.RunID,
						"chaosUID": string(experimentsDetails.ChaosUID),
					},
				},
				Spec: apiv1.PodSpec{
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
								"/bin/sh",
							},
							Args: []string{
								"-c",
								"sleep 10000",
							},
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
			},
		},
	}

	_, err := clients.KubeClient.AppsV1().DaemonSets(experimentsDetails.ChaosNamespace).Create(helperDaemonset)
	return err

}
