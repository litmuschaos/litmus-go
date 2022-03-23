package gcp

import (
	"strings"

	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/option"
)

// DiskVolumeDetach will detach a disk volume from a VM instance
func DiskVolumeDetach(instanceName string, gcpProjectID string, zone string, deviceName string) error {

	// create an empty context
	ctx := context.Background()

	json, err := GetServiceAccountJSONFromSecret()
	if err != nil {
		return err
	}

	// create a new GCP Compute Service client using the GCP service account credentials
	computeService, err := compute.NewService(ctx, option.WithCredentialsJSON(json))
	if err != nil {
		return err
	}

	response, err := computeService.Instances.DetachDisk(gcpProjectID, zone, instanceName, deviceName).Context(ctx).Do()
	if err != nil {
		return err
	}

	log.InfoWithValues("Detaching disk having:", logrus.Fields{
		"Device":       deviceName,
		"InstanceName": instanceName,
		"Status":       response.Status,
	})

	return nil
}

// DiskVolumeAttach will attach a disk volume to a VM instance
func DiskVolumeAttach(instanceName string, gcpProjectID string, zone string, deviceName string, diskName string) error {

	// create an empty context
	ctx := context.Background()

	json, err := GetServiceAccountJSONFromSecret()
	if err != nil {
		return err
	}

	// create a new GCP Compute Service client using the GCP service account credentials
	computeService, err := compute.NewService(ctx, option.WithCredentialsJSON(json))
	if err != nil {
		return err
	}

	diskDetails, err := computeService.Disks.Get(gcpProjectID, zone, diskName).Context(ctx).Do()
	if err != nil {
		return err
	}

	requestBody := &compute.AttachedDisk{
		DeviceName: deviceName,
		Source:     diskDetails.SelfLink,
	}

	response, err := computeService.Instances.AttachDisk(gcpProjectID, zone, instanceName, requestBody).Context(ctx).Do()
	if err != nil {
		return err
	}

	log.InfoWithValues("Attaching disk having:", logrus.Fields{
		"Device":       deviceName,
		"InstanceName": instanceName,
		"Status":       response.Status,
	})

	return nil
}

//GetVolumeAttachmentDetails returns the name of the VM instance attached to a disk volume
func GetVolumeAttachmentDetails(gcpProjectID string, zone string, diskName string) (string, error) {

	// create an empty context
	ctx := context.Background()

	json, err := GetServiceAccountJSONFromSecret()
	if err != nil {
		return "", err
	}

	// create a new GCP Compute Service client using the GCP service account credentials
	computeService, err := compute.NewService(ctx, option.WithCredentialsJSON(json))
	if err != nil {
		return "", err
	}

	diskDetails, err := computeService.Disks.Get(gcpProjectID, zone, diskName).Context(ctx).Do()
	if err != nil {
		return "", err
	}

	if len(diskDetails.Users) > 0 {
		// diskDetails.Users[0] is the URL that links to the user of the disk (attached instance) in the form: projects/project/zones/zone/instances/instance
		// hence we split the URL string via the '/' delimiter and get the string in the last index position to get the instance name
		splitUserURL := strings.Split(diskDetails.Users[0], "/")
		attachedInstanceName := splitUserURL[len(splitUserURL)-1]

		return attachedInstanceName, nil
	}

	return "", errors.Errorf("disk not attached to any instance")
}

// GetDiskDeviceNameForVM returns the device name for the target disk for a given VM
func GetDiskDeviceNameForVM(targetDiskName, gcpProjectID, zone, instanceName string) (string, error) {

	// create an empty context
	ctx := context.Background()

	json, err := GetServiceAccountJSONFromSecret()
	if err != nil {
		return "", err
	}

	// create a new GCP Compute Service client using the GCP service account credentials
	computeService, err := compute.NewService(ctx, option.WithCredentialsJSON(json))
	if err != nil {
		return "", err
	}

	instanceDetails, err := computeService.Instances.Get(gcpProjectID, zone, instanceName).Context(ctx).Do()
	if err != nil {
		return "", err
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

	return "", errors.Errorf("%s disk not found for %s vm instance", targetDiskName, instanceName)
}
