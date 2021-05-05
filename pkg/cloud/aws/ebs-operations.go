package aws

import (
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/kube-aws/ebs-loss-by-tag/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// EBSVolumeDetach will detach the ebs volume from ec2 instance
func EBSVolumeDetach(ebsVolumeID, region string) error {

	// Load session from shared config
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
		Config:            aws.Config{Region: aws.String(region)},
	}))

	// Create new EC2 client
	ec2Svc := ec2.New(sess)
	input := &ec2.DetachVolumeInput{
		VolumeId: aws.String(ebsVolumeID),
	}

	result, err := ec2Svc.DetachVolume(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			default:
				return errors.Errorf(aerr.Error())
			}
		} else {
			return errors.Errorf(err.Error())
		}
	}

	log.InfoWithValues("Detaching ebs having:", logrus.Fields{
		"VolumeId":   *result.VolumeId,
		"State":      *result.State,
		"Device":     *result.Device,
		"InstanceID": *result.InstanceId,
	})

	return nil
}

// EBSVolumeAttach will attach the ebs volume to the instance
func EBSVolumeAttach(ebsVolumeID, ec2InstanceID, deviceName, region string) error {

	// Load session from shared config
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
		Config:            aws.Config{Region: aws.String(region)},
	}))

	// Create new EC2 client
	ec2Svc := ec2.New(sess)

	//Attaching the ebs volume after chaos
	input := &ec2.AttachVolumeInput{
		Device:     aws.String(deviceName),
		InstanceId: aws.String(ec2InstanceID),
		VolumeId:   aws.String(ebsVolumeID),
	}

	result, err := ec2Svc.AttachVolume(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			default:
				return errors.Errorf(aerr.Error())
			}
		} else {
			return errors.Errorf(err.Error())
		}
	}

	log.InfoWithValues("Attaching ebs having:", logrus.Fields{
		"VolumeId":   *result.VolumeId,
		"State":      *result.State,
		"Device":     *result.Device,
		"InstanceId": *result.InstanceId,
	})
	return nil
}

//SetTargetVolumeIDs will filter out the volume under chaos
func SetTargetVolumeIDs(experimentsDetails *experimentTypes.ExperimentDetails) error {

	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
		Config:            aws.Config{Region: aws.String(experimentsDetails.Region)},
	}))

	params := setVolumeParameter(experimentsDetails.VolumeTag)
	ec2Svc := ec2.New(sess)
	res, err := ec2Svc.DescribeVolumes(params)
	if err != nil {
		return errors.Errorf("fail to describe the volumes of given tag, err: %v", err.Error())
	}
	for _, volumeDetails := range res.Volumes {
		if *volumeDetails.State == "in-use" {
			experimentsDetails.TargetVolumeIDList = append(experimentsDetails.TargetVolumeIDList, *volumeDetails.Attachments[0].VolumeId)
		}

	}
	if len(experimentsDetails.TargetVolumeIDList) == 0 {
		return errors.Errorf("fail to get any attaced volumes to detach using tag: %v", experimentsDetails.VolumeTag)
	}

	log.InfoWithValues("[Info]: Targeting the attached volumes,", logrus.Fields{
		"Total number of volume filtered": len(res.Volumes),
		"Number of attached volumes":      len(experimentsDetails.TargetVolumeIDList),
	})

	return nil
}

//GetVolumeAttachmentDetails will give the attachment information of the ebs volume
func GetVolumeAttachmentDetails(volumeID, volumeTag, region string) (string, string, error) {

	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
		Config:            aws.Config{Region: aws.String(region)},
	}))

	params := setVolumeParameter(volumeTag)
	ec2Svc := ec2.New(sess)
	res, err := ec2Svc.DescribeVolumes(params)
	if err != nil {
		return "", "", errors.Errorf("fail to describe the volumes of given tag, err: %v", err.Error())
	}
	for _, volumeDetails := range res.Volumes {
		if *volumeDetails.VolumeId == volumeID {
			return *volumeDetails.Attachments[0].InstanceId, *volumeDetails.Attachments[0].Device, nil
		}
	}

	return "", "", errors.Errorf("no attachment details found for the given volume")
}

//setVolumeParameter will set a filter to get the volume with a given tag
func setVolumeParameter(ebsVolumeTag string) *ec2.DescribeVolumesInput {
	volumeTag := strings.Split(ebsVolumeTag, ":")
	params := &ec2.DescribeVolumesInput{
		Filters: []*ec2.Filter{
			&ec2.Filter{
				Name: aws.String("tag:" + volumeTag[0]),
				Values: []*string{
					aws.String(volumeTag[1]),
				},
			},
		},
	}
	return params
}
