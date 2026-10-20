package gcp

import (
	"fmt"
	"strings"

	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/gcp/gcp-vm-disk-loss/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/compute/v1"
)

// DiskVolumeDetach will detach a disk volume from a VM instance
func DiskVolumeDetach(computeService *compute.Service, instanceName string, gcpProjectID string, zone string, deviceName string) error {

	response, err := computeService.Instances.DetachDisk(gcpProjectID, zone, instanceName, deviceName).Do()
	if err != nil {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosInject, Target: fmt.Sprintf("{deviceName: %s, zone: %s}", deviceName, zone), Reason: err.Error()}
	}

	log.InfoWithValues("Detaching disk having:", logrus.Fields{
		"Device":       deviceName,
		"InstanceName": instanceName,
		"Status":       response.Status,
	})

	return nil
}

// DiskVolumeAttach will attach a disk volume to a VM instance
func DiskVolumeAttach(computeService *compute.Service, instanceName string, gcpProjectID string, zone string, deviceName string, diskName string) error {

	diskDetails, err := computeService.Disks.Get(gcpProjectID, zone, diskName).Do()
	if err != nil {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosRevert, Target: fmt.Sprintf("{diskName: %s, zone: %s}", diskName, zone), Reason: err.Error()}
	}

	requestBody := &compute.AttachedDisk{
		DeviceName: deviceName,
		Source:     diskDetails.SelfLink,
	}

	response, err := computeService.Instances.AttachDisk(gcpProjectID, zone, instanceName, requestBody).Do()
	if err != nil {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosRevert, Target: fmt.Sprintf("{diskName: %s, zone: %s}", diskName, zone), Reason: err.Error()}
	}

	log.InfoWithValues("Attaching disk having:", logrus.Fields{
		"Device":       deviceName,
		"InstanceName": instanceName,
		"Status":       response.Status,
	})

	return nil
}

// GetVolumeAttachmentDetails returns the name of the VM instance attached to a disk volume
func GetVolumeAttachmentDetails(computeService *compute.Service, gcpProjectID string, zone string, diskName string) (string, error) {

	diskDetails, err := computeService.Disks.Get(gcpProjectID, zone, diskName).Do()
	if err != nil {
		return "", cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Target: fmt.Sprintf("{diskName: %s, zone: %s}", diskName, zone), Reason: err.Error()}
	}

	if len(diskDetails.Users) > 0 {
		// diskDetails.Users[0] is the URL that links to the user of the disk (attached instance) in the form: projects/project/zones/zone/instances/instance
		// hence we split the URL string via the '/' delimiter and get the string in the last index position to get the instance name
		splitUserURL := strings.Split(diskDetails.Users[0], "/")
		attachedInstanceName := splitUserURL[len(splitUserURL)-1]

		return attachedInstanceName, nil
	}

	return "", cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Target: fmt.Sprintf("{diskName: %s, zone: %s}", diskName, zone), Reason: fmt.Sprintf("%s disk is not attached to any vm instance", diskName)}
}

// GetDiskDeviceNameForVM returns the device name for the target disk for a given VM
func GetDiskDeviceNameForVM(computeService *compute.Service, targetDiskName, gcpProjectID, zone, instanceName string) (string, error) {

	instanceDetails, err := computeService.Instances.Get(gcpProjectID, zone, instanceName).Do()
	if err != nil {
		return "", cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Target: fmt.Sprintf("{diskName: %s, zone: %s}", targetDiskName, zone), Reason: err.Error()}
	}

	for _, disk := range instanceDetails.Disks {

		// disk.Source is the URL of the disk resource in the form: projects/project/zones/zone/disks/disk
		// hence we split the URL string via the '/' delimiter and get the string in the last index position to get the disk name
		splitDiskURL := strings.Split(disk.Source, "/")
		diskName := splitDiskURL[len(splitDiskURL)-1]

		if diskName == targetDiskName {
			return disk.DeviceName, nil
		}
	}

	return "", cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Target: fmt.Sprintf("{diskName: %s, zone: %s}", targetDiskName, zone), Reason: fmt.Sprintf("%s disk not found for %s vm instance", targetDiskName, instanceName)}
}

// SetTargetDiskVolumes will select the target disk volumes which are attached to some VM instance and filtered from the given label
func SetTargetDiskVolumes(computeService *compute.Service, experimentsDetails *experimentTypes.ExperimentDetails) error {

	var (
		response *compute.DiskList
		err      error
	)

	if strings.Contains(experimentsDetails.DiskVolumeLabel, ":") {
		// the label is of format key:value
		response, err = computeService.Disks.List(experimentsDetails.GCPProjectID, experimentsDetails.Zones).Filter("labels." + experimentsDetails.DiskVolumeLabel).Do()
	} else {
		// the label only has key
		response, err = computeService.Disks.List(experimentsDetails.GCPProjectID, experimentsDetails.Zones).Filter("labels." + experimentsDetails.DiskVolumeLabel + ":*").Do()
	}
	if err != nil {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeTargetSelection, Target: fmt.Sprintf("{label: %s, zone: %s}", experimentsDetails.DiskVolumeLabel, experimentsDetails.Zones), Reason: err.Error()}
	}

	for _, disk := range response.Items {
		if len(disk.Users) > 0 {
			experimentsDetails.TargetDiskVolumeNamesList = append(experimentsDetails.TargetDiskVolumeNamesList, disk.Name)
		}
	}

	if len(experimentsDetails.TargetDiskVolumeNamesList) == 0 {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeTargetSelection, Target: fmt.Sprintf("{label: %s, zone: %s}", experimentsDetails.DiskVolumeLabel, experimentsDetails.Zones), Reason: "no attached disk volumes found with the given label"}
	}

	log.InfoWithValues("[Info]: Targeting the attached disk volumes filtered from disk label", logrus.Fields{
		"Number of attached disk volumes filtered": len(experimentsDetails.TargetDiskVolumeNamesList),
		"Attached disk volume names":               experimentsDetails.TargetDiskVolumeNamesList,
	})

	return nil
}
