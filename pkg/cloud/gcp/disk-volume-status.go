package gcp

import (
	"strings"
	"time"

	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/option"
)

// WaitForVolumeDetachment will wait for the disk volume to completely detach from a VM instance
func WaitForVolumeDetachment(diskName string, gcpProjectID string, instanceName string, zone string, delay int, timeout int) error {

	log.Info("[Status]: Checking disk volume status for detachment")
	return retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {

			volumeState, err := GetDiskVolumeState(diskName, gcpProjectID, instanceName, zone)
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
func WaitForVolumeAttachment(diskName string, gcpProjectID string, instanceName string, zone string, delay int, timeout int) error {

	log.Info("[Status]: Checking disk volume status for attachment")
	return retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {

			volumeState, err := GetDiskVolumeState(diskName, gcpProjectID, instanceName, zone)
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

//GetDiskVolumeState will verify and give the VM instance details along with the disk volume details
func GetDiskVolumeState(diskName string, gcpProjectID string, instanceName string, zone string) (string, error) {

	// create an empty context
	ctx := context.Background()

	json, err := GetServiceAccountJSONFromSecret()
	if err != nil {
		return "", errors.Errorf(err.Error())
	}

	// create a new GCP Compute Service client using the GCP service account credentials
	computeService, err := compute.NewService(ctx, option.WithCredentialsJSON(json))
	if err != nil {
		return "", errors.Errorf(err.Error())
	}

	diskDetails, err := computeService.Disks.Get(gcpProjectID, zone, diskName).Context(ctx).Do()
	if err != nil {
		return "", errors.Errorf(err.Error())
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

//DiskVolumeStateCheck will check the attachment state of the given volume
func DiskVolumeStateCheck(gcpProjectID string, zones string, diskNames string) error {

	diskNamesList := strings.Split(diskNames, ",")
	if len(diskNamesList) == 0 {
		return errors.Errorf("no disk name provided, please provide the name of the disk to detach")
	}

	zonesList := strings.Split(zones, ",")
	if len(zonesList) == 0 {
		return errors.Errorf("no zones provided, please provide the zone of the disk to be detached")
	}

	if len(diskNamesList) != len(zonesList) {
		return errors.Errorf("unequal number of disk names and zones found")
	}

	for i := range diskNamesList {

		instanceName, err := GetVolumeAttachmentDetails(gcpProjectID, zonesList[i], diskNamesList[i])
		if err != nil {
			return errors.Errorf("failed to get the vm instance name for the given volume, err: %v", err)
		}

		volumeState, err := GetDiskVolumeState(diskNamesList[i], gcpProjectID, instanceName, zonesList[i])
		if err != nil || volumeState != "attached" {
			return errors.Errorf("failed to get the disk volume %v in attached state, err: %v", diskNames[i], err)
		}
	}

	return nil
}

//CheckDiskVolumeDetachmentInitialisation will check the start of volume detachment process
func CheckDiskVolumeDetachmentInitialisation(gcpProjectID string, diskNamesList []string, instanceNamesList []string, zones []string) error {

	timeout := 3
	delay := 1
	return retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {

			for i := range diskNamesList {
				currentVolumeState, err := GetDiskVolumeState(diskNamesList[i], gcpProjectID, instanceNamesList[i], zones[i])
				if err != nil {
					return errors.Errorf("failed to get the volume status")
				}
				if currentVolumeState == "attached" {
					return errors.Errorf("the volume detachment has not started yet for volume %v", diskNamesList[i])
				}
			}
			return nil
		})
}
