package aws

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/ebs-loss/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

//GetEBSStatus will verify and give the ec2 instance details along with ebs volume idetails.
func GetEBSStatus(experimentsDetails *experimentTypes.ExperimentDetails) (string, error) {

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

	// Iteration over instances
	for _, instanceDetails := range result.Reservations[0].Instances {

		if *instanceDetails.InstanceId == experimentsDetails.Ec2InstanceID {
			// DISPLAY THE INSTANCE INFORMATION
			log.InfoWithValues("The selected instance information is as follows:", logrus.Fields{
				"Instance Id":   *instanceDetails.InstanceId,
				"Instance Type": *instanceDetails.InstanceType,
				"KeyName":       *instanceDetails.KeyName,
			})

			// Iteration to get the volume
			for _, volumeDetails := range instanceDetails.BlockDeviceMappings {
				if *volumeDetails.Ebs.VolumeId == experimentsDetails.EBSVolumeID && *volumeDetails.DeviceName == experimentsDetails.DeviceName {
					// DISPLAY THE EBS VOLUME INFORMATION
					log.InfoWithValues("The selected EBS volume information is as follows:", logrus.Fields{
						"DeviceName": *volumeDetails.DeviceName,
						"VolumeId":   *volumeDetails.Ebs.VolumeId,
						"Status":     *volumeDetails.Ebs.Status,
					})

					return *volumeDetails.Ebs.Status, nil
				}
			}
		} else {
			return "", errors.Errorf("unable to find EC2 instance Id: %v", *instanceDetails.InstanceId)
		}
	}

	return "", nil
}
