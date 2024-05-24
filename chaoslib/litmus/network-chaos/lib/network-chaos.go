package lib

import (
	"context"
	"fmt"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"net"
	"strconv"
	"strings"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
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

var serviceMesh = []string{"istio", "envoy"}

//PrepareAndInjectChaos contains the prepration & injection steps
func PrepareAndInjectChaos(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails, args string) error {

	targetPodList := apiv1.PodList{}
	var err error
	var podsAffectedPerc int
	// Get the target pod details for the chaos execution
	// if the target pod is not defined it will derive the random target pod list using pod affected percentage
	if experimentsDetails.TargetPods == "" && chaosDetails.AppDetail.Label == "" {
		return errors.Errorf("please provide one of the appLabel or TARGET_PODS")
	}
	//setup the tunables if provided in range
	SetChaosTunables(experimentsDetails)

	switch experimentsDetails.NetworkChaosType {
	case "network-loss":
		log.InfoWithValues("[Info]: The chaos tunables are:", logrus.Fields{
			"NetworkPacketLossPercentage": experimentsDetails.NetworkPacketLossPercentage,
			"Sequence":                    experimentsDetails.Sequence,
			"PodsAffectedPerc":            experimentsDetails.PodsAffectedPerc,
		})
	case "network-latency":
		log.InfoWithValues("[Info]: The chaos tunables are:", logrus.Fields{
			"NetworkLatency":   strconv.Itoa(experimentsDetails.NetworkLatency),
			"Sequence":         experimentsDetails.Sequence,
			"PodsAffectedPerc": experimentsDetails.PodsAffectedPerc,
		})
	case "network-corruption":
		log.InfoWithValues("[Info]: The chaos tunables are:", logrus.Fields{
			"NetworkPacketCorruptionPercentage": experimentsDetails.NetworkPacketCorruptionPercentage,
			"Sequence":                          experimentsDetails.Sequence,
			"PodsAffectedPerc":                  experimentsDetails.PodsAffectedPerc,
		})
	case "network-duplication":
		log.InfoWithValues("[Info]: The chaos tunables are:", logrus.Fields{
			"NetworkPacketDuplicationPercentage": experimentsDetails.NetworkPacketDuplicationPercentage,
			"Sequence":                           experimentsDetails.Sequence,
			"PodsAffectedPerc":                   experimentsDetails.PodsAffectedPerc,
		})
	}
	podsAffectedPerc, _ = strconv.Atoi(experimentsDetails.PodsAffectedPerc)
	if experimentsDetails.NodeLabel == "" {

		//targetPodList, err := common.GetPodListFromSpecifiedNodes(experimentsDetails.TargetPods, experimentsDetails.PodsAffectedPerc, clients, chaosDetails)
		targetPodList, err = common.GetPodList(experimentsDetails.TargetPods, podsAffectedPerc, clients, chaosDetails)
		if err != nil {
			return err
		}
	} else {
		//targetPodList, err := common.GetPodList(experimentsDetails.TargetPods, experimentsDetails.PodsAffectedPerc, clients, chaosDetails)
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
		if err = injectChaosInSerialMode(experimentsDetails, targetPodList, clients, chaosDetails, args, resultDetails, eventsDetails); err != nil {
			return err
		}
	case "parallel":
		if err = injectChaosInParallelMode(experimentsDetails, targetPodList, clients, chaosDetails, args, resultDetails, eventsDetails); err != nil {
			return err
		}
	default:
		return errors.Errorf("%v sequence is not supported", experimentsDetails.Sequence)
	}

	return nil
}

// injectChaosInSerialMode inject the network chaos in all target application serially (one by one)
func injectChaosInSerialMode(experimentsDetails *experimentTypes.ExperimentDetails, targetPodList apiv1.PodList, clients clients.ClientSets, chaosDetails *types.ChaosDetails, args string, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails) error {

	labelSuffix := common.GetRunID()
	// run the probes during chaos
	if len(resultDetails.ProbeDetails) != 0 {
		if err := probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
			return err
		}
	}

	// creating the helper pod to perform network chaos
	for _, pod := range targetPodList.Items {

		destIPs, err := GetTargetIps(experimentsDetails.DestinationIPs, experimentsDetails.DestinationHosts, clients, isServiceMeshEnabledForPod(pod))
		if err != nil {
			return err
		}

		//Get the target container name of the application pod
		if !experimentsDetails.IsTargetContainerProvided {
			experimentsDetails.TargetContainer, err = common.GetTargetContainer(experimentsDetails.AppNS, pod.Name, clients)
			if err != nil {
				return errors.Errorf("unable to get the target container name, err: %v", err)
			}
		}

		log.InfoWithValues("[Info]: Details of application under chaos injection", logrus.Fields{
			"PodName":       pod.Name,
			"NodeName":      pod.Spec.NodeName,
			"ContainerName": experimentsDetails.TargetContainer,
		})
		runID := common.GetRunID()
		if err := createHelperPod(experimentsDetails, clients, chaosDetails, pod.Name, pod.Spec.NodeName, runID, args, labelSuffix, destIPs); err != nil {
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

		//Deleting all the helper pod for container-kill chaos
		log.Info("[Cleanup]: Deleting the the helper pod")
		if err := common.DeletePod(experimentsDetails.ExperimentName+"-helper-"+runID, appLabel, experimentsDetails.ChaosNamespace, chaosDetails.Timeout, chaosDetails.Delay, clients); err != nil {
			return errors.Errorf("unable to delete the helper pods, err: %v", err)
		}
	}

	return nil
}

// injectChaosInParallelMode inject the network chaos in all target application in parallel mode (all at once)
func injectChaosInParallelMode(experimentsDetails *experimentTypes.ExperimentDetails, targetPodList apiv1.PodList, clients clients.ClientSets, chaosDetails *types.ChaosDetails, args string, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails) error {

	labelSuffix := common.GetRunID()
	var err error
	// run the probes during chaos
	if len(resultDetails.ProbeDetails) != 0 {
		if err := probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
			return err
		}
	}

	// creating the helper pod to perform network chaos
	for _, pod := range targetPodList.Items {

		destIPs, err := GetTargetIps(experimentsDetails.DestinationIPs, experimentsDetails.DestinationHosts, clients, isServiceMeshEnabledForPod(pod))
		if err != nil {
			return err
		}

		//Get the target container name of the application pod
		//It checks the empty target container for the first iteration only
		if !experimentsDetails.IsTargetContainerProvided {
			experimentsDetails.TargetContainer, err = common.GetTargetContainer(experimentsDetails.AppNS, pod.Name, clients)
			if err != nil {
				return errors.Errorf("unable to get the target container name, err: %v", err)
			}
		}

		log.InfoWithValues("[Info]: Details of application under chaos injection", logrus.Fields{
			"PodName":       pod.Name,
			"NodeName":      pod.Spec.NodeName,
			"ContainerName": experimentsDetails.TargetContainer,
		})
		runID := common.GetRunID()
		if err := createHelperPod(experimentsDetails, clients, chaosDetails, pod.Name, pod.Spec.NodeName, runID, args, labelSuffix, destIPs); err != nil {
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

	//Deleting all the helper pod for container-kill chaos
	log.Info("[Cleanup]: Deleting all the helper pod")
	if err := common.DeleteAllPod(appLabel, experimentsDetails.ChaosNamespace, chaosDetails.Timeout, chaosDetails.Delay, clients); err != nil {
		return errors.Errorf("unable to delete the helper pods, err: %v", err)
	}

	return nil
}

// createHelperPod derive the attributes for helper pod and create the helper pod
func createHelperPod(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, chaosDetails *types.ChaosDetails, podName, nodeName, runID, args, labelSuffix, destIPs string) error {

	privilegedEnable := true
	terminationGracePeriodSeconds := int64(experimentsDetails.TerminationGracePeriodSeconds)

	helperPod := &apiv1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:        experimentsDetails.ExperimentName + "-helper-" + runID,
			Namespace:   experimentsDetails.ChaosNamespace,
			Labels:      common.GetHelperLabels(chaosDetails.Labels, runID, labelSuffix, experimentsDetails.ExperimentName),
			Annotations: chaosDetails.Annotations,
		},
		Spec: apiv1.PodSpec{
			HostPID:                       true,
			TerminationGracePeriodSeconds: &terminationGracePeriodSeconds,
			ImagePullSecrets:              chaosDetails.ImagePullSecrets,
			ServiceAccountName:            experimentsDetails.ChaosServiceAccount,
			RestartPolicy:                 apiv1.RestartPolicyNever,
			NodeName:                      nodeName,
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
					Name: "netns",
					VolumeSource: apiv1.VolumeSource{
						HostPath: &apiv1.HostPathVolumeSource{
							Path: "/var/run/netns",
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
						"./helpers -name network-chaos",
					},
					Resources: chaosDetails.Resources,
					Env:       getPodEnv(experimentsDetails, podName, args, destIPs),
					VolumeMounts: []apiv1.VolumeMount{
						{
							Name:      "cri-socket",
							MountPath: experimentsDetails.SocketPath,
						},
						{
							Name:      "netns",
							MountPath: "/var/run/netns",
						},
					},
					SecurityContext: &apiv1.SecurityContext{
						Privileged: &privilegedEnable,
						Capabilities: &apiv1.Capabilities{
							Add: []apiv1.Capability{
								"NET_ADMIN",
								"SYS_ADMIN",
							},
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
func getPodEnv(experimentsDetails *experimentTypes.ExperimentDetails, podName, args, destIPs string) []apiv1.EnvVar {

	var envDetails common.ENVDetails
	envDetails.SetEnv("APP_NAMESPACE", experimentsDetails.AppNS).
		SetEnv("APP_POD", podName).
		SetEnv("APP_CONTAINER", experimentsDetails.TargetContainer).
		SetEnv("TOTAL_CHAOS_DURATION", strconv.Itoa(experimentsDetails.ChaosDuration)).
		SetEnv("CHAOS_NAMESPACE", experimentsDetails.ChaosNamespace).
		SetEnv("CHAOSENGINE", experimentsDetails.EngineName).
		SetEnv("CHAOS_UID", string(experimentsDetails.ChaosUID)).
		SetEnv("CONTAINER_RUNTIME", experimentsDetails.ContainerRuntime).
		SetEnv("NETEM_COMMAND", args).
		SetEnv("NETWORK_INTERFACE", experimentsDetails.NetworkInterface).
		SetEnv("EXPERIMENT_NAME", experimentsDetails.ExperimentName).
		SetEnv("SOCKET_PATH", experimentsDetails.SocketPath).
		SetEnv("DESTINATION_IPS", destIPs).
		SetEnv("INSTANCE_ID", experimentsDetails.InstanceID).
		SetEnv("SOURCE_PORTS", experimentsDetails.SourcePorts).
		SetEnv("DESTINATION_PORTS", experimentsDetails.DestinationPorts).
		SetEnvFromDownwardAPI("v1", "metadata.name")

	return envDetails.ENV
}

// GetTargetIps return the comma separated target ips
// It fetch the ips from the target ips (if defined by users)
// it append the ips from the host, if target host is provided
func GetTargetIps(targetIPs, targetHosts string, clients clients.ClientSets, serviceMesh bool) (string, error) {

	ipsFromHost, err := getIpsForTargetHosts(targetHosts, clients, serviceMesh)
	if err != nil {
		return "", err
	}
	if targetIPs == "" {
		targetIPs = ipsFromHost
	} else if ipsFromHost != "" {
		targetIPs = targetIPs + "," + ipsFromHost
	}
	return targetIPs, nil
}

// it derive the pod ips from the kubernetes service
func getPodIPFromService(host string, clients clients.ClientSets) ([]string, error) {
	var ips []string
	svcFields := strings.Split(host, ".")
	if len(svcFields) != 5 {
		return ips, fmt.Errorf("provide the valid FQDN for service in '<svc-name>.<namespace>.svc.cluster.local format, host: %v", host)
	}
	svcName, svcNs := svcFields[0], svcFields[1]
	svc, err := clients.KubeClient.CoreV1().Services(svcNs).Get(context.Background(), svcName, v1.GetOptions{})
	if err != nil {
		if k8serrors.IsForbidden(err) {
			log.Warnf("forbidden - failed to get %v service in %v namespace, err: %v", svcName, svcNs, err)
			return ips, nil
		}
		return ips, err
	}
	for k, v := range svc.Spec.Selector {
		pods, err := clients.KubeClient.CoreV1().Pods(svcNs).List(context.Background(), v1.ListOptions{LabelSelector: fmt.Sprintf("%s=%s", k, v)})
		if err != nil {
			return ips, err
		}
		for _, p := range pods.Items {
			ips = append(ips, p.Status.PodIP)
		}
	}
	return ips, nil
}

// getIpsForTargetHosts resolves IP addresses for comma-separated list of target hosts and returns comma-separated ips
func getIpsForTargetHosts(targetHosts string, clients clients.ClientSets, serviceMesh bool) (string, error) {
	if targetHosts == "" {
		return "", nil
	}
	hosts := strings.Split(targetHosts, ",")
	finalHosts := ""
	var commaSeparatedIPs []string
	for i := range hosts {
		hosts[i] = strings.TrimSpace(hosts[i])
		if strings.Contains(hosts[i], "svc.cluster.local") && serviceMesh {
			ips, err := getPodIPFromService(hosts[i], clients)
			if err != nil {
				return "", err
			}
			log.Infof("Host: {%v}, IP address: {%v}", hosts[i], ips)
			commaSeparatedIPs = append(commaSeparatedIPs, ips...)
			if finalHosts == "" {
				finalHosts = hosts[i]
			} else {
				finalHosts = finalHosts + "," + hosts[i]
			}
			finalHosts = finalHosts + "," + hosts[i]
			continue
		}
		ips, err := net.LookupIP(hosts[i])
		if err != nil {
			log.Warnf("Unknown host: {%v}, it won't be included in the scope of chaos", hosts[i])
		} else {
			for j := range ips {
				log.Infof("Host: {%v}, IP address: {%v}", hosts[i], ips[j])
				commaSeparatedIPs = append(commaSeparatedIPs, ips[j].String())
			}
			if finalHosts == "" {
				finalHosts = hosts[i]
			} else {
				finalHosts = finalHosts + "," + hosts[i]
			}
		}
	}
	if len(commaSeparatedIPs) == 0 {
		return "", errors.Errorf("provided hosts: {%v} are invalid, unable to resolve", targetHosts)
	}
	log.Infof("Injecting chaos on {%v} hosts", finalHosts)
	return strings.Join(commaSeparatedIPs, ","), nil
}

//SetChaosTunables will setup a random value within a given range of values
//If the value is not provided in range it'll setup the initial provided value.
func SetChaosTunables(experimentsDetails *experimentTypes.ExperimentDetails) {
	experimentsDetails.NetworkPacketLossPercentage = common.ValidateRange(experimentsDetails.NetworkPacketLossPercentage)
	experimentsDetails.NetworkPacketCorruptionPercentage = common.ValidateRange(experimentsDetails.NetworkPacketCorruptionPercentage)
	experimentsDetails.NetworkPacketDuplicationPercentage = common.ValidateRange(experimentsDetails.NetworkPacketDuplicationPercentage)
	experimentsDetails.PodsAffectedPerc = common.ValidateRange(experimentsDetails.PodsAffectedPerc)
	experimentsDetails.Sequence = common.GetRandomSequence(experimentsDetails.Sequence)
}

// It checks if pod contains service mesh sidecar
func isServiceMeshEnabledForPod(pod apiv1.Pod) bool {
	for _, c := range pod.Spec.Containers {
		if common.StringExistsInSlice(c.Name, serviceMesh) {
			return true
		}
	}
	return false
}
