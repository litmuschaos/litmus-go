package lib

import (
	"net"
	"strings"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/network-chaos/types"
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

//PrepareAndInjectChaos contains the prepration and chaos injection steps
func PrepareAndInjectChaos(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails, args []string) error {

	// Get the target pod details for the chaos execution
	// if the target pod is not defined it will derive the random target pod list using pod affected percentage
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

	if experimentsDetails.EngineName != "" {
		msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on target pod"
		types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
		events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
	}

	if experimentsDetails.Sequence == "serial" {
		if err = InjectChaosInSerialMode(experimentsDetails, targetPodList, clients, chaosDetails, args, resultDetails, eventsDetails); err != nil {
			return err
		}
	} else {
		if err = InjectChaosInParallelMode(experimentsDetails, targetPodList, clients, chaosDetails, args, resultDetails, eventsDetails); err != nil {
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
func InjectChaosInSerialMode(experimentsDetails *experimentTypes.ExperimentDetails, targetPodList apiv1.PodList, clients clients.ClientSets, chaosDetails *types.ChaosDetails, args []string, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails) error {

	// creating the helper pod to perform network chaos
	for _, pod := range targetPodList.Items {

		runID := common.GetRunID()

		log.InfoWithValues("[Info]: Details of application under chaos injection", logrus.Fields{
			"PodName":  pod.Name,
			"NodeName": pod.Spec.NodeName,
		})
		// args contains details of the specific chaos injection
		// constructing `argsWithRegex` based on updated regex with a diff pod name
		// without extending/concatenating the args var itself
		argsWithRegex := append(args, "re2:k8s_POD_"+pod.Name+"_"+experimentsDetails.AppNS)
		log.Infof("Arguments for running %v are %v", experimentsDetails.ExperimentName, argsWithRegex)
		err = CreateHelperPod(experimentsDetails, clients, pod.Spec.NodeName, runID, argsWithRegex)
		if err != nil {
			return errors.Errorf("Unable to create the helper pod, err: %v", err)
		}

		//checking the status of the helper pod, wait till the pod comes to running state else fail the experiment
		log.Info("[Status]: Checking the status of the helper pod")
		err = status.CheckApplicationStatus(experimentsDetails.ChaosNamespace, "app="+experimentsDetails.ExperimentName+"-helper", experimentsDetails.Timeout, experimentsDetails.Delay, clients)
		if err != nil {
			common.DeleteHelperPodBasedOnJobCleanupPolicy(experimentsDetails.ExperimentName+"-"+runID, "app="+experimentsDetails.ExperimentName+"-helper", chaosDetails, clients)
			return errors.Errorf("helper pod is not in running state, err: %v", err)
		}

		// run the probes during chaos
		if len(resultDetails.ProbeDetails) != 0 {
			if err = probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
				common.DeleteHelperPodBasedOnJobCleanupPolicy(experimentsDetails.ExperimentName+"-"+runID, "app="+experimentsDetails.ExperimentName+"-helper", chaosDetails, clients)
				return err
			}
		}

		// Wait till the completion of helper pod
		log.Infof("[Wait]: Waiting for %vs till the completion of the helper pod", experimentsDetails.ChaosDuration)
		podStatus, err := status.WaitForCompletion(experimentsDetails.ChaosNamespace, "app="+experimentsDetails.ExperimentName+"-helper", clients, experimentsDetails.ChaosDuration+30, chaosDetails.ExperimentName)
		if err != nil || podStatus == "Failed" {
			common.DeleteHelperPodBasedOnJobCleanupPolicy(experimentsDetails.ExperimentName+"-"+runID, "app="+experimentsDetails.ExperimentName+"-helper", chaosDetails, clients)
			return errors.Errorf("helper pod failed, err: %v", err)
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
func InjectChaosInParallelMode(experimentsDetails *experimentTypes.ExperimentDetails, targetPodList apiv1.PodList, clients clients.ClientSets, chaosDetails *types.ChaosDetails, args []string, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails) error {

	// creating the helper pod to perform network chaos
	for _, pod := range targetPodList.Items {

		runID := common.GetRunID()

		log.InfoWithValues("[Info]: Details of application under chaos injection", logrus.Fields{
			"PodName":  pod.Name,
			"NodeName": pod.Spec.NodeName,
		})
		// args contains details of the specific chaos injection
		// constructing `argsWithRegex` based on updated regex with a diff pod name
		// without extending/concatenating the args var itself
		argsWithRegex := append(args, "re2:k8s_POD_"+pod.Name+"_"+experimentsDetails.AppNS)
		log.Infof("Arguments for running %v are %v", experimentsDetails.ExperimentName, argsWithRegex)
		err = CreateHelperPod(experimentsDetails, clients, pod.Spec.NodeName, runID, argsWithRegex)
		if err != nil {
			return errors.Errorf("Unable to create the helper pod, err: %v", err)
		}
	}

	//checking the status of the helper pod, wait till the pod comes to running state else fail the experiment
	log.Info("[Status]: Checking the status of the helper pod")
	err = status.CheckApplicationStatus(experimentsDetails.ChaosNamespace, "app="+experimentsDetails.ExperimentName+"-helper", experimentsDetails.Timeout, experimentsDetails.Delay, clients)
	if err != nil {
		common.DeleteAllHelperPodBasedOnJobCleanupPolicy("app="+experimentsDetails.ExperimentName+"-helper", chaosDetails, clients)
		return errors.Errorf("helper pod is not in running state, err: %v", err)
	}

	// run the probes during chaos
	if len(resultDetails.ProbeDetails) != 0 {
		if err = probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
			common.DeleteAllHelperPodBasedOnJobCleanupPolicy("app="+experimentsDetails.ExperimentName+"-helper", chaosDetails, clients)
			return err
		}
	}

	// Wait till the completion of helper pod
	log.Infof("[Wait]: Waiting for %vs till the completion of the helper pod", experimentsDetails.ChaosDuration)
	podStatus, err := status.WaitForCompletion(experimentsDetails.ChaosNamespace, "app="+experimentsDetails.ExperimentName+"-helper", clients, experimentsDetails.ChaosDuration+30, chaosDetails.ExperimentName)
	if err != nil || podStatus == "Failed" {
		common.DeleteAllHelperPodBasedOnJobCleanupPolicy("app="+experimentsDetails.ExperimentName+"-helper", chaosDetails, clients)
		return errors.Errorf("helper pod failed, err: %v", err)
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
func CreateHelperPod(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, appNodeName, runID string, args []string) error {

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
					Name:            experimentsDetails.ExperimentName,
					Image:           experimentsDetails.LIBImage,
					ImagePullPolicy: apiv1.PullPolicy(experimentsDetails.LIBImagePullPolicy),
					Command: []string{
						"pumba",
					},
					Args:      args,
					Resources: experimentsDetails.Resources,
					VolumeMounts: []apiv1.VolumeMount{
						{
							Name:      "dockersocket",
							MountPath: "/var/run/docker.sock",
						},
					},
				},
			},
		},
	}

	_, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.ChaosNamespace).Create(helperPod)
	return err
}

// AddTargetIpsArgs inserts a comma-separated list of targetIPs (if provided by the user) into the pumba command/args
func AddTargetIpsArgs(targetIPs string, args []string) []string {
	if targetIPs == "" {
		return args
	}
	ips := strings.Split(targetIPs, ",")
	for i := range ips {
		args = append(args, "--target", strings.TrimSpace(ips[i]))
	}
	return args
}

// GetIpsForTargetHosts resolves IP addresses for comma-separated list of target hosts and returns comma-separated ips
func GetIpsForTargetHosts(targetHosts string) string {
	if targetHosts == "" {
		return ""
	}
	hosts := strings.Split(targetHosts, ",")
	var commaSeparatedIPs []string
	for i := range hosts {
		ips, err := net.LookupIP(hosts[i])
		if err != nil {
			log.Infof("Unknown host")
		} else {
			for j := range ips {
				log.Infof("IP address: %v", ips[j])
				commaSeparatedIPs = append(commaSeparatedIPs, ips[j].String())
			}
		}
	}
	return strings.Join(commaSeparatedIPs, ",")
}
