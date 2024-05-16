package aws

import (
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/litmuschaos/litmus-go/pkg/cloud/aws/common"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/kube-aws/ebs-loss/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/palantir/stacktrace"
	"github.com/sirupsen/logrus"
)

// WaitForVolumeDetachment will wait the ebs volume to completely detach
func WaitForVolumeDetachment(ebsVolumeID, ec2InstanceID, region string, delay, timeout int) error {
	log.Info("[Status]: Checking EBS volume status for detachment")
	return retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {
			volumeState, err := GetEBSStatus(ebsVolumeID, ec2InstanceID, region)
			if err != nil {
				return stacktrace.Propagate(err, "failed to get the volume state")
			}
			// We are checking the the attached state as well here as in case of PVs the volume may get attached itself
			// To check if all the volumes have undergone detachment process we make use of CheckEBSDetachmentInitialisation
			// TODO: Need to check an optimised approach to do this using apis.
			if volumeState != "detached" && volumeState != "attached" {
				log.Infof("[Info]: The volume state is %v", volumeState)
				return cerrors.Error{
					ErrorCode: cerrors.ErrorTypeChaosInject,
					Reason:    "volume is not detached within timeout",
					Target:    fmt.Sprintf("{EBS Volume ID: %v, EC2 Instance ID: %v, Region: %v}", ebsVolumeID, ec2InstanceID, region),
				}
			}
			log.Infof("[Info]: The volume state is %v", volumeState)
			return nil
		})
}

// WaitForVolumeAttachment will wait for the ebs volume to get attached on ec2 instance
func WaitForVolumeAttachment(ebsVolumeID, ec2InstanceID, region string, delay, timeout int) error {
	log.Info("[Status]: Checking EBS volume status for attachment")
	return retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {

			volumeState, err := GetEBSStatus(ebsVolumeID, ec2InstanceID, region)
			if err != nil {
				return stacktrace.Propagate(err, "failed to get the volume status")
			}
			if volumeState != "attached" {
				log.Infof("[Info]: The volume state is %v", volumeState)
				return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosRevert,
					Reason: "volume is not attached within timeout",
					Target: fmt.Sprintf("{EBS Volume ID: %v, EC2 Instance ID: %v, Region: %v}", ebsVolumeID, ec2InstanceID, region),
				}
			}
			log.Infof("[Info]: The volume state is %v", volumeState)
			return nil
		})
}

// GetEBSStatus will verify and give the ec2 instance details along with ebs volume details.
func GetEBSStatus(ebsVolumeID, ec2InstanceID, region string) (string, error) {

	// Load session from shared config
	sess := common.GetAWSSession(region)

	// Create new EC2 client
	ec2Svc := ec2.New(sess)
	input := &ec2.DescribeVolumesInput{}

	// Call to get detailed information on each instance
	result, err := ec2Svc.DescribeVolumes(input)
	if err != nil {
		return "", cerrors.Error{
			ErrorCode: cerrors.ErrorTypeStatusChecks,
			Reason:    fmt.Sprintf("failed to get EBS volume status: %v", common.CheckAWSError(err).Error()),
			Target:    fmt.Sprintf("{EBS Volume ID: %v, EC2 Instance ID: %v, Region: %v}", ebsVolumeID, ec2InstanceID, region),
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
	return "", cerrors.Error{
		ErrorCode: cerrors.ErrorTypeStatusChecks,
		Reason:    "unable to find the EBS volume",
		Target:    fmt.Sprintf("{EBS Volume ID: %v, EC2 Instance ID: %v, Region: %v}", ebsVolumeID, ec2InstanceID, region),
	}
}

// EBSStateCheckByID will check the attachment state of the given volume
func EBSStateCheckByID(volumeIDs, region string) error {

	volumeIDList := strings.Split(volumeIDs, ",")
	if len(volumeIDList) == 0 {
		return cerrors.Error{
			ErrorCode: cerrors.ErrorTypeStatusChecks,
			Reason:    "no volumeID provided, please provide a volume to detach",
			Target:    fmt.Sprintf("{Region: %v}", region),
		}
	}
	for _, id := range volumeIDList {
		instanceID, _, err := GetVolumeAttachmentDetails(id, "", region)
		if err != nil {
			return stacktrace.Propagate(err, "failed to get the instanceID for the given volume")
		}
		volumeState, err := GetEBSStatus(id, instanceID, region)
		if err != nil || volumeState != "attached" {
			return cerrors.Error{
				ErrorCode: cerrors.ErrorTypeStatusChecks,
				Reason:    "failed to get the EBS volume in attached state",
				Target:    fmt.Sprintf("{EBS Volume ID: %v, Region: %v}", id, region),
			}
		}
	}
	return nil
}

// PostChaosVolumeStatusCheck is the volume state check after chaos completion
func PostChaosVolumeStatusCheck(experimentsDetails *experimentTypes.ExperimentDetails) error {

	var ec2InstanceIDList []string
	for i, volumeID := range experimentsDetails.TargetVolumeIDList {
		//Get volume attachment details
		ec2InstanceID, _, err := GetVolumeAttachmentDetails(volumeID, experimentsDetails.VolumeTag, experimentsDetails.Region)
		if err != nil || ec2InstanceID == "" {
			return stacktrace.Propagate(err, "failed to get the attachment info")
		}
		ec2InstanceIDList = append(ec2InstanceIDList, ec2InstanceID)

		//Getting the EBS volume attachment status
		ebsState, err := GetEBSStatus(volumeID, ec2InstanceIDList[i], experimentsDetails.Region)
		if err != nil {
			return stacktrace.Propagate(err, "failed to get the EBS status")
		}
		if ebsState != "attached" {
			return cerrors.Error{
				ErrorCode: cerrors.ErrorTypeStatusChecks,
				Reason:    "Volume is not in attached state post chaos",
				Target:    fmt.Sprintf("{EBS Volume ID: %v, Region: %v}", volumeID, experimentsDetails.Region),
			}
		}
	}
	return nil
}

// CheckEBSDetachmentInitialisation will check the start of volume detachment process
func CheckEBSDetachmentInitialisation(volumeIDs []string, instanceID []string, region string) error {
	timeout := 3
	delay := 1
	return retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {

			for i, id := range volumeIDs {
				currentVolumeState, err := GetEBSStatus(id, instanceID[i], region)
				if err != nil {
					return stacktrace.Propagate(err, "failed to get the volume status")
				}
				if currentVolumeState == "attached" {
					return cerrors.Error{
						ErrorCode: cerrors.ErrorTypeChaosInject,
						Reason:    "The volume detachment process hasn't started yet",
						Target:    fmt.Sprintf("{EBS Volume ID: %v, EC2 Instance ID: %v, Region: %v}", id, instanceID[i], region),
					}
				}
			}
			return nil
		})
}
