package aws

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/kube-aws/ebs-loss/types"
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
	input := &ec2.DescribeVolumesInput{}

	// Call to get detailed information on each instance
	result, err := ec2Svc.DescribeVolumes(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			default:
				return "", errors.Errorf(aerr.Error())
			}
		} else {
			return "", errors.Errorf(err.Error())
		}
	}

	for _, volumeDetails := range result.Volumes {

		if *volumeDetails.VolumeId == experimentsDetails.EBSVolumeID {
			if len(volumeDetails.Attachments) > 0 {
				if *volumeDetails.Attachments[0].InstanceId == experimentsDetails.Ec2InstanceID {

					// DISPLAY THE EBS VOLUME INFORMATION
					log.InfoWithValues("The selected EBS volume is:", logrus.Fields{
						"VolumeId":   *volumeDetails.VolumeId,
						"VolumeType": *volumeDetails.VolumeType,
						"Status":     *volumeDetails.State,
						"InstanceId": *volumeDetails.Attachments[0].InstanceId,
						"Device":     *volumeDetails.Attachments[0].Device,
						"State":      *volumeDetails.Attachments[0].State,
					})
					return *volumeDetails.Attachments[0].State, nil
				}
			}
			return "", nil
		}
	}
	return "", errors.Errorf("unable to find the ebs volume with volumeId %v", experimentsDetails.EBSVolumeID)
}
