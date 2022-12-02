package gcp

import (
	"fmt"
	"strings"

	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"google.golang.org/api/compute/v1"
)

// GetVMInstanceStatus returns the status of a VM instance
func GetVMInstanceStatus(computeService *compute.Service, instanceName string, gcpProjectID string, instanceZone string) (string, error) {

	var err error

	// get information about the requisite VM instance
	response, err := computeService.Instances.Get(gcpProjectID, instanceZone, instanceName).Do()
	if err != nil {
		return "", cerrors.Error{ErrorCode: cerrors.ErrorTypeStatusChecks, Target: fmt.Sprintf("{vmName: %s, zone: %s}", instanceName, instanceZone), Reason: err.Error()}
	}

	// return the VM status
	return response.Status, nil
}

// InstanceStatusCheckByName is used to check the status of all the VM instances under chaos
func InstanceStatusCheckByName(computeService *compute.Service, managedInstanceGroup string, delay, timeout int, check string, instanceNames string, gcpProjectId string, instanceZones string) error {

	instanceNamesList := strings.Split(instanceNames, ",")

	instanceZonesList := strings.Split(instanceZones, ",")

	if managedInstanceGroup != "enable" && managedInstanceGroup != "disable" {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeStatusChecks, Target: fmt.Sprintf("{vmNames: %s, zones: %s}", instanceNamesList, instanceZonesList), Reason: fmt.Sprintf("invalid value for MANAGED_INSTANCE_GROUP: %s", managedInstanceGroup)}
	}

	if len(instanceNamesList) == 0 {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeStatusChecks, Target: fmt.Sprintf("{vmNames: %v}", instanceNamesList), Reason: "no vm instance name found to stop"}
	}

	if len(instanceNamesList) != len(instanceZonesList) {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeStatusChecks, Target: fmt.Sprintf("{vmNames: %v, zones: %v}", instanceNamesList, instanceZonesList), Reason: "unequal number of vm instance names and zones"}
	}

	log.Infof("[Info]: The vm instances under chaos (IUC) are: %v", instanceNamesList)

	return InstanceStatusCheck(computeService, instanceNamesList, gcpProjectId, instanceZonesList)
}

// InstanceStatusCheck is used to check whether all VM instances under chaos are running or not
func InstanceStatusCheck(computeService *compute.Service, instanceNamesList []string, gcpProjectId string, instanceZonesList []string) error {

	var zone string

	for i := range instanceNamesList {

		zone = instanceZonesList[0]
		if len(instanceZonesList) > 1 {
			zone = instanceZonesList[i]
		}

		instanceState, err := GetVMInstanceStatus(computeService, instanceNamesList[i], gcpProjectId, zone)
		if err != nil {
			return err
		}

		if instanceState != "RUNNING" {
			return cerrors.Error{ErrorCode: cerrors.ErrorTypeStatusChecks, Target: fmt.Sprintf("{vmName: %s, zone: %s}", instanceNamesList[i], zone), Reason: fmt.Sprintf("vm instance is not in RUNNING state, current state: %s", instanceState)}
		}
	}

	return nil
}
