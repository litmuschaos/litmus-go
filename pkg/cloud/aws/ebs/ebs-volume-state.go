package aws

import (
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/litmuschaos/litmus-go/pkg/cloud/aws/common"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/kube-aws/ebs-loss/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// WaitForVolumeDetachment will wait the ebs volume to completely detach
func WaitForVolumeDetachment(ebsVolumeID, ec2InstanceID, region string, delay, timeout int) error {

	log.Info("[Status]: Checking ebs volume status for detachment")
	return retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {
			volumeState, err := GetEBSStatus(ebsVolumeID, ec2InstanceID, region)
			if err != nil {
				return errors.Errorf("failed to get the volume state")
			}
			// We are checking the the attached state as well here as in case of PVs the volume may get attached itself
			// To check if all the volumes have undergone detachment process we make use of CheckEBSDetachmentInitialisation
			// TODO: Need to check an optimised approach to do this using apis.
			if volumeState != "detached" && volumeState != "attached" {
				log.Infof("[Info]: The volume state is %v", volumeState)
				return errors.Errorf("volume is not yet in detached state")
			}
			log.Infof("[Info]: The volume state is %v", volumeState)
			return nil
		})
}

// WaitForVolumeAttachment will wait for the ebs volume to get attached on ec2 instance
func WaitForVolumeAttachment(ebsVolumeID, ec2InstanceID, region string, delay, timeout int) error {

	log.Info("[Status]: Checking ebs volume status for attachment")
	return retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {

			volumeState, err := GetEBSStatus(ebsVolumeID, ec2InstanceID, region)
			if err != nil {
				return errors.Errorf("failed to get the volume status")
			}
			if volumeState != "attached" {
				log.Infof("[Info]: The volume state is %v", volumeState)
				return errors.Errorf("volume is not yet in attached state")
			}
			log.Infof("[Info]: The volume state is %v", volumeState)
			return nil
		})
}

//GetEBSStatus will verify and give the ec2 instance details along with ebs volume details.
func GetEBSStatus(ebsVolumeID, ec2InstanceID, region string) (string, error) {

	// Load session from shared config
	sess := common.GetAWSSession(region)

	// Create new EC2 client
	ec2Svc := ec2.New(sess)
	input := &ec2.DescribeVolumesInput{}

	// Call to get detailed information on each instance
	result, err := ec2Svc.DescribeVolumes(input)
	if err != nil {
		return "", common.CheckAWSError(err)
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

//EBSStateCheckByID will check the attachment state of the given volume
func EBSStateCheckByID(volumeIDs, region string) error {

	volumeIDList := strings.Split(volumeIDs, ",")
	if len(volumeIDList) == 0 {
		return errors.Errorf("no volumeID provided, please provide a volume to detach")
	}
	for _, id := range volumeIDList {
		instanceID, _, err := GetVolumeAttachmentDetails(id, "", region)
		if err != nil {
			return errors.Errorf("fail to get the instanceID for the given volume, err: %v", err)
		}
		volumeState, err := GetEBSStatus(id, instanceID, region)
		if err != nil || volumeState != "attached" {
			return errors.Errorf("fail to get the ebs volume %v in attached state, err: %v", id, err)
		}
	}
	return nil
}

//PostChaosVolumeStatusCheck is the volume state check after chaos completion
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

//CheckEBSDetachmentInitialisation will check the start of volume detachment process
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
					return errors.Errorf("failed to get the volume status")
				}
				if currentVolumeState == "attached" {
					return errors.Errorf("the volume detachment has not started yet for volume %v", id)
				}
			}
			return nil
		})
}
