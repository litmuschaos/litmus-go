package common

import (
	"context"
	"fmt"
	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/palantir/stacktrace"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/math"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var err error

//GetNodeList check for the availability of the application node for the chaos execution
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
		nodes, err = getAllNodes(clients)
		if err != nil {
			return nil, stacktrace.Propagate(err, "could not get all nodes")
		}
	default:
		nodes, err = getNodesByLabels(nodeLabel, clients)
		if err != nil {
			return nil, stacktrace.Propagate(err, "could not get nodes by labels")
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
		podList, err := clients.KubeClient.CoreV1().Pods(namespace).List(context.Background(), v1.ListOptions{LabelSelector: labels})
		if err != nil {
			return "", cerrors.Error{ErrorCode: cerrors.ErrorTypeTargetSelection, Target: fmt.Sprintf("{podLabel: %s, namespace: %s}", labels, namespace), Reason: err.Error()}
		} else if len(podList.Items) == 0 {
			return "", cerrors.Error{ErrorCode: cerrors.ErrorTypeTargetSelection, Target: fmt.Sprintf("{podLabel: %s, namespace: %s}", labels, namespace), Reason: "no pod found with matching labels"}
		}

		rand.Seed(time.Now().Unix())
		randomIndex := rand.Intn(len(podList.Items))
		return podList.Items[randomIndex].Spec.NodeName, nil
	default:
		nodeList, err := getNodesByLabels(nodeLabel, clients)
		if err != nil {
			return "", stacktrace.Propagate(err, "could not get nodes by labels")
		}
		rand.Seed(time.Now().Unix())
		randomIndex := rand.Intn(len(nodeList.Items))
		return nodeList.Items[randomIndex].Name, nil
	}
}

func getAllNodes(clients clients.ClientSets) (*apiv1.NodeList, error) {
	nodeList, err := clients.KubeClient.CoreV1().Nodes().List(context.Background(), v1.ListOptions{})
	if err != nil {
		return nil, cerrors.Error{ErrorCode: cerrors.ErrorTypeTargetSelection, Reason: fmt.Sprintf("failed to list all nodes: %s", err.Error())}
	} else if len(nodeList.Items) == 0 {
		return nil, cerrors.Error{ErrorCode: cerrors.ErrorTypeTargetSelection, Reason: "no node found!"}
	}
	return nodeList, nil
}

func getNodesByLabels(nodeLabel string, clients clients.ClientSets) (*apiv1.NodeList, error) {
	nodeList, err := clients.KubeClient.CoreV1().Nodes().List(context.Background(), v1.ListOptions{LabelSelector: nodeLabel})
	if err != nil {
		return nil, cerrors.Error{ErrorCode: cerrors.ErrorTypeTargetSelection, Target: fmt.Sprintf("{nodeLabel: %s}", nodeLabel), Reason: err.Error()}
	} else if len(nodeList.Items) == 0 {
		return nil, cerrors.Error{ErrorCode: cerrors.ErrorTypeTargetSelection, Target: fmt.Sprintf("{nodeLabel: %s}", nodeLabel), Reason: "no node found with matching labels"}
	}
	return nodeList, nil
}
