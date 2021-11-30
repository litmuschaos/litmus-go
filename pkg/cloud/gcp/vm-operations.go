package gcp

import (
	"time"

	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/option"
)

// VMInstanceStop stops a VM Instance
func VMInstanceStop(instanceName string, gcpProjectID string, instanceZone string) error {
	// create an empty context
	ctx := context.Background()

	// get service account credentials json
	json, err := GetServiceAccountJSONFromSecret()
	if err != nil {
		return errors.Errorf(err.Error())
	}

	// create a new GCP Compute Service client using the GCP service account credentials
	computeService, err := compute.NewService(ctx, option.WithCredentialsJSON(json))
	if err != nil {
		return errors.Errorf(err.Error())
	}

	// stop the requisite VM instance
	_, err = computeService.Instances.Stop(gcpProjectID, instanceZone, instanceName).Context(ctx).Do()
	if err != nil {
		return errors.Errorf(err.Error())
	}

	log.InfoWithValues("Stopping VM instance:", logrus.Fields{
		"InstanceName": instanceName,
		"InstanceZone": instanceZone,
	})

	return nil
}

// VMInstanceStart starts a VM instance
func VMInstanceStart(instanceName string, gcpProjectID string, instanceZone string) error {
	// create an empty context
	ctx := context.Background()

	// get service account credentials json
	json, err := GetServiceAccountJSONFromSecret()
	if err != nil {
		return errors.Errorf(err.Error())
	}

	// create a new GCP Compute Service client using the GCP service account credentials
	computeService, err := compute.NewService(ctx, option.WithCredentialsJSON(json))
	if err != nil {
		return errors.Errorf(err.Error())
	}

	// start the requisite VM instance
	_, err = computeService.Instances.Start(gcpProjectID, instanceZone, instanceName).Context(ctx).Do()
	if err != nil {
		return errors.Errorf(err.Error())
	}

	log.InfoWithValues("Starting VM instance:", logrus.Fields{
		"InstanceName": instanceName,
		"InstanceZone": instanceZone,
	})

	return nil
}

// WaitForVMInstanceDown will wait for the VM instance to attain the TERMINATED status
func WaitForVMInstanceDown(timeout int, delay int, instanceName string, gcpProjectID string, instanceZone string) error {
	log.Info("[Status]: Checking VM instance status")
	return retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {
			instanceState, err := GetVMInstanceStatus(instanceName, gcpProjectID, instanceZone)
			if err != nil {
				return errors.Errorf("failed to get the instance status")
			}
			if instanceState != "TERMINATED" {
				log.Infof("The instance state is %v", instanceState)
				return errors.Errorf("instance is not yet in stopped state")
			}
			log.Infof("The instance state is %v", instanceState)
			return nil
		})
}

// WaitForVMInstanceUp will wait for the VM instance to attain the RUNNING status
func WaitForVMInstanceUp(timeout int, delay int, instanceName string, gcpProjectID string, instanceZone string) error {
	log.Info("[Status]: Checking VM instance status")
	return retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {

			instanceState, err := GetVMInstanceStatus(instanceName, gcpProjectID, instanceZone)
			if err != nil {
				return errors.Errorf("failed to get the instance status")
			}
			if instanceState != "RUNNING" {
				log.Infof("The instance state is %v", instanceState)
				return errors.Errorf("instance is not yet in running state")
			}
			log.Infof("The instance state is %v", instanceState)
			return nil
		})
}
