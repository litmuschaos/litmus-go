package node_memory_hog

import (
	"math/rand"
	"strconv"
	"time"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/node-memory-hog/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/openebs/maya/pkg/util/retry"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PrepareNodeMemoryHog contains prepration steps before chaos injection
func PrepareNodeMemoryHog(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	//Select the node name
	appNodeName, err := GetNodeName(experimentsDetails, clients)
	if err != nil {
		return errors.Errorf("Unable to get the node name due to, err: %v", err)
	}

	log.InfoWithValues("[Info]: Details of application under chaos injection", logrus.Fields{
		"NodeName":             appNodeName,
		"MemoryHog Percentage": experimentsDetails.MemoryPercentage,
	})

	experimentsDetails.RunID = GetRunID()

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", strconv.Itoa(experimentsDetails.RampTime))
		waitForRampTime(experimentsDetails)
	}

	if experimentsDetails.EngineName != "" {
		msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on " + appNodeName + " node"
		types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, chaosDetails)
		events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
	}

	// Creating the helper pod to perform node memory hog
	err = CreateHelperPod(experimentsDetails, clients, appNodeName)
	if err != nil {
		return errors.Errorf("Unable to create the helper pod, err: %v", err)
	}

	//Checking the status of helper pod
	log.Info("[Status]: Checking the status of the helper pod")
	err = status.CheckApplicationStatus(experimentsDetails.ChaosNamespace, "name=node-memory-hog-"+experimentsDetails.RunID, clients)
	if err != nil {
		return errors.Errorf("helper pod is not in running state, err: %v", err)
	}

	// Wait till the completion of helper pod
	log.Infof("[Wait]: Waiting for %vs till the completion of the helper pod", strconv.Itoa(experimentsDetails.ChaosDuration+30))

	err = status.WaitForCompletion(experimentsDetails.ChaosNamespace, "name=node-memory-hog-"+experimentsDetails.RunID, clients, experimentsDetails.ChaosDuration+30)
	if err != nil {
		return err
	}

	// Checking the status of application node
	log.Info("[Status]: Getting the status of application node")
	err = status.CheckNodeStatus(appNodeName, clients)
	if err != nil {
		log.Warn("Application node is not in the ready state, you may need to manually recover the node")
	}

	//Deleting the helper pod
	log.Info("[Cleanup]: Deleting the helper pod")
	err = DeleteHelperPod(experimentsDetails, clients)
	if err != nil {
		return errors.Errorf("Unable to delete the helper pod, err: %v", err)
	}

	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", strconv.Itoa(experimentsDetails.RampTime))
		waitForRampTime(experimentsDetails)
	}
	return nil
}

//GetNodeName will select a random replica of application pod and return the node name of that application pod
func GetNodeName(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) (string, error) {
	podList, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.AppNS).List(v1.ListOptions{LabelSelector: experimentsDetails.AppLabel})
	if err != nil || len(podList.Items) == 0 {
		return "", errors.Wrapf(err, "Fail to get the application pod in %v namespace, due to err: %v", experimentsDetails.AppNS, err)
	}

	rand.Seed(time.Now().Unix())
	randomIndex := rand.Intn(len(podList.Items))
	nodeName := podList.Items[randomIndex].Spec.NodeName

	return nodeName, nil
}

//waitForRampTime waits for the given ramp time duration (in seconds)
func waitForRampTime(experimentsDetails *experimentTypes.ExperimentDetails) {
	time.Sleep(time.Duration(experimentsDetails.RampTime) * time.Second)
}

// GetRunID generate a random string
func GetRunID() string {
	var letterRunes = []rune("abcdefghijklmnopqrstuvwxyz")
	runID := make([]rune, 6)
	for i := range runID {
		runID[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(runID)
}

// CreateHelperPod derive the attributes for helper pod and create the helper pod
func CreateHelperPod(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, appNodeName string) error {

	helperPod := &apiv1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:      "node-memory-hog-" + experimentsDetails.RunID,
			Namespace: experimentsDetails.ChaosNamespace,
			Labels: map[string]string{
				"app":      "node-memory-hog",
				"name":     "node-memory-hog-" + experimentsDetails.RunID,
				"chaosUID": string(experimentsDetails.ChaosUID),
			},
		},
		Spec: apiv1.PodSpec{
			RestartPolicy: apiv1.RestartPolicyNever,
			NodeName:      appNodeName,
			Containers: []apiv1.Container{
				{
					Name:            "node-memory-hog",
					Image:           experimentsDetails.LIBImage,
					ImagePullPolicy: apiv1.PullAlways,
					Args: []string{
						"--vm",
						"1",
						"--vm-bytes",
						strconv.Itoa(experimentsDetails.MemoryPercentage) + "%",
						"--timeout",
						strconv.Itoa(experimentsDetails.ChaosDuration),
					},
				},
			},
		},
	}

	_, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.ChaosNamespace).Create(helperPod)
	return err
}

//DeleteHelperPod delete the helper pod
func DeleteHelperPod(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets) error {

	err := clients.KubeClient.CoreV1().Pods(experimentsDetails.ChaosNamespace).Delete("node-memory-hog-"+experimentsDetails.RunID, &v1.DeleteOptions{})

	if err != nil {
		return err
	}

	err = retry.
		Times(90).
		Wait(1 * time.Second).
		Try(func(attempt uint) error {
			podSpec, err := clients.KubeClient.CoreV1().Pods(experimentsDetails.ChaosNamespace).List(v1.ListOptions{LabelSelector: "name=node-memory-hog-" + experimentsDetails.RunID})
			if err != nil || len(podSpec.Items) != 0 {
				return errors.Errorf("Helper Pod is not deleted yet, err: %v", err)
			}
			return nil
		})

	return err
}
