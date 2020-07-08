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
			nodeReady := false
			for _, condition := range conditions {

				if condition.Type == apiv1.NodeReady && condition.Status == apiv1.ConditionTrue {
					nodeReady = true
					break
				}
			}
			if !nodeReady {
				return errors.Errorf("Node is not in ready state")
			}
			log.InfoWithValues("The running status of Nodes are as follows", logrus.Fields{
				"Node": node.Name, "Ready": nodeReady})

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
			nodeReady := false
			for _, condition := range conditions {

				if condition.Type == apiv1.NodeReady && condition.Status == apiv1.ConditionTrue {
					nodeReady = true
					break
				}
			}
			if nodeReady {
				return errors.Errorf("Node is in ready state")
			}
			log.InfoWithValues("The running status of Nodes are as follows", logrus.Fields{
				"Node": node.Name, "Ready": nodeReady})

			return nil
		})
	if err != nil {
		return err
	}
	return nil
}
