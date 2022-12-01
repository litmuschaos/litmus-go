package azure

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/compute/mgmt/compute"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/litmuschaos/litmus-go/pkg/cloud/azure/common"
	"github.com/palantir/stacktrace"

	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
)

// AzureInstanceStop stops the target instance
func AzureInstanceStop(timeout, delay int, subscriptionID, resourceGroup, azureInstanceName string) error {
	vmClient := compute.NewVirtualMachinesClient(subscriptionID)

	authorizer, err := auth.NewAuthorizerFromFile(azure.PublicCloud.ResourceManagerEndpoint)
	if err != nil {
		return cerrors.Error{
			ErrorCode: cerrors.ErrorTypeChaosInject,
			Reason:    fmt.Sprintf("authorization set up failed: %v", err),
			Target:    fmt.Sprintf("{Azure Instance Name: %v, Resource Group: %v}", azureInstanceName, resourceGroup),
		}
	}

	vmClient.Authorizer = authorizer

	log.Info("[Info]: Stopping the instance")
	_, err = vmClient.PowerOff(context.TODO(), resourceGroup, azureInstanceName, &vmClient.SkipResourceProviderRegistration)
	if err != nil {
		return cerrors.Error{
			ErrorCode: cerrors.ErrorTypeChaosInject,
			Reason:    fmt.Sprintf("failed to stop the instance: %v", err),
			Target:    fmt.Sprintf("{Azure Instance Name: %v, Resource Group: %v}", azureInstanceName, resourceGroup),
		}
	}

	return nil
}

// AzureInstanceStart starts the target instance
func AzureInstanceStart(timeout, delay int, subscriptionID, resourceGroup, azureInstanceName string) error {

	vmClient := compute.NewVirtualMachinesClient(subscriptionID)

	authorizer, err := auth.NewAuthorizerFromFile(azure.PublicCloud.ResourceManagerEndpoint)
	if err != nil {
		return cerrors.Error{
			ErrorCode: cerrors.ErrorTypeChaosRevert,
			Reason:    fmt.Sprintf("authorization set up failed: %v", err),
			Target:    fmt.Sprintf("{Azure Instance Name: %v, Resource Group: %v}", azureInstanceName, resourceGroup),
		}
	}

	vmClient.Authorizer = authorizer

	log.Info("[Info]: Starting back the instance to running state")
	_, err = vmClient.Start(context.TODO(), resourceGroup, azureInstanceName)
	if err != nil {
		return cerrors.Error{
			ErrorCode: cerrors.ErrorTypeChaosRevert,
			Reason:    fmt.Sprintf("failed to start the instance: %v", err),
			Target:    fmt.Sprintf("{Azure Instance Name: %v, Resource Group: %v}", azureInstanceName, resourceGroup),
		}
	}

	return nil
}

// AzureScaleSetInstanceStop stops the target instance in the scale set
func AzureScaleSetInstanceStop(timeout, delay int, subscriptionID, resourceGroup, azureInstanceName string) error {
	vmssClient := compute.NewVirtualMachineScaleSetVMsClient(subscriptionID)

	authorizer, err := auth.NewAuthorizerFromFile(azure.PublicCloud.ResourceManagerEndpoint)
	if err != nil {
		return cerrors.Error{
			ErrorCode: cerrors.ErrorTypeChaosInject,
			Reason:    fmt.Sprintf("authorization set up failed: %v", err),
			Target:    fmt.Sprintf("{Azure Instance Name: %v, Resource Group: %v}", azureInstanceName, resourceGroup),
		}
	}

	vmssClient.Authorizer = authorizer

	virtualMachineScaleSetName, virtualMachineId := common.GetScaleSetNameAndInstanceId(azureInstanceName)

	log.Info("[Info]: Stopping the instance")
	_, err = vmssClient.PowerOff(context.TODO(), resourceGroup, virtualMachineScaleSetName, virtualMachineId, &vmssClient.SkipResourceProviderRegistration)
	if err != nil {
		return cerrors.Error{
			ErrorCode: cerrors.ErrorTypeChaosInject,
			Reason:    fmt.Sprintf("failed to stop the instance: %v", err),
			Target:    fmt.Sprintf("{Azure Instance Name: %v, Resource Group: %v}", azureInstanceName, resourceGroup),
		}
	}

	return nil
}

// AzureScaleSetInstanceStart starts the target instance in the scale set
func AzureScaleSetInstanceStart(timeout, delay int, subscriptionID, resourceGroup, azureInstanceName string) error {
	vmssClient := compute.NewVirtualMachineScaleSetVMsClient(subscriptionID)

	authorizer, err := auth.NewAuthorizerFromFile(azure.PublicCloud.ResourceManagerEndpoint)
	if err != nil {
		return cerrors.Error{
			ErrorCode: cerrors.ErrorTypeChaosRevert,
			Reason:    fmt.Sprintf("authorization set up failed: %v", err),
			Target:    fmt.Sprintf("{Azure Instance Name: %v, Resource Group: %v}", azureInstanceName, resourceGroup)}
	}

	vmssClient.Authorizer = authorizer

	virtualMachineScaleSetName, virtualMachineId := common.GetScaleSetNameAndInstanceId(azureInstanceName)

	log.Info("[Info]: Starting back the instance to running state")
	_, err = vmssClient.Start(context.TODO(), resourceGroup, virtualMachineScaleSetName, virtualMachineId)
	if err != nil {
		return cerrors.Error{
			ErrorCode: cerrors.ErrorTypeChaosRevert,
			Reason:    fmt.Sprintf("failed to start the instance: %v", err),
			Target:    fmt.Sprintf("{Azure Instance Name: %v, Resource Group: %v}", azureInstanceName, resourceGroup),
		}
	}

	return nil
}

// WaitForAzureComputeDown will wait for the azure compute instance to get in stopped state
func WaitForAzureComputeDown(timeout, delay int, scaleSet, subscriptionID, resourceGroup, azureInstanceName string) error {

	var instanceState string
	var err error

	log.Info("[Status]: Checking Azure instance status")
	return retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {
			if scaleSet == "enable" {
				scaleSetName, vmId := common.GetScaleSetNameAndInstanceId(azureInstanceName)
				instanceState, err = GetAzureScaleSetInstanceStatus(subscriptionID, resourceGroup, scaleSetName, vmId)
			} else {
				instanceState, err = GetAzureInstanceStatus(subscriptionID, resourceGroup, azureInstanceName)
			}
			if err != nil {
				return stacktrace.Propagate(err, "failed to get the instance status")
			}
			if instanceState != "VM stopped" {
				return cerrors.Error{
					ErrorCode: cerrors.ErrorTypeChaosInject,
					Reason:    "instance is not in stopped within timeout",
					Target:    fmt.Sprintf("{Azure Instance Name: %v, Resource Group: %v}", azureInstanceName, resourceGroup),
				}
			}
			return nil
		})
}

// WaitForAzureComputeUp will wait for the azure compute instance to get in running state
func WaitForAzureComputeUp(timeout, delay int, scaleSet, subscriptionID, resourceGroup, azureInstanceName string) error {

	var instanceState string
	var err error

	log.Info("[Status]: Checking Azure instance status")
	return retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {

			switch scaleSet {
			case "enable":
				scaleSetName, vmId := common.GetScaleSetNameAndInstanceId(azureInstanceName)
				instanceState, err = GetAzureScaleSetInstanceStatus(subscriptionID, resourceGroup, scaleSetName, vmId)
			default:
				instanceState, err = GetAzureInstanceStatus(subscriptionID, resourceGroup, azureInstanceName)
			}
			if err != nil {
				return stacktrace.Propagate(err, "failed to get instance status")
			}
			if instanceState != "VM running" {
				return cerrors.Error{
					ErrorCode: cerrors.ErrorTypeChaosRevert,
					Reason:    "instance is not in running state within timeout",
					Target:    fmt.Sprintf("{Azure Instance Name: %v, Resource Group: %v}", azureInstanceName, resourceGroup),
				}
			}
			return nil
		})
}
