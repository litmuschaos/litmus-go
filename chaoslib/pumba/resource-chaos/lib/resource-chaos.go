package lib

import (
	"strconv"
	"time"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/pod-cpu-hog/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var err error

// PreparePodCPUHog contains prepration steps before chaos injection
func PreparePodCPUHog(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	var appNodeName string
	if experimentsDetails.TargetPod == "" {
		experimentsDetails.TargetPod, appNodeName, err = common.GetPodAndNodeName(experimentsDetails.AppNS, experimentsDetails.TargetPod, experimentsDetails.AppLabel, clients)
	} else {
		_, appNodeName, err = common.GetPodAndNodeName(experimentsDetails.AppNS, experimentsDetails.TargetPod, experimentsDetails.AppLabel, clients)
	}

	log.InfoWithValues("[Info]: Details of application under chaos injection", logrus.Fields{
		"NodeName": appNodeName,
		"AppName":  experimentsDetails.TargetPod,
	})

	experimentsDetails.RunID = common.GetRunID()

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", strconv.Itoa(experimentsDetails.RampTime))
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	if experimentsDetails.EngineName != "" {
		msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on " + appNodeName + " node"
		types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
		events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
	}

	// Get Chaos Pod Annotation
	experimentsDetails.Annotations, err = common.GetChaosPodAnnotation(experimentsDetails.ChaosPodName, experimentsDetails.ChaosNamespace, clients)
	if err != nil {
		return errors.Errorf("unable to get annotation, due to %v", err)
	}

	// Creating the helper pod to perform node cpu hog
	err = CreateHelperPod(experimentsDetails, clients, appNodeName)
	if err != nil {
		return errors.Errorf("Unable to create the helper pod, err: %v", err)
	}

	//Checking the status of helper pod
	log.Info("[Status]: Checking the status of the helper pod")
	err = status.CheckApplicationStatus(experimentsDetails.ChaosNamespace, "name="+experimentsDetails.ExperimentName+"-"+experimentsDetails.RunID, experimentsDetails.Timeout, experimentsDetails.Delay, clients)
	if err != nil {
		return errors.Errorf("helper pod is not in running state, err: %v", err)
	}

	// Wait till the completion of helper pod
	log.Infof("[Wait]: Waiting for %vs till the completion of the helper pod", strconv.Itoa(experimentsDetails.ChaosDuration+30))

	podStatus, err := WaitForChaosComplition(experimentsDetails.ChaosNamespace, "name="+experimentsDetails.ExperimentName+"-"+experimentsDetails.RunID, clients, experimentsDetails.ChaosDuration, "pumba-stress")
	if err != nil || podStatus == "Failed" {
		return errors.Errorf("helper pod failed due to, err: %v", err)
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

// WaitForChaosComplition will check the pods status to be completed for chaos duration
// if the application pod or container is not completed still it will not throw error
func WaitForChaosComplition(appNs string, appLabel string, clients clients.ClientSets, duration int, containerName string) (string, error) {

	var podStatus string

	ChaosStartTimeStamp := time.Now().Unix()
	err := retry.
		Times(uint(duration)).
		Wait(1 * time.Second).
		Try(func(attempt uint) error {
			podSpec, err := clients.KubeClient.CoreV1().Pods(appNs).List(v1.ListOptions{LabelSelector: appLabel})
			if err != nil || len(podSpec.Items) == 0 {
				return errors.Errorf("Unable to get the pod, err: %v", err)
			}

			for _, pod := range podSpec.Items {
				podStatus = string(pod.Status.Phase)
				log.Infof("helper pod status: %v", podStatus)
				ChaosCurrentTimeStamp := time.Now().Unix()
				chaosDiffTimeStamp := ChaosCurrentTimeStamp - ChaosStartTimeStamp

				if podStatus != "Succeeded" && podStatus != "Failed" {
					if chaosDiffTimeStamp < int64(duration) {
						return errors.Errorf("Chaos Duration is not completed")
					}
				}

				log.InfoWithValues("The running status of Pods are as follows", logrus.Fields{
					"Pod": pod.Name, "Status": podStatus})
			}
			return nil
		})
	if err != nil {
		return "", err
	}
	return podStatus, nil

}

// CreateHelperPod derive the attributes for helper pod and create the helper pod
func CreateHelperPod(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, appNodeName string) error {

	helperPod := &apiv1.Pod{
		TypeMeta: v1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      experimentsDetails.ExperimentName + "-" + experimentsDetails.RunID,
			Namespace: experimentsDetails.ChaosNamespace,
			Labels: map[string]string{
				"app":      experimentsDetails.ExperimentName,
				"name":     experimentsDetails.ExperimentName + "-" + experimentsDetails.RunID,
				"chaosUID": string(experimentsDetails.ChaosUID),
				// prevent pumba from killing itself
				"com.gaiaadm.pumba": "true",
			},
			Annotations: experimentsDetails.Annotations,
		},
		Spec: apiv1.PodSpec{
			RestartPolicy: apiv1.RestartPolicyNever,
			NodeName:      appNodeName,
			Volumes: []apiv1.Volume{
				apiv1.Volume{
					Name: "dockersocket",
					VolumeSource: apiv1.VolumeSource{
						HostPath: &apiv1.HostPathVolumeSource{
							Path: "/var/run/docker.sock",
						},
					},
				},
			},
			Containers: []apiv1.Container{
				apiv1.Container{
					Name:  "pumba-stress",
					Image: experimentsDetails.LIBImage,
					Args:  GetContainerArguments(experimentsDetails),
					VolumeMounts: []apiv1.VolumeMount{
						apiv1.VolumeMount{
							Name:      "dockersocket",
							MountPath: "/var/run/docker.sock",
						},
					},
					ImagePullPolicy: apiv1.PullPolicy("Always"),
					SecurityContext: &apiv1.SecurityContext{
						Capabilities: &apiv1.Capabilities{
							Add: []apiv1.Capability{
								apiv1.Capability("SYS_ADMIN"),
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
func GetContainerArguments(experimentsDetails *experimentTypes.ExperimentDetails) []string {
	stressArgs := []string{

		"--log-level",
		"debug",
		"--label",
		"io.kubernetes.pod.name=" + experimentsDetails.TargetPod + "",
		"stress",
		"--duration",
		strconv.Itoa(experimentsDetails.ChaosDuration) + "s",
		"--stressors",
		"--cpu " + strconv.Itoa(experimentsDetails.CPUcores) + " --timeout " + strconv.Itoa(experimentsDetails.ChaosDuration) + "",
	}
	return stressArgs
}
