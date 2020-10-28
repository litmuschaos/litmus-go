package aws

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/kube-aws/ec2-terminate/types"
	"github.com/pkg/errors"
)

//GetEC2InstanceStatus will verify and give the ec2 instance details along with ebs volume idetails.
func GetEC2InstanceStatus(experimentsDetails *experimentTypes.ExperimentDetails) (string, error) {

	// Load session from shared config
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
		Config:            aws.Config{Region: aws.String(experimentsDetails.Region)},
	}))

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
	return "", errors.Errorf("fail to get the status of ec2 instance with instanceID %v", experimentsDetails.Ec2InstanceID)
}
