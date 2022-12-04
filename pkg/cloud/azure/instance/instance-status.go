package azure

import (
	"context"
	"strings"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/compute/mgmt/compute"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/litmuschaos/litmus-go/pkg/cloud/azure/common"

	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/pkg/errors"
)

//GetAzureInstanceStatus will verify the azure instance state details
func GetAzureInstanceStatus(subscriptionID, resourceGroup, azureInstanceName string) (string, error) {

	vmClient := compute.NewVirtualMachinesClient(subscriptionID)

	authorizer, err := auth.NewAuthorizerFromFile(azure.PublicCloud.ResourceManagerEndpoint)
	if err != nil {
		return "", errors.Errorf("fail to setup authorization, err: %v", err)
	}

	vmClient.Authorizer = authorizer

	instanceDetails, err := vmClient.InstanceView(context.TODO(), resourceGroup, azureInstanceName)
	if err != nil {
		return "", errors.Errorf("fail to get the instance to check status, err: %v", err)
	}
	// The *instanceDetails.Statuses list contains the instance status details as shown below
	// Item 1: Provisioning succeeded
	// Item 2: VM running
	if len(*instanceDetails.Statuses) < 2 {
		return "", errors.Errorf("fail to get the instatus vm status")
	}

	// To print VM status
	log.Infof("[Status]: The instance %v state is: '%s'", azureInstanceName, *(*instanceDetails.Statuses)[1].DisplayStatus)
	return *(*instanceDetails.Statuses)[1].DisplayStatus, nil
}

//GetAzureScaleSetInstanceStatus will verify the azure instance state details in the scale set
func GetAzureScaleSetInstanceStatus(subscriptionID, resourceGroup, virtualMachineScaleSetName, virtualMachineId string) (string, error) {

	vmssClient := compute.NewVirtualMachineScaleSetVMsClient(subscriptionID)

	authorizer, err := auth.NewAuthorizerFromFile(azure.PublicCloud.ResourceManagerEndpoint)
	if err != nil {
		return "", errors.Errorf("fail to setup authorization, err: %v", err)
	}

	vmssClient.Authorizer = authorizer

	instanceDetails, err := vmssClient.GetInstanceView(context.TODO(), resourceGroup, virtualMachineScaleSetName, virtualMachineId)
	if err != nil {
		return "", errors.Errorf("fail to get the instance to check status, err: %v", err)
	}
	// The *instanceDetails.Statuses list contains the instance status details as shown below
	// Item 1: Provisioning succeeded
	// Item 2: VM running
	if len(*instanceDetails.Statuses) < 2 {
		return "", errors.Errorf("fail to get the instatus vm status")
	}

	// To print VM status
	log.Infof("[Status]: The instance %v_%v state is: '%s'", virtualMachineScaleSetName, virtualMachineId, *(*instanceDetails.Statuses)[1].DisplayStatus)
	return *(*instanceDetails.Statuses)[1].DisplayStatus, nil
}

// InstanceStatusCheckByName is used to check the instance status of all the instance under chaos
func InstanceStatusCheckByName(azureInstanceNames, scaleSet, subscriptionID, resourceGroup string) error {
	if azureInstanceNames == "" {
		return errors.Errorf("no instance found to check the status")
	}
	instanceNameList := strings.Split(strings.TrimSpace(azureInstanceNames), ",")
	log.Infof("[Info]: The instance under chaos(IUC) are: %v", instanceNameList)
	switch scaleSet {
	case "enable":
		return ScaleSetInstanceStatusCheck(instanceNameList, subscriptionID, resourceGroup)
	default:
		return InstanceStatusCheck(instanceNameList, subscriptionID, resourceGroup)
	}
}

// InstanceStatusCheck is used to check the instance status of given list of instances
func InstanceStatusCheck(targetInstanceNameList []string, subscriptionID, resourceGroup string) error {

	for _, vmName := range targetInstanceNameList {
		instanceState, err := GetAzureInstanceStatus(subscriptionID, resourceGroup, vmName)
		if err != nil {
			return err
		}
		if instanceState != "VM running" {
			return errors.Errorf("failed to get the azure instance '%v' in running state, current state: %v", vmName, instanceState)
		}
	}
	return nil
}

//GetAzureInstanceProvisionStatus will check for the azure instance provision state details
func GetAzureInstanceProvisionStatus(subscriptionID, resourceGroup, azureInstanceName, scaleSet string) (string, error) {

	if scaleSet == "enable" {
		vmssClient := compute.NewVirtualMachineScaleSetVMsClient(subscriptionID)
		authorizer, err := auth.NewAuthorizerFromFile(azure.PublicCloud.ResourceManagerEndpoint)
		if err != nil {
			return "", errors.Errorf("fail to setup authorization, err: %v", err)
		}
		vmssClient.Authorizer = authorizer
		scaleSetName, vmId := common.GetScaleSetNameAndInstanceId(azureInstanceName)
		vm, err := vmssClient.Get(context.TODO(), resourceGroup, scaleSetName, vmId, "instanceView")
		if err != nil {
			return "", errors.Errorf("fail to get the instance to check status, err: %v", err)
		}
		instanceDetails := vm.VirtualMachineScaleSetVMProperties.InstanceView
		// To print VM provision status
		log.Infof("[Status]: The instance %v provision state is: '%s'", azureInstanceName, *(*instanceDetails.Statuses)[0].DisplayStatus)
		return *(*instanceDetails.Statuses)[0].DisplayStatus, nil
	}

	vmClient := compute.NewVirtualMachinesClient(subscriptionID)

	authorizer, err := auth.NewAuthorizerFromFile(azure.PublicCloud.ResourceManagerEndpoint)
	if err != nil {
		return "", errors.Errorf("fail to setup authorization, err: %v", err)
	}
	vmClient.Authorizer = authorizer

	instanceDetails, err := vmClient.InstanceView(context.TODO(), resourceGroup, azureInstanceName)
	if err != nil {
		return "", errors.Errorf("fail to get the instance to check status, err: %v", err)
	}
	// To print VM provision status
	log.Infof("[Status]: The instance %v provision state is: '%s'", azureInstanceName, *(*instanceDetails.Statuses)[0].DisplayStatus)
	return *(*instanceDetails.Statuses)[0].DisplayStatus, nil

}

// ScaleSetInstanceStatusCheck is used to check the instance status of given list of instances belonging to scale set
func ScaleSetInstanceStatusCheck(targetInstanceNameList []string, subscriptionID, resourceGroup string) error {

	for _, instanceName := range targetInstanceNameList {
		scaleSet, vm := common.GetScaleSetNameAndInstanceId(instanceName)
		instanceState, err := GetAzureScaleSetInstanceStatus(subscriptionID, resourceGroup, scaleSet, vm)
		if err != nil {
			return err
		}
		if instanceState != "VM running" {
			return errors.Errorf("failed to get the azure instance '%v_%v' in running state, current state: %v", scaleSet, vm, instanceState)
		}
	}
	return nil
}
