package status

import (
	"strings"
	"time"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/openebs/maya/pkg/util/retry"
	"github.com/pkg/errors"
	logrus "github.com/sirupsen/logrus"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CheckNodeStatus checks the status of the node
func CheckNodeStatus(nodes string, timeout, delay int, clients clients.ClientSets) error {

	nodeList := apiv1.NodeList{}
	return retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {
			if nodes != "" {
				targetNodes := strings.Split(nodes, ",")
				for index := range targetNodes {
					node, err := clients.KubeClient.CoreV1().Nodes().Get(targetNodes[index], metav1.GetOptions{})
					if err != nil {
						return err
					}
					nodeList.Items = append(nodeList.Items, *node)
				}
			} else {
				nodes, err := clients.KubeClient.CoreV1().Nodes().List(metav1.ListOptions{})
				if err != nil {
					return err
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
					return errors.Errorf("Node is not in ready state")
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
			node, err := clients.KubeClient.CoreV1().Nodes().Get(nodeName, metav1.GetOptions{})
			if err != nil {
				return err
			}
			conditions := node.Status.Conditions
			isReady := false
			for _, condition := range conditions {
				if condition.Type == apiv1.NodeReady && condition.Status == apiv1.ConditionTrue {
					isReady = true
					break
				}
			}
			// It will retries until the node becomes NotReady
			if isReady {
				return errors.Errorf("Node is not in NotReady state")
			}
			log.InfoWithValues("The Node status are as follows", logrus.Fields{
				"Node": node.Name, "Ready": isReady})

			return nil
		})
}
