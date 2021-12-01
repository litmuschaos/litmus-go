package lib

import (
	"fmt"
	"strconv"
	"strings"

	network_chaos "github.com/litmuschaos/litmus-go/chaoslib/litmus/pod-network-chaos/lib"
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

// PrepareNodeNetworkChaos contains prepration steps before chaos injection
func PrepareNodeNetworkChaos(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails, expName string) error {

	var err error
	var uniqueIps []string

	if experimentsDetails.TargetNode == "" {
		//Select node for node-network-chaos
		experimentsDetails.TargetNode, err = common.GetNodeName(experimentsDetails.AppNS, experimentsDetails.AppLabel, experimentsDetails.NodeLabel, clients)
		if err != nil {
			return err
		}
	}

	log.InfoWithValues("[Info]: Details of node under chaos injection", logrus.Fields{
		"NodeName": experimentsDetails.TargetNode,
	})

	experimentsDetails.RunID = common.GetRunID()

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	if experimentsDetails.EngineName != "" {
		msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on " + experimentsDetails.TargetNode + " node"
		types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
		events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
	}

	if experimentsDetails.EngineName != "" {
		if err := common.SetHelperData(chaosDetails, clients); err != nil {
			return err
		}
	}

	// get all the target ips
	destinationIPs, err := network_chaos.GetTargetIps(experimentsDetails.DestinationIPs, experimentsDetails.DestinationHosts)
	if err != nil {
		return err
	}

	if destinationIPs != "" {
		ips := strings.Split(destinationIPs, ",")

		// removing duplicates ips from the list, if any
		for i := range ips {
			isPresent := false
			for j := range uniqueIps {
				if ips[i] == uniqueIps[j] {
					isPresent = true
				}
			}
			if !isPresent {
				uniqueIps = append(uniqueIps, ips[i])
			}

		}
	}

	command := prepareChaosCommand(experimentsDetails.NetworkPacketLossPercentage, experimentsDetails.NetworkLatency, experimentsDetails.ChaosDuration, expName, uniqueIps)

	// Creating the helper pod to perform node-network-chaos
	if err = createHelperPod(experimentsDetails, chaosDetails, clients, experimentsDetails.TargetNode, command); err != nil {
		return errors.Errorf("unable to create the helper pod, err: %v", err)
	}

	appLabel := "name=" + experimentsDetails.ExperimentName + "-helper-" + experimentsDetails.RunID

	//Checking the status of helper pod
	log.Info("[Status]: Checking the status of the helper pod")
	if err = status.CheckHelperStatus(experimentsDetails.ChaosNamespace, appLabel, experimentsDetails.Timeout, experimentsDetails.Delay, clients); err != nil {
		common.DeleteHelperPodBasedOnJobCleanupPolicy(experimentsDetails.ExperimentName+"-helper-"+experimentsDetails.RunID, appLabel, chaosDetails, clients)
		return errors.Errorf("helper pod is not in running state, err: %v", err)
	}

	// run the probes during chaos
	if len(resultDetails.ProbeDetails) != 0 {
		if err = probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
			common.DeleteAllHelperPodBasedOnJobCleanupPolicy(appLabel, chaosDetails, clients)
			return err
		}
	}

	// Wait till the completion of helper pod
	log.Info("[Wait]: Waiting till the completion of the helper pod")

	podStatus, err := status.WaitForCompletion(experimentsDetails.ChaosNamespace, appLabel, clients, experimentsDetails.ChaosDuration+experimentsDetails.Timeout, experimentsDetails.ExperimentName)
	if err != nil || podStatus == "Failed" {
		common.DeleteHelperPodBasedOnJobCleanupPolicy(experimentsDetails.ExperimentName+"-helper-"+experimentsDetails.RunID, appLabel, chaosDetails, clients)
		return errors.Errorf("helper pod failed, err: %v", err)
	}

	// Checking the status of target nodes
	log.Info("[Status]: Getting the status of target nodes")
	if err = status.CheckNodeStatus(experimentsDetails.TargetNode, experimentsDetails.Timeout, experimentsDetails.Delay, clients); err != nil {
		common.DeleteHelperPodBasedOnJobCleanupPolicy(experimentsDetails.ExperimentName+"-helper-"+experimentsDetails.RunID, appLabel, chaosDetails, clients)
		log.Warnf("Target nodes are not in the ready state, you may need to manually recover the node, err: %v", err)
	}

	//Deleting the helper pod
	log.Info("[Cleanup]: Deleting the helper pod")
	if err = common.DeletePod(experimentsDetails.ExperimentName+"-helper-"+experimentsDetails.RunID, appLabel, experimentsDetails.ChaosNamespace, chaosDetails.Timeout, chaosDetails.Delay, clients); err != nil {
		return errors.Errorf("unable to delete the helper pod, err: %v", err)
	}

	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	return nil
}

func prepareChaosCommand(packetLossPercentage, networkLatency, duration int, expName string, IPs []string) string {

	var cmd string
	fault := "loss " + strconv.Itoa(packetLossPercentage)
	if expName == "node-network-latency" {
		fault = "delay " + strconv.Itoa(networkLatency)
	}

	if len(IPs) != 0 {
		cmd = "sudo tc qdisc replace dev eth0 root handle 1: prio && sudo tc qdisc replace dev eth0 parent 1:3 netem " + fault
		var IpCommand []string
		for _, ip := range IPs {
			tc := fmt.Sprintf("sudo tc filter add dev eth0 protocol ip parent 1:0 prio 3 u32 match ip dst %v flowid 1:3", ip)
			if strings.Contains(ip, ":") {
				tc = fmt.Sprintf("sudo tc filter add dev eth0 protocol ip parent 1:0 prio 3 u32 match ip6 dst %v flowid 1:3", ip)
			}
			IpCommand = append(IpCommand, tc)
		}
		cmd = cmd + " && " + strings.Join(IpCommand, " && ")
	} else {
		cmd = "sudo tc qdisc replace dev eth0 root netem loss " + strconv.Itoa(packetLossPercentage)
	}

	// add kill command
	cmd = cmd + " && sleep " + strconv.Itoa(duration) + " && sudo tc qdisc delete dev eth0 root"

	return cmd
}

// createHelperPod derive the attributes for helper pod and create the helper pod
func createHelperPod(experimentsDetails *experimentTypes.ExperimentDetails, chaosDetails *types.ChaosDetails, clients clients.ClientSets, appNodeName, command string) error {

	privilegedEnable := true
	helperPod := &apiv1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:      experimentsDetails.ExperimentName + "-helper-" + experimentsDetails.RunID,
			Namespace: experimentsDetails.ChaosNamespace,
			Labels: map[string]string{
				"app":                       experimentsDetails.ExperimentName,
				"name":                      experimentsDetails.ExperimentName + "-helper-" + experimentsDetails.RunID,
				"chaosUID":                  string(experimentsDetails.ChaosUID),
				"app.kubernetes.io/part-of": "litmus",
			},
			Annotations: chaosDetails.Annotations,
		},
		Spec: apiv1.PodSpec{
			HostNetwork:      true,
			RestartPolicy:    apiv1.RestartPolicyNever,
			ImagePullSecrets: chaosDetails.ImagePullSecrets,
			NodeName:         appNodeName,
			Volumes: []apiv1.Volume{
				{
					Name: "cri-socket",
					VolumeSource: apiv1.VolumeSource{
						HostPath: &apiv1.HostPathVolumeSource{
							Path: experimentsDetails.SocketPath,
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
						command,
					},
					Resources: chaosDetails.Resources,
					VolumeMounts: []apiv1.VolumeMount{
						{
							Name:      "cri-socket",
							MountPath: experimentsDetails.SocketPath,
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

	_, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.ChaosNamespace).Create(helperPod)
	return err
}
