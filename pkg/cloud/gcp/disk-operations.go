package gcp

import (
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/option"
)

func DiskVolumeDetach(instanceName string, gcpProjectID string, instanceZone string, deviceName string) error {

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

	response, err := computeService.Instances.DetachDisk(gcpProjectID, instanceZone, instanceName, deviceName).Context(ctx).Do()
	if err != nil {
		return errors.Errorf(err.Error())
	}

	log.InfoWithValues("Detaching ebs having:", logrus.Fields{
		"Device":       deviceName,
		"InstanceName": instanceName,
		"Status":       response.Status,
	})

	return nil
}

func DiskVolumeAttach(instanceName string, gcpProjectID string, instanceZone string, deviceName string, diskName string) error {

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

	diskDetails, err := computeService.Disks.Get(gcpProjectID, instanceZone, diskName).Context(ctx).Do()
	if err != nil {
		return errors.Errorf(err.Error())
	}

	requestBody := &compute.AttachedDisk{
		DeviceName: deviceName,
		Source:     diskDetails.SelfLink,
	}

	response, err := computeService.Instances.AttachDisk(gcpProjectID, instanceZone, instanceName, requestBody).Context(ctx).Do()
	if err != nil {
		return errors.Errorf(err.Error())
	}

	log.InfoWithValues("Attaching ebs having:", logrus.Fields{
		"Status":       response.Status,
		"Device":       deviceName,
		"InstanceName": instanceName,
	})

	return nil
}
