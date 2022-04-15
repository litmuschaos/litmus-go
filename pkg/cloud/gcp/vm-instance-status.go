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

//InstanceStatusCheckByName is used to check the status of all the VM instances under chaos
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

	if check == "pre-chaos" {
		return InstanceStatusCheckPreChaos(computeService, instanceNamesList, gcpProjectId, instanceZonesList)
	}

	return InstanceStatusCheckPostChaos(computeService, timeout, delay, instanceNamesList, gcpProjectId, instanceZonesList)
}

//InstanceStatusCheckPreChaos is used to check whether all VM instances under chaos are running or not without any re-check
func InstanceStatusCheckPreChaos(computeService *compute.Service, instanceNamesList []string, gcpProjectId string, instanceZonesList []string) error {

	for i := range instanceNamesList {

		instanceState, err := GetVMInstanceStatus(computeService, instanceNamesList[i], gcpProjectId, instanceZonesList[i])
		if err != nil {
			return err
		}

		if instanceState != "RUNNING" {
			return errors.Errorf("%s vm instance is not in RUNNING state, current state: %v", instanceNamesList[i], instanceState)
		}
	}

	return nil
}

//InstanceStatusCheckPostChaos is used to check whether all VM instances under chaos are running or not with re-check
func InstanceStatusCheckPostChaos(computeService *compute.Service, timeout, delay int, instanceNamesList []string, gcpProjectId string, instanceZonesList []string) error {

	for i := range instanceNamesList {
		return WaitForVMInstanceUp(computeService, timeout, delay, instanceNamesList[i], gcpProjectId, instanceZonesList[i])
	}

	return nil
}
