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
		return errors.Errorf(err.Error())
	}

	// create a new GCP Compute Service client using the GCP service account credentials
	computeService, err := compute.NewService(ctx, option.WithCredentialsJSON(json))
	if err != nil {
		return errors.Errorf(err.Error())
	}

	response, err := computeService.Instances.DetachDisk(gcpProjectID, zone, instanceName, deviceName).Context(ctx).Do()
	if err != nil {
		return errors.Errorf(err.Error())
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
		return errors.Errorf(err.Error())
	}

	// create a new GCP Compute Service client using the GCP service account credentials
	computeService, err := compute.NewService(ctx, option.WithCredentialsJSON(json))
	if err != nil {
		return errors.Errorf(err.Error())
	}

	diskDetails, err := computeService.Disks.Get(gcpProjectID, zone, diskName).Context(ctx).Do()
	if err != nil {
		return errors.Errorf(err.Error())
	}

	requestBody := &compute.AttachedDisk{
		DeviceName: deviceName,
		Source:     diskDetails.SelfLink,
	}

	response, err := computeService.Instances.AttachDisk(gcpProjectID, zone, instanceName, requestBody).Context(ctx).Do()
	if err != nil {
		return errors.Errorf(err.Error())
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

	if len(diskDetails.Users) > 0 {
		// diskDetails.Users[0] is the URL that links to the user of the disk (attached instance) in the form: projects/project/zones/zone/instances/instance
		// hence we split the URL string via the '/' delimiter and get the string in the last index position to get the instance name
		splitUserURL := strings.Split(diskDetails.Users[0], "/")
		attachedInstanceName := splitUserURL[len(splitUserURL)-1]

		return attachedInstanceName, nil
	}

	return "", errors.Errorf("disk not attached to any instance")
}
