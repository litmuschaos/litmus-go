package aws

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/litmuschaos/litmus-go/pkg/cloud/aws/common"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/kube-aws/ebs-loss/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/sirupsen/logrus"
)

// EBSVolumeDetach will detach the ebs volume from ec2 instance
func EBSVolumeDetach(ebsVolumeID, region string) error {

	// Load session from shared config
	sess := common.GetAWSSession(region)

	// Create new EC2 client
	ec2Svc := ec2.New(sess)
	input := &ec2.DetachVolumeInput{
		VolumeId: aws.String(ebsVolumeID),
	}

	result, err := ec2Svc.DetachVolume(input)
	if err != nil {
		return cerrors.Error{
			ErrorCode: cerrors.ErrorTypeChaosInject,
			Reason:    fmt.Sprintf("failed to detach volume: %v", common.CheckAWSError(err).Error()),
			Target:    fmt.Sprintf("{EBS Volume ID: %v, Region: %v}", ebsVolumeID, region),
		}
	}

	log.InfoWithValues("Detaching EBS having:", logrus.Fields{
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
	sess := common.GetAWSSession(region)

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
		return cerrors.Error{
			ErrorCode: cerrors.ErrorTypeChaosRevert,
			Reason:    fmt.Sprintf("failed to attach volume: %v", common.CheckAWSError(err).Error()),
			Target:    fmt.Sprintf("{EBS Volume ID: %v, Region: %v}", ebsVolumeID, region),
		}
	}

	log.InfoWithValues("Attaching EBS having:", logrus.Fields{
		"VolumeId":   *result.VolumeId,
		"State":      *result.State,
		"Device":     *result.Device,
		"InstanceId": *result.InstanceId,
	})
	return nil
}

// SetTargetVolumeIDs will filter out the volume under chaos
func SetTargetVolumeIDs(experimentsDetails *experimentTypes.ExperimentDetails) error {

	sess := common.GetAWSSession(experimentsDetails.Region)

	params := getVolumeFilter(experimentsDetails.VolumeTag)
	ec2Svc := ec2.New(sess)
	res, err := ec2Svc.DescribeVolumes(params)
	if err != nil {
		return cerrors.Error{
			ErrorCode: cerrors.ErrorTypeTargetSelection,
			Reason:    fmt.Sprintf("failed to fetch the volumes with the given tag: %v", err),
			Target:    fmt.Sprintf("{EBS Volume Tag: %v, Region: %v}", experimentsDetails.VolumeTag, experimentsDetails.Region),
		}
	}
	for _, volumeDetails := range res.Volumes {
		if *volumeDetails.State == "in-use" {
			experimentsDetails.TargetVolumeIDList = append(experimentsDetails.TargetVolumeIDList, *volumeDetails.Attachments[0].VolumeId)
		}

	}
	if len(experimentsDetails.TargetVolumeIDList) == 0 {
		return cerrors.Error{
			ErrorCode: cerrors.ErrorTypeTargetSelection,
			Reason:    "no attached volumes found",
			Target:    fmt.Sprintf("{EBS Volume Tag: %v, Region: %v}", experimentsDetails.VolumeTag, experimentsDetails.Region),
		}
	}

	log.InfoWithValues("[Info]: Targeting the attached volumes,", logrus.Fields{
		"Total number of volume filtered": len(res.Volumes),
		"Number of attached volumes":      len(experimentsDetails.TargetVolumeIDList),
	})

	return nil
}

// GetVolumeAttachmentDetails will give the attachment information of the ebs volume
func GetVolumeAttachmentDetails(volumeID, volumeTag, region string) (string, string, error) {

	sess := common.GetAWSSession(region)

	ec2Svc := ec2.New(sess)
	param := getVolumeFilter(volumeTag)
	res, err := ec2Svc.DescribeVolumes(param)
	if err != nil {
		return "", "", cerrors.Error{
			ErrorCode: cerrors.ErrorTypeTargetSelection,
			Reason:    fmt.Sprintf("failed to fetch the volumes with the given tag: %v", err),
			Target:    fmt.Sprintf("{EBS Volume Tag: %v, Region: %v}", volumeTag, region),
		}
	}
	for _, volumeDetails := range res.Volumes {
		if *volumeDetails.VolumeId == volumeID {
			//As the first item of the attachment list contains the attachment details
			return *volumeDetails.Attachments[0].InstanceId, *volumeDetails.Attachments[0].Device, nil
		}
	}
	return "", "", cerrors.Error{
		ErrorCode: cerrors.ErrorTypeTargetSelection,
		Reason:    "no attachment details found for the given volumeID",
		Target:    fmt.Sprintf("{EBS Volume ID: %v, Region: %v}", volumeID, region),
	}
}

// getVolumeFilter will set a filter and return to get the volume with a given tag
func getVolumeFilter(ebsVolumeTag string) *ec2.DescribeVolumesInput {
	if ebsVolumeTag != "" {
		volumeTag := strings.Split(ebsVolumeTag, ":")
		params := &ec2.DescribeVolumesInput{
			Filters: []*ec2.Filter{
				{
					Name: aws.String("tag:" + volumeTag[0]),
					Values: []*string{
						aws.String(volumeTag[1]),
					},
				},
			},
		}
		return params
	}
	return nil
}
