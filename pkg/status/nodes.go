package status

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/litmuschaos/litmus-go/pkg/cerrors"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	logrus "github.com/sirupsen/logrus"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CheckNodeStatus checks the status of the node
func CheckNodeStatus(nodes string, timeout, delay int, clients clients.ClientSets) error {

	return retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {
			nodeList := apiv1.NodeList{}
			if nodes != "" {
				targetNodes := strings.Split(nodes, ",")
				for index := range targetNodes {
					node, err := clients.KubeClient.CoreV1().Nodes().Get(context.Background(), targetNodes[index], metav1.GetOptions{})
					if err != nil {
						return cerrors.Error{ErrorCode: cerrors.ErrorTypeStatusChecks, Target: fmt.Sprintf("{nodeName: %s}", targetNodes[index]), Reason: err.Error()}
					}
					nodeList.Items = append(nodeList.Items, *node)
				}
			} else {
				nodes, err := clients.KubeClient.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
				if err != nil {
					return cerrors.Error{ErrorCode: cerrors.ErrorTypeStatusChecks, Reason: fmt.Sprintf("failed to list all nodes: %s", err.Error())}
				}
				nodeList = *nodes
			}
			for _, node := range nodeList.Items {
				conditions := node.Status.Conditions
				isReady := false
				for _, condition := range conditions {
					if condition.Type == apiv1.NodeReady && condition.Status == apiv1.ConditionTrue {
						isReady = true
						break
					}
				}
				if !isReady {
					return cerrors.Error{ErrorCode: cerrors.ErrorTypeStatusChecks, Target: fmt.Sprintf("{nodeName: %s}", node.Name), Reason: "node is not in ready state"}
				}
				log.InfoWithValues("The Node status are as follows", logrus.Fields{
					"Node": node.Name, "Ready": isReady})
			}
			return nil
		})
}

// CheckNodeNotReadyState check for node to be in not ready state
func CheckNodeNotReadyState(nodeName string, timeout, delay int, clients clients.ClientSets) error {
	return retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {
			node, err := clients.KubeClient.CoreV1().Nodes().Get(context.Background(), nodeName, metav1.GetOptions{})
			if err != nil {
				return cerrors.Error{ErrorCode: cerrors.ErrorTypeStatusChecks, Target: fmt.Sprintf("{nodeName: %s}", nodeName), Reason: err.Error()}
			}
			conditions := node.Status.Conditions
			isReady := false
			for _, condition := range conditions {
				if condition.Type == apiv1.NodeReady && condition.Status == apiv1.ConditionTrue {
					isReady = true
					break
				}
			}
			// It will retry until the node becomes NotReady
			if isReady {
				return cerrors.Error{ErrorCode: cerrors.ErrorTypeStatusChecks, Target: fmt.Sprintf("{nodeName: %s}", nodeName), Reason: "node is not in NotReady state during chaos"}
			}
			log.InfoWithValues("The Node status are as follows", logrus.Fields{
				"Node": node.Name, "Ready": isReady})

			return nil
		})
}
