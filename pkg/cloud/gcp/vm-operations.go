package gcp

import (
	"time"

	experimentTypes "github.com/litmuschaos/litmus-go/pkg/gcp/gcp-vm-instance-stop/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/compute/v1"
)

// VMInstanceStop stops a VM Instance
func VMInstanceStop(computeService *compute.Service, instanceName string, gcpProjectID string, instanceZone string) error {

	// stop the requisite VM instance
	_, err := computeService.Instances.Stop(gcpProjectID, instanceZone, instanceName).Do()
	if err != nil {
		return err
	}

	log.InfoWithValues("Stopping VM instance:", logrus.Fields{
		"InstanceName": instanceName,
		"InstanceZone": instanceZone,
	})

	return nil
}

// VMInstanceStart starts a VM instance
func VMInstanceStart(computeService *compute.Service, instanceName string, gcpProjectID string, instanceZone string) error {

	// start the requisite VM instance
	_, err := computeService.Instances.Start(gcpProjectID, instanceZone, instanceName).Do()
	if err != nil {
		return err
	}

	log.InfoWithValues("Starting VM instance:", logrus.Fields{
		"InstanceName": instanceName,
		"InstanceZone": instanceZone,
	})

	return nil
}

// WaitForVMInstanceDown will wait for the VM instance to attain the TERMINATED status
func WaitForVMInstanceDown(computeService *compute.Service, timeout int, delay int, instanceName string, gcpProjectID string, instanceZone string) error {
	log.Info("[Status]: Checking VM instance status")
	return retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {
			instanceState, err := GetVMInstanceStatus(computeService, instanceName, gcpProjectID, instanceZone)
			if err != nil {
				return errors.Errorf("failed to get the %s instance status", instanceName)
			}
			if instanceState != "TERMINATED" {
				log.Infof("The %s instance state is %v", instanceName, instanceState)
				return errors.Errorf("%s instance is not yet in stopped state", instanceName)
			}
			log.Infof("The %s instance state is %v", instanceName, instanceState)
			return nil
		})
}

// WaitForVMInstanceUp will wait for the VM instance to attain the RUNNING status
func WaitForVMInstanceUp(computeService *compute.Service, timeout int, delay int, instanceName string, gcpProjectID string, instanceZone string) error {
	log.Info("[Status]: Checking VM instance status")
	return retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {

			instanceState, err := GetVMInstanceStatus(computeService, instanceName, gcpProjectID, instanceZone)
			if err != nil {
				return errors.Errorf("failed to get the %s instance status", instanceName)
			}
			if instanceState != "RUNNING" {
				log.Infof("The %s instance state is %v", instanceName, instanceState)
				return errors.Errorf("%s instance is not yet in running state", instanceName)
			}
			log.Infof("The %s instance state is %v", instanceName, instanceState)
			return nil
		})
}

//SetTargetInstance will select the target vm instances which are in RUNNING state and filtered from the given label
func SetTargetInstance(computeService *compute.Service, experimentsDetails *experimentTypes.ExperimentDetails) error {

	if experimentsDetails.InstanceLabel == "" {
		return errors.Errorf("label not found, please provide a valid label")
	}

	response, err := computeService.Instances.List(experimentsDetails.GCPProjectID, experimentsDetails.InstanceZone).Filter("labels." + experimentsDetails.InstanceLabel).Do()
	if err != nil {
		return (err)
	}

	for _, instance := range response.Items {
		if instance.Status == "RUNNING" {
			experimentsDetails.TargetVMInstanceNameList = append(experimentsDetails.TargetVMInstanceNameList, instance.Name)
		}
	}

	if len(experimentsDetails.TargetVMInstanceNameList) == 0 {
		return errors.Errorf("no RUNNING VM instances found with the label: %s", experimentsDetails.InstanceLabel)
	}

	log.InfoWithValues("[Info]: Targeting the RUNNING VM instances filtered from instance label", logrus.Fields{
		"Number of running instances filtered": len(experimentsDetails.TargetVMInstanceNameList),
	})

	return nil
}
