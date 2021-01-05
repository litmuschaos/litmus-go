package lib

import (
	"fmt"
	"math/rand"
	"strconv"
	"time"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/node-restart/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/pkg/errors"
	apiv1 "k8s.io/api/core/v1"
	k8stypes "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	schedulerapi "k8s.io/kubernetes/pkg/scheduler/api"
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
)

// PrepareNodeRestart contains preparation steps before chaos injection
func PrepareNodeRestart(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	//Select the node
	if experimentsDetails.TargetNode == "" {
		//Select node for node-restart
		targetNode, err := GetNode(experimentsDetails, clients)
		if err != nil {
			return err
		}

		experimentsDetails.TargetNode = targetNode.Spec.NodeName
		experimentsDetails.TargetNodeIP = targetNode.Status.HostIP
	}

	// Checking the status of target node
	log.Info("[Status]: Getting the status of target node")
	err = status.CheckNodeStatus(experimentsDetails.TargetNode, experimentsDetails.Timeout, experimentsDetails.Delay, clients)
	if err != nil {
		return errors.Errorf("Target node is not in ready state, err: %v", err)
	}

	experimentsDetails.RunID = common.GetRunID()
	appLabel := "name=" + experimentsDetails.ExperimentName + "-" + experimentsDetails.RunID

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", strconv.Itoa(experimentsDetails.RampTime))
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	if experimentsDetails.EngineName != "" {
		msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on " + experimentsDetails.TargetNode + " node"
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
	}
	// Creating the helper pod to perform node restart
	err = CreateHelperPod(experimentsDetails, clients)
	if err != nil {
		return errors.Errorf("Unable to create the helper pod, err: %v", err)
	}

	//Checking the status of helper pod
	log.Info("[Status]: Checking the status of the helper pod")
	err = CheckApplicationStatus(experimentsDetails.ChaosNamespace, appLabel, experimentsDetails.Timeout, experimentsDetails.Delay, clients)
	if err != nil {
		common.DeleteHelperPodBasedOnJobCleanupPolicy(experimentsDetails.ExperimentName+"-"+experimentsDetails.RunID, appLabel, chaosDetails, clients)
		return errors.Errorf("helper pod is not in running state, err: %v", err)
	}

	// Wait till the completion of helper pod
	log.Infof("[Wait]: Waiting for %vs till the completion of the helper pod", strconv.Itoa(experimentsDetails.ChaosDuration+30))

	podStatus, err := status.WaitForCompletion(experimentsDetails.ChaosNamespace, appLabel, clients, experimentsDetails.ChaosDuration+30, experimentsDetails.ExperimentName)
	if err != nil || podStatus == "Failed" {
		common.DeleteHelperPodBasedOnJobCleanupPolicy(experimentsDetails.ExperimentName+"-"+experimentsDetails.RunID, appLabel, chaosDetails, clients)
		return errors.Errorf("helper pod failed due to, err: %v", err)
	}

	// Checking the status of application node
	log.Info("[Status]: Getting the status of application node")
	err = status.CheckNodeStatus(experimentsDetails.TargetNode, experimentsDetails.Timeout, experimentsDetails.Delay, clients)
	if err != nil {
		common.DeleteHelperPodBasedOnJobCleanupPolicy(experimentsDetails.ExperimentName+"-"+experimentsDetails.RunID, appLabel, chaosDetails, clients)
		log.Warnf("Application node is not in the ready state, you may need to manually recover the node, err: %v", err)
	}

	//Deleting the helper pod
	log.Info("[Cleanup]: Deleting the helper pod")
	err = common.DeletePod(experimentsDetails.ExperimentName+"-"+experimentsDetails.RunID, appLabel, experimentsDetails.ChaosNamespace, chaosDetails.Timeout, chaosDetails.Delay, clients)
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

// CreateHelperPod derive the attributes for helper pod and create the helper pod
func CreateHelperPod(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) error {
	// This method is attaching emptyDir along with secret volume, and copy data from secret
	// to the emptyDir, because secret is mounted as readonly and with 777 perms and it can't be changed
	// because of: https://github.com/kubernetes/kubernetes/issues/57923

	helperPod := &apiv1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:      experimentsDetails.ExperimentName + "-" + experimentsDetails.RunID,
			Namespace: experimentsDetails.ChaosNamespace,
			Labels: map[string]string{
				"app":                       experimentsDetails.ExperimentName + "-" + "helper",
				"name":                      experimentsDetails.ExperimentName + "-" + experimentsDetails.RunID,
				"chaosUID":                  string(experimentsDetails.ChaosUID),
				"app.kubernetes.io/part-of": "litmus",
			},
			Annotations: experimentsDetails.Annotations,
		},
		Spec: apiv1.PodSpec{
			RestartPolicy: apiv1.RestartPolicyNever,
			Affinity: &k8stypes.Affinity{
				NodeAffinity: &k8stypes.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &k8stypes.NodeSelector{
						NodeSelectorTerms: []k8stypes.NodeSelectorTerm{
							{
								MatchFields: []k8stypes.NodeSelectorRequirement{
									{
										Key:      schedulerapi.NodeFieldSelectorKeyNodeName,
										Operator: k8stypes.NodeSelectorOpNotIn,
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
					Resources: experimentsDetails.Resources,
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

	_, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.ChaosNamespace).Create(helperPod)
	return err
}

//GetNode will select a random replica of application pod and return the node spec of that application pod
func GetNode(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) (*k8stypes.Pod, error) {
	podList, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).List(v1.ListOptions{LabelSelector: experimentsDetails.AppLabel})
	if err != nil || len(podList.Items) == 0 {
		return nil, errors.Wrapf(err, "Fail to get the application pod in %v namespace, err: %v", experimentsDetails.AppNS, err)
	}

	rand.Seed(time.Now().Unix())
	randomIndex := rand.Intn(len(podList.Items))
	podForNodeCandidate := podList.Items[randomIndex]

	return &podForNodeCandidate, nil
}

// CheckApplicationStatus checks the status of the AUT
func CheckApplicationStatus(appNs, appLabel string, timeout, delay int, clients clients.ClientSets) error {

	// Checking whether application containers are in ready state
	log.Info("[Status]: Checking whether application containers are in ready state")
	err := status.CheckContainerStatus(appNs, appLabel, timeout, delay, clients)
	if err != nil {
		return err
	}
	// Checking whether application pods are in running or completed state
	log.Info("[Status]: Checking whether application pods are in running or completed state")
	err = status.CheckPodStatusPhase(appNs, appLabel, timeout, delay, clients, "Running", "Completed")
	if err != nil {
		return err
	}

	return nil
}
