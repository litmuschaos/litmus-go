package aws

import (
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/litmuschaos/litmus-go/pkg/cloud/aws/common"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// EC2Stop will stop an aws ec2 instance
func EC2Stop(instanceID, region string) error {

	// Load session from shared config
	sess := common.GetAWSSession(region)

	// Create new EC2 client
	ec2Svc := ec2.New(sess)

	input := &ec2.StopInstancesInput{
		InstanceIds: []*string{
			aws.String(instanceID),
		},
	}
	result, err := ec2Svc.StopInstances(input)
	if err != nil {
		return common.CheckAWSError(err)
	}

	log.InfoWithValues("Stopping ec2 instance:", logrus.Fields{
		"CurrentState":  *result.StoppingInstances[0].CurrentState.Name,
		"PreviousState": *result.StoppingInstances[0].PreviousState.Name,
		"InstanceId":    *result.StoppingInstances[0].InstanceId,
	})

	return nil
}

// EC2Start will stop an aws ec2 instance
func EC2Start(instanceID, region string) error {

	sess := common.GetAWSSession(region)

	// Create new EC2 client
	ec2Svc := ec2.New(sess)

	input := &ec2.StartInstancesInput{
		InstanceIds: []*string{
			aws.String(instanceID),
		},
	}

	result, err := ec2Svc.StartInstances(input)
	if err != nil {
		return common.CheckAWSError(err)
	}

	log.InfoWithValues("Starting ec2 instance:", logrus.Fields{
		"CurrentState":  *result.StartingInstances[0].CurrentState.Name,
		"PreviousState": *result.StartingInstances[0].PreviousState.Name,
		"InstanceId":    *result.StartingInstances[0].InstanceId,
	})

	return nil
}

//WaitForEC2Down will wait for the ec2 instance to get in stopped state
func WaitForEC2Down(timeout, delay int, managedNodegroup, region, instanceID string) error {

	log.Info("[Status]: Checking EC2 instance status")
	return retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {

			instanceState, err := GetEC2InstanceStatus(instanceID, region)
			if err != nil {
				return errors.Errorf("failed to get the instance status")
			}
			if (managedNodegroup != "enable" && instanceState != "stopped") || (managedNodegroup == "enable" && instanceState != "terminated") {
				log.Infof("The instance state is %v", instanceState)
				return errors.Errorf("instance is not yet in stopped state")
			}
			log.Infof("The instance state is %v", instanceState)
			return nil
		})
}

//WaitForEC2Up will wait for the ec2 instance to get in running state
func WaitForEC2Up(timeout, delay int, managedNodegroup, region, instanceID string) error {

	log.Info("[Status]: Checking EC2 instance status")
	return retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {

			instanceState, err := GetEC2InstanceStatus(instanceID, region)
			if err != nil {
				return errors.Errorf("failed to get the instance status")
			}
			if instanceState != "running" {
				log.Infof("The instance state is %v", instanceState)
				return errors.Errorf("instance is not yet in running state")
			}
			log.Infof("The instance state is %v", instanceState)
			return nil
		})

}

//GetInstanceList will filter out the target instance under chaos using tag filters or the instance list provided.
func GetInstanceList(instanceTag, region string) ([]string, error) {

	var instanceList []string
	switch instanceTag {
	case "":
		return nil, errors.Errorf("fail to get the instance tag please provide a valid instance tag")

	default:
		instanceTag := strings.Split(instanceTag, ":")
		sess := common.GetAWSSession(region)

		params := &ec2.DescribeInstancesInput{
			Filters: []*ec2.Filter{
				{
					Name: aws.String("tag:" + instanceTag[0]),
					Values: []*string{
						aws.String(instanceTag[1]),
					},
				},
			},
		}
		ec2Svc := ec2.New(sess)
		res, err := ec2Svc.DescribeInstances(params)
		if err != nil {
			return nil, errors.Errorf("fail to list the insances, err: %v", err.Error())
		}

		for _, reservationDetails := range res.Reservations {
			for _, i := range reservationDetails.Instances {
				for _, t := range i.Tags {
					if *t.Key == instanceTag[0] {
						instanceList = append(instanceList, *i.InstanceId)
						break
					}
				}
			}
		}
	}
	return instanceList, nil
}
