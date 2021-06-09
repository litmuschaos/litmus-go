package aws

import (
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/cloud/aws/common"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/status"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/pkg/errors"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//GetEC2InstanceStatus will verify and give the ec2 instance details along with ebs volume idetails.
func GetEC2InstanceStatus(instanceID, region string) (string, error) {

	var err error
	// Load session from shared config
	sess := common.GetAWSSession(region)

	// Create new EC2 client
	ec2Svc := ec2.New(sess)

	// Call to get detailed information on each instance
	result, err := ec2Svc.DescribeInstances(nil)
	if err != nil {
		return "", err
	}

	for _, reservationDetails := range result.Reservations {

		for _, instanceDetails := range reservationDetails.Instances {

			if *instanceDetails.InstanceId == instanceID {
				return *instanceDetails.State.Name, nil
			}
		}
	}
	return "", errors.Errorf("failed to get the status of ec2 instance with instanceID %v", region)

}

// PreChaosNodeStatusCheck fetch the target node name from instance id and checks its status also fetch the total active nodes in the cluster
func PreChaosNodeStatusCheck(timeout, delay int, clients clients.ClientSets) (int, error) {

	nodeList, err := clients.KubeClient.CoreV1().Nodes().List(metav1.ListOptions{})
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

// PostChaosActiveNodeCountCheck checks the number of active nodes post chaos and validate the number of healthy node count post chaos.
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

// getActiveNodeCount fetch the target node and total node count from the cluster
func getActiveNodeCount(clients clients.ClientSets) (int, error) {

	nodeList, err := clients.KubeClient.CoreV1().Nodes().List(metav1.ListOptions{})
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

//InstanceStatusCheckByID is used to check the instance status of all the instance under chaos.
func InstanceStatusCheckByID(instanceID, region string) error {

	instanceIDList := strings.Split(instanceID, ",")
	if len(instanceIDList) == 0 {
		return errors.Errorf("no instance id found to terminate")
	}
	log.Infof("[Info]: The instances under chaos(IUC) are: %v", instanceIDList)
	return InstanceStatusCheck(instanceIDList, region)
}

//InstanceStatusCheckByTag is used to check the instance status of all the instance under chaos.
func InstanceStatusCheckByTag(instanceTag, region string) error {

	instanceIDList, err := GetInstanceList(instanceTag, region)
	if err != nil {
		return err
	}
	log.Infof("[Info]: The instances under chaos(IUC) are: %v", instanceIDList)
	return InstanceStatusCheck(instanceIDList, region)
}

//InstanceStatusCheck is used to check the instance status of the instances
func InstanceStatusCheck(targetInstanceIDList []string, region string) error {

	for _, id := range targetInstanceIDList {
		instanceState, err := GetEC2InstanceStatus(id, region)
		if err != nil {
			return err
		}
		if instanceState != "running" {
			return errors.Errorf("failed to get the ec2 instance '%v' in running sate, current state: %v", id, instanceState)
		}
	}
	return nil
}
