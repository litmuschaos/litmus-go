package aws

import (
	"strings"

	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/litmuschaos/litmus-go/pkg/cloud/aws/common"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/pkg/errors"
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
	return "", errors.Errorf("failed to get the status of ec2 instance with instanceID %v", instanceID)

}

//InstanceStatusCheckByID is used to check the instance status of all the instance under chaos.
func InstanceStatusCheckByID(instanceID, region string) error {
	if instanceID == "" {
		return errors.Errorf("no instance id found to terminate")
	}
	instanceIDList := strings.Split(strings.TrimSpace(instanceID), ",")
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

// PreChaosNodeCountCheck returns the active node count before injection of chaos
func PreChaosNodeCountCheck(instanceIDList []string, region string) (int, string, error) {

	var autoScalingGroupName string
	var nodeList []*autoscaling.InstanceDetails
	var err error
	// fetching all instances in the autoscaling groups
	if nodeList, err = getAutoScalingInstances(region); err != nil {
		return 0, "", err
	}

	// finding the autoscaling group name for the provided instance id
	if autoScalingGroupName = findAutoScalingGroupName(instanceIDList[0], nodeList); autoScalingGroupName == "" {
		return 0, "", errors.Errorf("instances not part of autoscaling group")
	}

	// finding the active node count for the autoscaling group
	nodeCount := findActiveNodeCount(autoScalingGroupName, region, nodeList)
	log.Infof("[Info]: Pre-Chaos Active Node Count: %v", nodeCount)
	if len(instanceIDList) > nodeCount {
		return 0, "", errors.Errorf("active node count less than number of provided instance IDs")
	}

	return nodeCount, autoScalingGroupName, nil
}

// PostChaosNodeCountCheck checks if the active node count after injection of chaos is equal to pre-chaos node count
func PostChaosNodeCountCheck(activeNodeCount int, autoScalingGroupName, region string) error {

	var nodeList []*autoscaling.InstanceDetails
	var err error

	// fetching all instances in the autoscaling groups
	if nodeList, err = getAutoScalingInstances(region); err != nil {
		return err
	}
	if autoScalingGroupName == "" {
		return errors.Errorf("autoscaling group not provided")
	}

	// finding the active node count for the autoscaling group
	nodeCount := findActiveNodeCount(autoScalingGroupName, region, nodeList)
	log.Infof("[Info]: Post-Chaos Active Node Count: %v", nodeCount)

	// checking if the post-chaos and pre-chaos node count are equal
	if nodeCount != activeNodeCount {
		return errors.Errorf("post-chaos active node count is not equal to the pre-chaos node count")
	}
	return nil
}

// getAutoScalingInstances fetches the list of instances in the autoscaling groups
func getAutoScalingInstances(region string) ([]*autoscaling.InstanceDetails, error) {

	sess := common.GetAWSSession(region)
	autoScalingSvc := autoscaling.New(sess)

	autoScalingInput := autoscaling.DescribeAutoScalingInstancesInput{}
	nodeList, err := autoScalingSvc.DescribeAutoScalingInstances(&autoScalingInput)
	if err != nil {
		return nil, errors.Errorf("failed to get the autoscaling instances, err: %v", err)
	}
	return nodeList.AutoScalingInstances, nil
}

// findInstancesInAutoScalingGroup returns the list of instances in the provided autoscaling group
func findInstancesInAutoScalingGroup(autoScalingGroupName string, nodeList []*autoscaling.InstanceDetails) []string {

	var instanceIDList []string
	for _, node := range nodeList {
		if *node.AutoScalingGroupName == autoScalingGroupName {
			instanceIDList = append(instanceIDList, *node.InstanceId)
		}
	}
	return instanceIDList
}

// findAutoScalingGroupName returns the autoscaling group name for the provided instance id
func findAutoScalingGroupName(instanceID string, nodeList []*autoscaling.InstanceDetails) string {
	for _, node := range nodeList {
		if *node.InstanceId == instanceID {
			return *node.AutoScalingGroupName
		}
	}
	return ""
}

// findActiveNodeCount returns the active node count for the provided autoscaling group
func findActiveNodeCount(autoScalingGroupName, region string, nodeList []*autoscaling.InstanceDetails) int {

	nodeCount := 0

	for _, id := range findInstancesInAutoScalingGroup(autoScalingGroupName, nodeList) {
		instanceState, err := GetEC2InstanceStatus(id, region)
		if err != nil {
			log.Errorf("instance status check failed for %v, err: %v", id, err)
		}
		if instanceState == "running" {
			nodeCount += 1
		}
	}
	return nodeCount
}
