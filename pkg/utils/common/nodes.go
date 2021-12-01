package common

import (
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/math"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/pkg/errors"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var err error

//GetNodeList check for the availibilty of the application node for the chaos execution
// if the application node is not defined it will derive the random target node list using node affected percentage
func GetNodeList(nodeNames, nodeLabel string, nodeAffPerc int, clients clients.ClientSets) ([]string, error) {

	var nodeList []string
	var nodes *apiv1.NodeList

	if nodeNames != "" {
		targetNodesList := strings.Split(nodeNames, ",")
		return targetNodesList, nil
	}

	switch nodeLabel {
	case "":
		nodes, err = clients.KubeClient.CoreV1().Nodes().List(v1.ListOptions{})
		if err != nil {
			return nil, errors.Errorf("Failed to find the nodes, err: %v", err)
		} else if len(nodes.Items) == 0 {
			return nil, errors.Errorf("Failed to find the nodes")
		}
	default:
		nodes, err = clients.KubeClient.CoreV1().Nodes().List(v1.ListOptions{LabelSelector: nodeLabel})
		if err != nil {
			return nil, errors.Errorf("Failed to find the nodes with matching label, err: %v", err)
		} else if len(nodes.Items) == 0 {
			return nil, errors.Errorf("Failed to find the nodes with matching label")
		}
	}

	newNodeListLength := math.Maximum(1, math.Adjustment(nodeAffPerc, len(nodes.Items)))

	// it will generate the random nodelist
	// it starts from the random index and choose requirement no of pods next to that index in a circular way.
	rand.Seed(time.Now().UnixNano())
	index := rand.Intn(len(nodes.Items))
	for i := 0; i < newNodeListLength; i++ {
		nodeList = append(nodeList, nodes.Items[index].Name)
		index = (index + 1) % len(nodes.Items)
	}

	log.Infof("[Chaos]:Number of nodes targeted: %v", strconv.Itoa(newNodeListLength))

	return nodeList, nil
}

//GetNodeName will select a random replica of application pod and return the node name of that application pod
func GetNodeName(namespace, labels, nodeLabel string, clients clients.ClientSets) (string, error) {

	switch nodeLabel {
	case "":
		podList, err := clients.KubeClient.CoreV1().Pods(namespace).List(v1.ListOptions{LabelSelector: labels})
		if err != nil {
			return "", errors.Wrapf(err, "Failed to find the application pods with matching labels in %v namespace, err: %v", namespace, err)
		} else if len(podList.Items) == 0 {
			return "", errors.Errorf("Failed to find the application pods with matching labels in %v namespace", namespace)
		}

		rand.Seed(time.Now().Unix())
		randomIndex := rand.Intn(len(podList.Items))
		return podList.Items[randomIndex].Spec.NodeName, nil
	default:
		nodeList, err := clients.KubeClient.CoreV1().Nodes().List(v1.ListOptions{LabelSelector: nodeLabel})
		if err != nil {
			return "", errors.Wrapf(err, "Failed to find the target nodes with matching labels in %v namespace, err: %v", namespace, err)
		} else if len(nodeList.Items) == 0 {
			return "", errors.Wrapf(err, "Failed to find the target nodes with matching labels in %v namespace", namespace)
		}
		rand.Seed(time.Now().Unix())
		randomIndex := rand.Intn(len(nodeList.Items))
		return nodeList.Items[randomIndex].Name, nil
	}
}

// PreChaosNodeStatusCheck fetches all the nodes in the cluster and checks their status, and fetches the total active nodes in the cluster, prior to the chaos experiment
func PreChaosNodeStatusCheck(timeout, delay int, clients clients.ClientSets) (int, error) {
	nodeList, err := clients.KubeClient.CoreV1().Nodes().List(v1.ListOptions{})
	if err != nil {
		return 0, errors.Errorf("fail to get the nodes, err: %v", err)
	}
	for _, node := range nodeList.Items {
		if err = status.CheckNodeStatus(node.Name, timeout, delay, clients); err != nil {
			log.Infof("[Info]: The cluster is unhealthy this might not work, due to %v", err)
		}
	}
	activeNodeCount, err := getActiveNodeCount(clients)
	if err != nil {
		return 0, errors.Errorf("fail to get the total active node count pre chaos, err: %v", err)
	}

	return activeNodeCount, nil
}

// PostChaosActiveNodeCountCheck checks the number of active nodes post chaos and validates the number of healthy node count post chaos
func PostChaosActiveNodeCountCheck(activeNodeCount, timeout, delay int, clients clients.ClientSets) error {
	err := retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {

			activeNodes, err := getActiveNodeCount(clients)
			if err != nil {
				return errors.Errorf("fail to get the total active nodes, err: %v", err)
			}
			if activeNodeCount != activeNodes {
				return errors.Errorf("fail to get equal active node post chaos")
			}
			return nil
		})
	return err
}

// getActiveNodeCount fetches the target node and total node count from the cluster
func getActiveNodeCount(clients clients.ClientSets) (int, error) {
	nodeList, err := clients.KubeClient.CoreV1().Nodes().List(v1.ListOptions{})
	if err != nil {
		return 0, errors.Errorf("fail to get the nodes, err: %v", err)
	}

	nodeCount := 0
	for _, node := range nodeList.Items {

		conditions := node.Status.Conditions
		for _, condition := range conditions {
			if condition.Type == apiv1.NodeReady && condition.Status == apiv1.ConditionTrue {
				nodeCount++
			}
		}
	}
	log.Infof("[Info]: Total number active nodes are: %v", nodeCount)

	return nodeCount, nil
}
