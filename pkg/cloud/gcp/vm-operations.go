package gcp

import (
	"fmt"
	"strings"
	"time"

	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/gcp/gcp-vm-instance-stop/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/palantir/stacktrace"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/compute/v1"
)

// VMInstanceStop stops a VM Instance
func VMInstanceStop(computeService *compute.Service, instanceName string, gcpProjectID string, instanceZone string) error {

	// stop the requisite VM instance
	_, err := computeService.Instances.Stop(gcpProjectID, instanceZone, instanceName).Do()
	if err != nil {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosInject, Target: fmt.Sprintf("{vmName: %s, zone: %s}", instanceName, instanceZone), Reason: err.Error()}
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
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeChaosRevert, Target: fmt.Sprintf("{vmName: %s, zone: %s}", instanceName, instanceZone), Reason: err.Error()}
	}

	log.InfoWithValues("Starting VM instance:", logrus.Fields{
		"InstanceName": instanceName,
		"InstanceZone": instanceZone,
	})

	return nil
}

// WaitForVMInstanceDown will wait for the VM instance to attain the TERMINATED status
func WaitForVMInstanceDown(computeService *compute.Service, timeout int, delay int, instanceName string, gcpProjectID string, instanceZone string) error {

	log.Infof("[Status]: Checking %s VM instance status", instanceName)

	return retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {

			instanceState, err := GetVMInstanceStatus(computeService, instanceName, gcpProjectID, instanceZone)
			if err != nil {
				return stacktrace.Propagate(err, "failed to get the vm instance status")
			}

			log.Infof("The %s vm instance state is %v", instanceName, instanceState)

			if instanceState != "TERMINATED" {
				return cerrors.Error{ErrorCode: cerrors.ErrorTypeStatusChecks, Target: fmt.Sprintf("{vmName: %s, zone: %s}", instanceName, instanceZone), Reason: "vm instance is not yet in stopped state"}
			}

			return nil
		})
}

// WaitForVMInstanceUp will wait for the VM instance to attain the RUNNING status
func WaitForVMInstanceUp(computeService *compute.Service, timeout int, delay int, instanceName string, gcpProjectID string, instanceZone string) error {

	log.Infof("[Status]: Checking %s VM instance status", instanceName)

	return retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {

			instanceState, err := GetVMInstanceStatus(computeService, instanceName, gcpProjectID, instanceZone)
			if err != nil {
				return stacktrace.Propagate(err, "failed to get the vm instance status")
			}

			log.Infof("The %s vm instance state is %v", instanceName, instanceState)

			if instanceState != "RUNNING" {
				return cerrors.Error{ErrorCode: cerrors.ErrorTypeStatusChecks, Target: fmt.Sprintf("{vmName: %s, zone: %s}", instanceName, instanceZone), Reason: "vm instance is not yet in running state"}
			}

			return nil
		})
}

// SetTargetInstance will select the target vm instances which are in RUNNING state and filtered from the given label
func SetTargetInstance(computeService *compute.Service, experimentsDetails *experimentTypes.ExperimentDetails) error {

	var (
		response *compute.InstanceList
		err      error
	)

	if experimentsDetails.InstanceLabel == "" {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeTargetSelection, Target: fmt.Sprintf("{label: %s}", experimentsDetails.InstanceLabel), Reason: "label not found, please provide a valid label"}
	}

	if strings.Contains(experimentsDetails.InstanceLabel, ":") {
		// the label is of format key:value
		response, err = computeService.Instances.List(experimentsDetails.GCPProjectID, experimentsDetails.Zones).Filter("labels." + experimentsDetails.InstanceLabel).Do()
	} else {
		// the label only has key
		response, err = computeService.Instances.List(experimentsDetails.GCPProjectID, experimentsDetails.Zones).Filter("labels." + experimentsDetails.InstanceLabel + ":*").Do()
	}
	if err != nil {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeTargetSelection, Target: fmt.Sprintf("{label: %s, zone: %s}", experimentsDetails.InstanceLabel, experimentsDetails.Zones), Reason: err.Error()}
	}

	for _, instance := range response.Items {
		if instance.Status == "RUNNING" {
			experimentsDetails.TargetVMInstanceNameList = append(experimentsDetails.TargetVMInstanceNameList, instance.Name)
		}
	}

	if len(experimentsDetails.TargetVMInstanceNameList) == 0 {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeTargetSelection, Target: fmt.Sprintf("{label: %s, zone: %s}", experimentsDetails.InstanceLabel, experimentsDetails.Zones), Reason: "no running vm instances found with the given label"}
	}

	log.InfoWithValues("[Info]: Targeting the RUNNING VM instances filtered from instance label", logrus.Fields{
		"Number of running instances filtered": len(experimentsDetails.TargetVMInstanceNameList),
	})

	return nil
}
