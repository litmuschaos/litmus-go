package lib

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/palantir/stacktrace"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/node-restart/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/litmuschaos/litmus-go/pkg/utils/stringutils"
	"github.com/sirupsen/logrus"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var err error

const (
	secretName      string = "id-rsa"
	privateKeyMount string = "/mnt"
	privateKeyPath  string = "/mnt/ssh-privatekey"
	emptyDirMount   string = "/data"
	emptyDirPath    string = "/data/ssh-privatekey"

	privateKeySecret string = "private-key-cm-"
	emptyDirVolume   string = "empty-dir-"
	ObjectNameField         = "metadata.name"
)

// PrepareNodeRestart contains preparation steps before chaos injection
func PrepareNodeRestart(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	//Select the node
	if experimentsDetails.TargetNode == "" {
		//Select node for node-restart
		experimentsDetails.TargetNode, err = common.GetNodeName(experimentsDetails.AppNS, experimentsDetails.AppLabel, experimentsDetails.NodeLabel, clients)
		if err != nil {
			return stacktrace.Propagate(err, "could not get node name")
		}
	}

	// get the node ip
	if experimentsDetails.TargetNodeIP == "" {
		experimentsDetails.TargetNodeIP, err = getInternalIP(experimentsDetails.TargetNode, clients)
		if err != nil {
			return stacktrace.Propagate(err, "could not get internal ip")
		}
	}

	log.InfoWithValues("[Info]: Details of application under chaos injection", logrus.Fields{
		"Target Node":    experimentsDetails.TargetNode,
		"Target Node IP": experimentsDetails.TargetNodeIP,
	})

	experimentsDetails.RunID = stringutils.GetRunID()

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", strconv.Itoa(experimentsDetails.RampTime))
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	if experimentsDetails.EngineName != "" {
		msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on " + experimentsDetails.TargetNode + " node"
		types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
		events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")

		if err := common.SetHelperData(chaosDetails, experimentsDetails.SetHelperData, clients); err != nil {
			return err
		}
	}

	// Creating the helper pod to perform node restart
	if err = createHelperPod(experimentsDetails, chaosDetails, clients); err != nil {
		return stacktrace.Propagate(err, "could not create helper pod")
	}

	appLabel := fmt.Sprintf("app=%s-helper-%s", experimentsDetails.ExperimentName, experimentsDetails.RunID)

	//Checking the status of helper pod
	log.Info("[Status]: Checking the status of the helper pod")
	if err = status.CheckHelperStatus(experimentsDetails.ChaosNamespace, appLabel, experimentsDetails.Timeout, experimentsDetails.Delay, clients); err != nil {
		common.DeleteHelperPodBasedOnJobCleanupPolicy(experimentsDetails.ExperimentName+"-helper-"+experimentsDetails.RunID, appLabel, chaosDetails, clients)
		return stacktrace.Propagate(err, "could not check helper status")
	}

	common.SetTargets(experimentsDetails.TargetNode, "targeted", "node", chaosDetails)

	// run the probes during chaos
	if len(resultDetails.ProbeDetails) != 0 {
		if err = probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
			common.DeleteAllHelperPodBasedOnJobCleanupPolicy(appLabel, chaosDetails, clients)
			return err
		}
	}

	// Wait till the completion of helper pod
	log.Info("[Wait]: Waiting till the completion of the helper pod")
	podStatus, err := status.WaitForCompletion(experimentsDetails.ChaosNamespace, appLabel, clients, experimentsDetails.ChaosDuration+experimentsDetails.Timeout, common.GetContainerNames(chaosDetails)...)
	if err != nil || podStatus == "Failed" {
		common.DeleteHelperPodBasedOnJobCleanupPolicy(experimentsDetails.ExperimentName+"-helper-"+experimentsDetails.RunID, appLabel, chaosDetails, clients)
		return common.HelperFailedError(err, appLabel, chaosDetails.ChaosNamespace, false)
	}

	//Deleting the helper pod
	log.Info("[Cleanup]: Deleting the helper pod")
	if err = common.DeletePod(experimentsDetails.ExperimentName+"-helper-"+experimentsDetails.RunID, appLabel, experimentsDetails.ChaosNamespace, chaosDetails.Timeout, chaosDetails.Delay, clients); err != nil {
		return stacktrace.Propagate(err, "could not delete helper pod")
	}

	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", strconv.Itoa(experimentsDetails.RampTime))
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	return nil
}

// createHelperPod derive the attributes for helper pod and create the helper pod
func createHelperPod(experimentsDetails *experimentTypes.ExperimentDetails, chaosDetails *types.ChaosDetails, clients clients.ClientSets) error {
	// This method is attaching emptyDir along with secret volume, and copy data from secret
	// to the emptyDir, because secret is mounted as readonly and with 777 perms and it can't be changed
	// because of: https://github.com/kubernetes/kubernetes/issues/57923

	terminationGracePeriodSeconds := int64(experimentsDetails.TerminationGracePeriodSeconds)

	helperPod := &apiv1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:        experimentsDetails.ExperimentName + "-helper-" + experimentsDetails.RunID,
			Namespace:   experimentsDetails.ChaosNamespace,
			Labels:      common.GetHelperLabels(chaosDetails.Labels, experimentsDetails.RunID, experimentsDetails.ExperimentName),
			Annotations: chaosDetails.Annotations,
		},
		Spec: apiv1.PodSpec{
			RestartPolicy:                 apiv1.RestartPolicyNever,
			ImagePullSecrets:              chaosDetails.ImagePullSecrets,
			TerminationGracePeriodSeconds: &terminationGracePeriodSeconds,
			Affinity: &apiv1.Affinity{
				NodeAffinity: &apiv1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &apiv1.NodeSelector{
						NodeSelectorTerms: []apiv1.NodeSelectorTerm{
							{
								MatchFields: []apiv1.NodeSelectorRequirement{
									{
										Key:      ObjectNameField,
										Operator: apiv1.NodeSelectorOpNotIn,
										Values:   []string{experimentsDetails.TargetNode},
									},
								},
							},
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
						"/bin/sh",
					},
					Args:      []string{"-c", fmt.Sprintf("cp %[1]s %[2]s && chmod 400 %[2]s && ssh -o \"StrictHostKeyChecking=no\" -o \"UserKnownHostsFile=/dev/null\" -i %[2]s %[3]s@%[4]s %[5]s", privateKeyPath, emptyDirPath, experimentsDetails.SSHUser, experimentsDetails.TargetNodeIP, experimentsDetails.RebootCommand)},
					Resources: chaosDetails.Resources,
					VolumeMounts: []apiv1.VolumeMount{
						{
							Name:      privateKeySecret + experimentsDetails.RunID,
							MountPath: privateKeyMount,
						},
						{
							Name:      emptyDirVolume + experimentsDetails.RunID,
							MountPath: emptyDirMount,
						},
					},
				},
			},
			Volumes: []apiv1.Volume{
				{
					Name: privateKeySecret + experimentsDetails.RunID,
					VolumeSource: apiv1.VolumeSource{
						Secret: &apiv1.SecretVolumeSource{
							SecretName: secretName,
						},
					},
				},
				{
					Name: emptyDirVolume + experimentsDetails.RunID,
					VolumeSource: apiv1.VolumeSource{
						EmptyDir: &apiv1.EmptyDirVolumeSource{},
					},
				},
			},
		},
	}

	if len(chaosDetails.SideCar) != 0 {
		helperPod.Spec.Containers = append(helperPod.Spec.Containers, common.BuildSidecar(chaosDetails)...)
		helperPod.Spec.Volumes = append(helperPod.Spec.Volumes, common.GetSidecarVolumes(chaosDetails)...)
	}

	_, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.ChaosNamespace).Create(context.Background(), helperPod, v1.CreateOptions{})
	if err != nil {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Reason: fmt.Sprintf("unable to create helper pod: %s", err.Error())}
	}
	return nil
}

// getInternalIP gets the internal ip of the given node
func getInternalIP(nodeName string, clients clients.ClientSets) (string, error) {
	node, err := clients.KubeClient.CoreV1().Nodes().Get(context.Background(), nodeName, v1.GetOptions{})
	if err != nil {
		return "", cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Target: fmt.Sprintf("{nodeName: %s}", nodeName), Reason: err.Error()}
	}
	for _, addr := range node.Status.Addresses {
		if strings.ToLower(string(addr.Type)) == "internalip" {
			return addr.Address, nil
		}
	}
	return "", cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Target: fmt.Sprintf("{nodeName: %s}", nodeName), Reason: "failed to get the internal ip of the target node"}
}
