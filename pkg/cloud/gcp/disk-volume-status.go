package gcp

import (
	"strings"
	"time"

	experimentTypes "github.com/litmuschaos/litmus-go/pkg/gcp/gcp-vm-disk-loss/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/compute/v1"
)

// WaitForVolumeDetachment will wait for the disk volume to completely detach from a VM instance
func WaitForVolumeDetachment(computeService *compute.Service, diskName, gcpProjectID, instanceName, zone string, delay, timeout int) error {

	log.Info("[Status]: Checking disk volume status for detachment")
	return retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {

			volumeState, err := GetDiskVolumeState(computeService, diskName, gcpProjectID, instanceName, zone)
			if err != nil {
				return errors.Errorf("failed to get the volume state")
			}

			if volumeState != "detached" {
				log.Infof("[Info]: The volume state is %v", volumeState)
				return errors.Errorf("volume is not yet in detached state")
			}

			log.Infof("[Info]: The volume state is %v", volumeState)
			return nil
		})
}

// WaitForVolumeAttachment will wait for the disk volume to get attached to a VM instance
func WaitForVolumeAttachment(computeService *compute.Service, diskName, gcpProjectID, instanceName, zone string, delay, timeout int) error {

	log.Info("[Status]: Checking disk volume status for attachment")
	return retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {

			volumeState, err := GetDiskVolumeState(computeService, diskName, gcpProjectID, instanceName, zone)
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

// GetDiskVolumeState will verify and give the VM instance details along with the disk volume details
func GetDiskVolumeState(computeService *compute.Service, diskName, gcpProjectID, instanceName, zone string) (string, error) {

	diskDetails, err := computeService.Disks.Get(gcpProjectID, zone, diskName).Do()
	if err != nil {
		return "", err
	}

	for _, user := range diskDetails.Users {

		// 'user' is a URL that links to the users of the disk (attached instances) in the form: projects/project/zones/zone/instances/instance
		// hence we split the URL string via the '/' delimiter and get the string in the last index position to get the instance name
		splitUserURL := strings.Split(user, "/")
		attachedInstanceName := splitUserURL[len(splitUserURL)-1]

		if attachedInstanceName == instanceName {

			// diskDetails.Type is a URL of the disk type resource describing which disk type is used to create the disk in the form: projects/project/zones/zone/diskTypes/diskType
			// hence we split the URL string via the '/' delimiter and get the string in the last index position to get the disk type
			splitDiskTypeURL := strings.Split(diskDetails.Type, "/")
			diskType := splitDiskTypeURL[len(splitDiskTypeURL)-1]

			log.InfoWithValues("The selected disk volume is:", logrus.Fields{
				"VolumeName":   diskName,
				"VolumeType":   diskType,
				"Status":       diskDetails.Status,
				"InstanceName": instanceName,
			})

			return "attached", nil
		}
	}

	return "detached", nil
}

// DiskVolumeStateCheck will check the attachment state of the given volume
func DiskVolumeStateCheck(computeService *compute.Service, experimentsDetails *experimentTypes.ExperimentDetails) error {

	if experimentsDetails.GCPProjectID == "" {
		return errors.Errorf("no gcp project id provided, please provide the project id")
	}

	diskNamesList := strings.Split(experimentsDetails.DiskVolumeNames, ",")
	if len(diskNamesList) == 0 {
		return errors.Errorf("no disk name provided, please provide the name of the disk")
	}

	zonesList := strings.Split(experimentsDetails.Zones, ",")
	if len(zonesList) == 0 {
		return errors.Errorf("no zone provided, please provide the zone of the disk")
	}

	if len(diskNamesList) != len(zonesList) {
		return errors.Errorf("unequal number of disk names and zones found, please verify the input details")
	}

	for i := range diskNamesList {
		instanceName, err := GetVolumeAttachmentDetails(computeService, experimentsDetails.GCPProjectID, zonesList[i], diskNamesList[i])
		if err != nil || instanceName == "" {
			return errors.Errorf("failed to get the vm instance name for %s disk volume, err: %v", diskNamesList[i], err)
		}
	}

	return nil
}

// SetTargetDiskInstanceNames fetches the vm instances to which the disks are attached
func SetTargetDiskInstanceNames(computeService *compute.Service, experimentsDetails *experimentTypes.ExperimentDetails) error {

	diskNamesList := strings.Split(experimentsDetails.DiskVolumeNames, ",")
	zonesList := strings.Split(experimentsDetails.Zones, ",")

	for i := range diskNamesList {
		instanceName, err := GetVolumeAttachmentDetails(computeService, experimentsDetails.GCPProjectID, zonesList[i], diskNamesList[i])
		if err != nil || instanceName == "" {
			return errors.Errorf("failed to get the vm instance name for %s disk volume, err: %v", diskNamesList[i], err)
		}

		experimentsDetails.TargetDiskInstanceNamesList = append(experimentsDetails.TargetDiskInstanceNamesList, instanceName)
	}

	return nil
}
