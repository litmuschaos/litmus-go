package common

import (
	"math/rand"
	"strconv"
	"time"

	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/math"
	"github.com/pkg/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//GetNodeList check for the availibilty of the application node for the chaos execution
// if the application node is not defined it will derive the random target node list using node affected percentage
func GetNodeList(nodeName string, nodeAffPerc int, clients clients.ClientSets) ([]string, error) {

	var nodeList []string

	if nodeName != "" {
		nodeList = append(nodeList, nodeName)
		return nodeList, nil
	}
	nodes, err := clients.KubeClient.CoreV1().Nodes().List(v1.ListOptions{})
	if err != nil || len(nodes.Items) == 0 {
		return nil, errors.Errorf("Failed to find the nodes, err: %v", err)
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
func GetNodeName(namespace, labels string, clients clients.ClientSets) (string, error) {
	podList, err := clients.KubeClient.CoreV1().Pods(namespace).List(v1.ListOptions{LabelSelector: labels})
	if err != nil || len(podList.Items) == 0 {
		return "", errors.Wrapf(err, "Failed to find the application pods with matching labels in %v namespace, err: %v", namespace, err)
	}

	rand.Seed(time.Now().Unix())
	randomIndex := rand.Intn(len(podList.Items))
	nodeName := podList.Items[randomIndex].Spec.NodeName

	return nodeName, nil
}
