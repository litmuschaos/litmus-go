package aws

import (
	"strings"

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
