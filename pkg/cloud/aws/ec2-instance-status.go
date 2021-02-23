package aws

import (
	"math/rand"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/kube-aws/ec2-terminate/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/pkg/errors"
)

//GetEC2InstanceStatus will verify and give the ec2 instance details along with ebs volume idetails.
func GetEC2InstanceStatus(experimentsDetails *experimentTypes.ExperimentDetails) (string, error) {

	var err error
	// Load session from shared config
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
		Config:            aws.Config{Region: aws.String(experimentsDetails.Region)},
	}))

	if experimentsDetails.Ec2InstanceID == "" {
		log.Infof("[PreChaos]: Instance id is not provided, selecting a random instance from %v region", experimentsDetails.Region)
		experimentsDetails.Ec2InstanceID, err = GetRandomInstance(experimentsDetails.Region, sess)
		if err != nil {
			return "", errors.Errorf("fail to select a random running instance from %v region, err: %v", experimentsDetails.Region, err)
		}
	}

	// Create new EC2 client
	ec2Svc := ec2.New(sess)

	// Call to get detailed information on each instance
	result, err := ec2Svc.DescribeInstances(nil)
	if err != nil {
		return "", err
	}

	for _, reservationDetails := range result.Reservations {

		for _, instanceDetails := range reservationDetails.Instances {

			if *instanceDetails.InstanceId == experimentsDetails.Ec2InstanceID {
				return *instanceDetails.State.Name, nil
			}
		}
	}
	return "", errors.Errorf("failed to get the status of ec2 instance with instanceID %v", experimentsDetails.Ec2InstanceID)

}

// GetRandomInstance will give a random running instance from a specific region
func GetRandomInstance(region string, session *session.Session) (string, error) {

	// Create new EC2 client
	ec2svc := ec2.New(session)
	instanceList := make([]string, 0)

	//filter and target only running instances
	params := &ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("instance-state-name"),
				Values: []*string{aws.String("running")},
			},
		},
	}
	resp, err := ec2svc.DescribeInstances(params)
	if err != nil {
		return "", errors.Errorf("fail to list the insances, err: %v", err.Error())
	}

	for _, reservationDetails := range resp.Reservations {
		for _, instanceDetails := range reservationDetails.Instances {
			instanceList = append(instanceList, *instanceDetails.InstanceId)
		}
	}
	if len(instanceList) == 0 {
		return "", errors.Errorf("No running instance found in the given region")
	}
	randomIndex := rand.Intn(len(instanceList))
	return instanceList[randomIndex], nil
}
