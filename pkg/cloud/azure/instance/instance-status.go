package azure

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/compute/mgmt/compute"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/litmuschaos/litmus-go/pkg/cloud/azure/common"
	"github.com/palantir/stacktrace"

	"github.com/litmuschaos/litmus-go/pkg/log"
)

// GetAzureInstanceStatus will verify the azure instance state details
func GetAzureInstanceStatus(subscriptionID, resourceGroup, azureInstanceName string) (string, error) {

	vmClient := compute.NewVirtualMachinesClient(subscriptionID)

	authorizer, err := auth.NewAuthorizerFromFile(azure.PublicCloud.ResourceManagerEndpoint)
	if err != nil {
		return "", cerrors.Error{
			ErrorCode: cerrors.ErrorTypeStatusChecks,
			Reason:    fmt.Sprintf("authorization set up failed: %v", err),
			Target:    fmt.Sprintf("{Azure Instance Name: %v, Resource Group: %v}", azureInstanceName, resourceGroup),
		}
	}

	vmClient.Authorizer = authorizer

	instanceDetails, err := vmClient.InstanceView(context.TODO(), resourceGroup, azureInstanceName)
	if err != nil {
		return "", cerrors.Error{
			ErrorCode: cerrors.ErrorTypeStatusChecks,
			Reason:    fmt.Sprintf("failed to get the instance: %v", err),
			Target:    fmt.Sprintf("{Azure Instance Name: %v, Resource Group: %v}", azureInstanceName, resourceGroup),
		}
	}
	// The *instanceDetails.Statuses list contains the instance status details as shown below
	// Item 1: Provisioning succeeded
	// Item 2: VM running
	if len(*instanceDetails.Statuses) < 2 {
		return "", cerrors.Error{
			ErrorCode: cerrors.ErrorTypeStatusChecks,
			Reason:    "failed to get the instance status",
			Target:    fmt.Sprintf("{Azure Instance Name: %v, Resource Group: %v}", azureInstanceName, resourceGroup),
		}
	}

	// To print VM status
	log.Infof("[Status]: The instance %v state is: '%v'", azureInstanceName, *(*instanceDetails.Statuses)[1].DisplayStatus)
	return *(*instanceDetails.Statuses)[1].DisplayStatus, nil
}

// GetAzureScaleSetInstanceStatus will verify the azure instance state details in the scale set
func GetAzureScaleSetInstanceStatus(subscriptionID, resourceGroup, virtualMachineScaleSetName, virtualMachineId string) (string, error) {

	vmssClient := compute.NewVirtualMachineScaleSetVMsClient(subscriptionID)

	authorizer, err := auth.NewAuthorizerFromFile(azure.PublicCloud.ResourceManagerEndpoint)
	if err != nil {
		return "", cerrors.Error{
			ErrorCode: cerrors.ErrorTypeGeneric,
			Reason:    fmt.Sprintf("authorization set up failed: %v", err),
			Target:    fmt.Sprintf("{Azure Instance Name: %v_%v, Resource Group: %v}", virtualMachineScaleSetName, virtualMachineId, resourceGroup),
		}
	}

	vmssClient.Authorizer = authorizer

	instanceDetails, err := vmssClient.GetInstanceView(context.TODO(), resourceGroup, virtualMachineScaleSetName, virtualMachineId)
	if err != nil {
		return "", cerrors.Error{
			ErrorCode: cerrors.ErrorTypeStatusChecks,
			Reason:    fmt.Sprintf("failed to get the instance: %v", err),
			Target:    fmt.Sprintf("{Azure Instance Name: %v_%v, Resource Group: %v}", virtualMachineScaleSetName, virtualMachineId, resourceGroup),
		}
	}
	// The *instanceDetails.Statuses list contains the instance status details as shown below
	// Item 1: Provisioning succeeded
	// Item 2: VM running
	if len(*instanceDetails.Statuses) < 2 {
		return "", cerrors.Error{
			ErrorCode: cerrors.ErrorTypeStatusChecks,
			Reason:    "failed to get the instance status",
			Target:    fmt.Sprintf("{Azure Instance Name: %v_%v}", virtualMachineScaleSetName, virtualMachineId),
		}
	}

	// To print VM status
	log.Infof("[Status]: The instance %v_%v state is: '%v'", virtualMachineScaleSetName, virtualMachineId, *(*instanceDetails.Statuses)[1].DisplayStatus)
	return *(*instanceDetails.Statuses)[1].DisplayStatus, nil
}

// InstanceStatusCheckByName is used to check the instance status of all the instance under chaos
func InstanceStatusCheckByName(azureInstanceNames, scaleSet, subscriptionID, resourceGroup string) error {
	instanceNameList := strings.Split(azureInstanceNames, ",")
	if azureInstanceNames == "" || len(instanceNameList) == 0 {
		return cerrors.Error{
			ErrorCode: cerrors.ErrorTypeStatusChecks,
			Reason:    "no instance provided",
			Target:    fmt.Sprintf("{Azure Instance Names: %v, Resource Group: %v}", azureInstanceNames, resourceGroup),
		}
	}
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

	for _, azureInstanceName := range targetInstanceNameList {
		instanceState, err := GetAzureInstanceStatus(subscriptionID, resourceGroup, azureInstanceName)
		if err != nil {
			return stacktrace.Propagate(err, "failed to get instance status")
		}
		if instanceState != "VM running" {
			return cerrors.Error{
				ErrorCode: cerrors.ErrorTypeStatusChecks,
				Reason:    fmt.Sprintf("azure instance in not in running state, current state: %v", instanceState),
				Target:    fmt.Sprintf("{Azure Instance Name: %v, Resource Group: %v}", azureInstanceName, resourceGroup),
			}
		}
	}
	return nil
}

// GetAzureInstanceProvisionStatus will check for the azure instance provision state details
func GetAzureInstanceProvisionStatus(subscriptionID, resourceGroup, azureInstanceName, scaleSet string) (string, error) {

	if scaleSet == "enable" {
		vmssClient := compute.NewVirtualMachineScaleSetVMsClient(subscriptionID)
		authorizer, err := auth.NewAuthorizerFromFile(azure.PublicCloud.ResourceManagerEndpoint)
		if err != nil {
			return "", cerrors.Error{
				ErrorCode: cerrors.ErrorTypeGeneric,
				Reason:    fmt.Sprintf("authorization set up failed: %v", err),
				Target:    fmt.Sprintf("{Azure Instance Name: %v, Resource Group: %v}", azureInstanceName, resourceGroup),
			}
		}
		vmssClient.Authorizer = authorizer
		scaleSetName, vmId := common.GetScaleSetNameAndInstanceId(azureInstanceName)
		vm, err := vmssClient.Get(context.TODO(), resourceGroup, scaleSetName, vmId, "instanceView")
		if err != nil {
			return "", cerrors.Error{
				ErrorCode: cerrors.ErrorTypeStatusChecks,
				Reason:    fmt.Sprintf("failed to get the instance: %v", err),
				Target:    fmt.Sprintf("{Azure Instance Name: %v, Resource Group: %v}", azureInstanceName, resourceGroup),
			}
		}
		instanceDetails := vm.VirtualMachineScaleSetVMProperties.InstanceView
		// To print VM provision status
		log.Infof("[Status]: The instance %v provision state is: '%v'", azureInstanceName, *(*instanceDetails.Statuses)[0].DisplayStatus)
		return *(*instanceDetails.Statuses)[0].DisplayStatus, nil
	}

	vmClient := compute.NewVirtualMachinesClient(subscriptionID)

	authorizer, err := auth.NewAuthorizerFromFile(azure.PublicCloud.ResourceManagerEndpoint)
	if err != nil {
		return "", cerrors.Error{
			ErrorCode: cerrors.ErrorTypeGeneric,
			Reason:    fmt.Sprintf("authorization set up failed: %v", err),
			Target:    fmt.Sprintf("{Azure Instance Name: %v, Resource Group: %v}", azureInstanceName, resourceGroup),
		}
	}
	vmClient.Authorizer = authorizer

	instanceDetails, err := vmClient.InstanceView(context.TODO(), resourceGroup, azureInstanceName)
	if err != nil {
		return "", cerrors.Error{
			ErrorCode: cerrors.ErrorTypeStatusChecks,
			Reason:    fmt.Sprintf("failed to get the instance: %v", err),
			Target:    fmt.Sprintf("{Azure Instance Name: %v, Resource Group: %v}", azureInstanceName, resourceGroup),
		}
	}
	// To print VM provision status
	log.Infof("[Status]: The instance %v provision state is: '%v'", azureInstanceName, *(*instanceDetails.Statuses)[0].DisplayStatus)
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
			return cerrors.Error{
				ErrorCode: cerrors.ErrorTypeStatusChecks,
				Reason:    fmt.Sprintf("azure instance is not in running state, current state: %v", instanceState),
				Target:    fmt.Sprintf("{Azure Instance Name: %v_%v, Resource Group: %v}", scaleSet, vm, resourceGroup)}
		}
	}
	return nil
}
