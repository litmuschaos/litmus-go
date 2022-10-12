package gcp

import (
	"strings"

	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/pkg/errors"
	"google.golang.org/api/compute/v1"
)

// GetVMInstanceStatus returns the status of a VM instance
func GetVMInstanceStatus(computeService *compute.Service, instanceName string, gcpProjectID string, instanceZone string) (string, error) {

	var err error

	// get information about the requisite VM instance
	response, err := computeService.Instances.Get(gcpProjectID, instanceZone, instanceName).Do()
	if err != nil {
		return "", err
	}

	// return the VM status
	return response.Status, nil
}

// InstanceStatusCheckByName is used to check the status of all the VM instances under chaos
func InstanceStatusCheckByName(computeService *compute.Service, managedInstanceGroup string, delay, timeout int, check string, instanceNames string, gcpProjectId string, instanceZones string) error {

	instanceNamesList := strings.Split(instanceNames, ",")

	instanceZonesList := strings.Split(instanceZones, ",")

	if managedInstanceGroup != "enable" && managedInstanceGroup != "disable" {
		return errors.Errorf("invalid value for MANAGED_INSTANCE_GROUP: %v", managedInstanceGroup)
	}

	if len(instanceNamesList) == 0 {
		return errors.Errorf("no vm instance name found to stop")
	}

	if len(instanceNamesList) != len(instanceZonesList) {
		return errors.Errorf("the number of vm instance names and the number of regions are not equal")
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
			return errors.Errorf("%s vm instance is not in RUNNING state, current state: %v", instanceNamesList[i], instanceState)
		}
	}

	return nil
}
