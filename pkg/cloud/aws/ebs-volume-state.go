package aws

import (
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/kube-aws/ebs-loss-by-tag/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// WaitForVolumeDetachment will wait the ebs volume to completely detach
func WaitForVolumeDetachment(ebsVolumeID, ec2InstanceID, region string, delay, timeout int) error {

	log.Info("[Status]: Checking ebs volume status for detachment")
	err := retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {

			instanceState, err := GetEBSStatus(ebsVolumeID, ec2InstanceID, region)
			if err != nil {
				return errors.Errorf("failed to get the instance status")
			}
			if instanceState != "detached" {
				log.Infof("The instance state is %v", instanceState)
				return errors.Errorf("instance is not yet in detached state")
			}
			log.Infof("The instance state is %v", instanceState)
			return nil
		})
	return err
}

// WaitForVolumeAttachment will wait for the ebs volume to get attached on ec2 instance
func WaitForVolumeAttachment(ebsVolumeID, ec2InstanceID, region string, delay, timeout int) error {

	log.Info("[Status]: Checking ebs volume status for attachment")
	err := retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {

			instanceState, err := GetEBSStatus(ebsVolumeID, ec2InstanceID, region)
			if err != nil {
				return errors.Errorf("failed to get the instance status")
			}
			if instanceState != "attached" {
				log.Infof("The instance state is %v", instanceState)
				return errors.Errorf("instance is not yet in attached state")
			}
			log.Infof("The instance state is %v", instanceState)
			return nil
		})
	return err
}

//GetEBSStatus will verify and give the ec2 instance details along with ebs volume idetails.
func GetEBSStatus(ebsVolumeID, ec2InstanceID, region string) (string, error) {

	// Load session from shared config
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
		Config:            aws.Config{Region: aws.String(region)},
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

		if *volumeDetails.VolumeId == ebsVolumeID {
			if len(volumeDetails.Attachments) > 0 {
				if *volumeDetails.Attachments[0].InstanceId == ec2InstanceID {

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
			return "detached", nil
		}
	}
	return "", errors.Errorf("unable to find the ebs volume with volumeId %v", ebsVolumeID)
}

//PostChaosVolumeStatusCheck is the ebs volume state check after chaos
func PostChaosVolumeStatusCheck(experimentsDetails *experimentTypes.ExperimentDetails) error {

	var ec2InstanceIDList []string
	for i, volumeID := range experimentsDetails.TargetVolumeIDList {
		//Get volume attachment details
		ec2InstanceID, _, err := GetVolumeAttachmentDetails(volumeID, experimentsDetails.VolumeTag, experimentsDetails.Region)
		if err != nil || ec2InstanceID == "" {
			return errors.Errorf("fail to get the attachment info, err: %v", err)
		}
		ec2InstanceIDList = append(ec2InstanceIDList, ec2InstanceID)

		//Getting the EBS volume attachment status
		ebsState, err := GetEBSStatus(volumeID, ec2InstanceIDList[i], experimentsDetails.Region)
		if err != nil {
			return errors.Errorf("failed to get the ebs status, err: %v", err)
		}
		if ebsState != "attached" {
			return errors.Errorf("'%v' volume is not in attached state post chaos", volumeID)
		}

	}
	return nil
}
