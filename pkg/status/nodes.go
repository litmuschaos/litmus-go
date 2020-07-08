package status

import (
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
func CheckNodeStatus(nodeName string, clients clients.ClientSets) error {
	err := retry.
		Times(90).
		Wait(2 * time.Second).
		Try(func(attempt uint) error {
			node, err := clients.KubeClient.CoreV1().Nodes().Get(nodeName, metav1.GetOptions{})
			if err != nil {
				return errors.Errorf("Unable to get the node, err: %v", err)
			}
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
			log.InfoWithValues("The running status of Nodes are as follows", logrus.Fields{
				"Node": node.Name, "Ready": isReady})

			return nil
		})
	if err != nil {
		return err
	}
	return nil
}

// CheckNodeNotReadyState check for node to be in not ready state
func CheckNodeNotReadyState(nodeName string, clients clients.ClientSets) error {
	err := retry.
		Times(90).
		Wait(2 * time.Second).
		Try(func(attempt uint) error {
			node, err := clients.KubeClient.CoreV1().Nodes().Get(nodeName, metav1.GetOptions{})
			if err != nil {
				return errors.Errorf("Unable to get the node, err: %v", err)
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
			log.InfoWithValues("The running status of Nodes are as follows", logrus.Fields{
				"Node": node.Name, "Ready": isReady})

			return nil
		})
	if err != nil {
		return err
	}
	return nil
}
