package azure

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/compute/mgmt/compute"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/litmuschaos/litmus-go/pkg/azure/disk-loss/types"
	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/litmuschaos/litmus-go/pkg/cloud/azure/common"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/palantir/stacktrace"
)

// DetachDisks will detach the list of disk provided for the specific VM instance or scale set vm instance
func DetachDisks(subscriptionID, resourceGroup, azureInstanceName, scaleSet string, diskNameList []string) error {

	authorizer, err := auth.NewAuthorizerFromFile(azure.PublicCloud.ResourceManagerEndpoint)
	if err != nil {
		return cerrors.Error{
			ErrorCode: cerrors.ErrorTypeChaosInject,
			Reason:    fmt.Sprintf("authorization set up failed: %v", err),
			Target:    fmt.Sprintf("{Azure Instance Name: %v, Resource Group: %v}", azureInstanceName, resourceGroup),
		}
	}

	// if the instance is virtual machine scale set (aks node)
	if scaleSet == "enable" {
		// Setup and authorize vm client
		vmssClient := compute.NewVirtualMachineScaleSetVMsClient(subscriptionID)
		vmssClient.Authorizer = authorizer

		// Fetch the vm instance
		scaleSetName, vmId := common.GetScaleSetNameAndInstanceId(azureInstanceName)
		vm, err := vmssClient.Get(context.TODO(), resourceGroup, scaleSetName, vmId, compute.InstanceViewTypes("instanceView"))
		if err != nil {
			return cerrors.Error{
				ErrorCode: cerrors.ErrorTypeChaosInject,
				Reason:    fmt.Sprintf("failed to get instance: %v", err),
				Target:    fmt.Sprintf("{Azure Instance Name: %v, Resource Group: %v}", azureInstanceName, resourceGroup),
			}
		}
		// Create list of Disks that are not to be detached
		var keepAttachedList []compute.DataDisk

		for _, disk := range *vm.VirtualMachineScaleSetVMProperties.StorageProfile.DataDisks {
			if !common.StringInSlice(*disk.Name, diskNameList) {
				keepAttachedList = append(keepAttachedList, disk)
			}
		}

		if len(keepAttachedList) < 1 {
			vm.VirtualMachineScaleSetVMProperties.StorageProfile.DataDisks = &[]compute.DataDisk{}
		} else {
			vm.VirtualMachineScaleSetVMProperties.StorageProfile.DataDisks = &keepAttachedList
		}

		// Setting image reference to nil so that API doesn't update the image
		vm.VirtualMachineScaleSetVMProperties.StorageProfile.ImageReference = nil

		// Update the VM with the keepAttachedList to detach the specified disks
		_, err = vmssClient.Update(context.TODO(), resourceGroup, scaleSetName, vmId, vm)
		if err != nil {
			return cerrors.Error{
				ErrorCode: cerrors.ErrorTypeChaosInject,
				Reason:    fmt.Sprintf("cannot detach disk: %v", err),
				Target:    fmt.Sprintf("{Azure Instance Name: %v, Resource Group: %v}", azureInstanceName, resourceGroup),
			}
		}

	} else {
		// Setup and authorize vm client
		vmClient := compute.NewVirtualMachinesClient(subscriptionID)
		vmClient.Authorizer = authorizer

		// Fetch the vm instance
		vm, err := vmClient.Get(context.TODO(), resourceGroup, azureInstanceName, compute.InstanceViewTypes("instanceView"))
		if err != nil {
			return cerrors.Error{
				ErrorCode: cerrors.ErrorTypeChaosInject,
				Reason:    fmt.Sprintf("failed to get instance: %v", err),
				Target:    fmt.Sprintf("{Azure Instance Name: %v, Resource Group: %v}", azureInstanceName, resourceGroup),
			}
		}
		// Create list of Disks that are not to be detached
		var keepAttachedList []compute.DataDisk

		for _, disk := range *vm.VirtualMachineProperties.StorageProfile.DataDisks {
			if !common.StringInSlice(*disk.Name, diskNameList) {
				keepAttachedList = append(keepAttachedList, disk)
			}
		}

		// Update the VM with the keepAttachedList to detach the specified disks
		if len(keepAttachedList) < 1 {
			vm.VirtualMachineProperties.StorageProfile.DataDisks = &[]compute.DataDisk{}
		} else {
			vm.VirtualMachineProperties.StorageProfile.DataDisks = &keepAttachedList
		}

		// Update the VM with the keepAttachedList to detach the specified disks
		_, err = vmClient.CreateOrUpdate(context.TODO(), resourceGroup, azureInstanceName, vm)
		if err != nil {
			return cerrors.Error{
				ErrorCode: cerrors.ErrorTypeChaosInject,
				Reason:    fmt.Sprintf("cannot detach disk(s): %v", err),
				Target:    fmt.Sprintf("{Azure Instance Name: %v, Resource Group: %v}", azureInstanceName, resourceGroup),
			}
		}
	}
	return nil
}

// AttachDisk will attach the list of disk provided for the specific VM instance
func AttachDisk(subscriptionID, resourceGroup, azureInstanceName, scaleSet string, diskList *[]compute.DataDisk) error {

	authorizer, err := auth.NewAuthorizerFromFile(azure.PublicCloud.ResourceManagerEndpoint)
	if err != nil {
		return cerrors.Error{
			ErrorCode: cerrors.ErrorTypeChaosRevert,
			Reason:    fmt.Sprintf("authorization set up failed: %v", err),
			Target:    fmt.Sprintf("{Azure Instance Name: %v, Resource Group: %v}", azureInstanceName, resourceGroup),
		}
	}

	// if the instance is virtual machine scale set (aks node)
	if scaleSet == "enable" {
		// Setup and authorize vm client
		vmClient := compute.NewVirtualMachineScaleSetVMsClient(subscriptionID)
		vmClient.Authorizer = authorizer

		// Fetch the vm instance
		scaleSetName, vmId := common.GetScaleSetNameAndInstanceId(azureInstanceName)
		vm, err := vmClient.Get(context.TODO(), resourceGroup, scaleSetName, vmId, compute.InstanceViewTypes("instanceView"))
		if err != nil {
			return cerrors.Error{
				ErrorCode: cerrors.ErrorTypeChaosRevert,
				Reason:    fmt.Sprintf("failed to get instance: %v", err),
				Target:    fmt.Sprintf("{Azure Instance Name: %v, Resource Group: %v}", azureInstanceName, resourceGroup),
			}
		}
		vm.VirtualMachineScaleSetVMProperties.StorageProfile.DataDisks = diskList

		// Setting image reference to nil so that API doesn't update the image
		vm.VirtualMachineScaleSetVMProperties.StorageProfile.ImageReference = nil

		// Update the VM properties
		_, err = vmClient.Update(context.TODO(), resourceGroup, scaleSetName, vmId, vm)
		if err != nil {
			return cerrors.Error{
				ErrorCode: cerrors.ErrorTypeChaosRevert,
				Reason:    fmt.Sprintf("cannot attach disk: %v", err),
				Target:    fmt.Sprintf("{Azure Instance Name: %v, Resource Group: %v}", azureInstanceName, resourceGroup)}
		}
	} else {
		// Setup and authorize vm client
		vmClient := compute.NewVirtualMachinesClient(subscriptionID)
		vmClient.Authorizer = authorizer

		// Fetch the vm instance
		vm, err := vmClient.Get(context.TODO(), resourceGroup, azureInstanceName, compute.InstanceViewTypes("instanceView"))
		if err != nil {
			return cerrors.Error{
				ErrorCode: cerrors.ErrorTypeChaosRevert,
				Reason:    fmt.Sprintf("failed to get instance: %v", err),
				Target:    fmt.Sprintf("{Azure Instance Name: %v, Resource Group: %v}", azureInstanceName, resourceGroup)}
		}

		// Attach the disk to VM properties
		vm.VirtualMachineProperties.StorageProfile.DataDisks = diskList

		// Update the VM properties
		_, err = vmClient.CreateOrUpdate(context.TODO(), resourceGroup, azureInstanceName, vm)
		if err != nil {
			return cerrors.Error{
				ErrorCode: cerrors.ErrorTypeChaosRevert,
				Reason:    fmt.Sprintf("cannot attach disk(s): %v", err),
				Target:    fmt.Sprintf("{Azure Instance Name: %v, Resource Group: %v}", azureInstanceName, resourceGroup)}
		}
	}
	return nil
}

// WaitForDiskToAttach waits until the disks are attached
func WaitForDiskToAttach(experimentsDetails *types.ExperimentDetails, diskName string) error {
	return retry.
		Times(uint(experimentsDetails.Timeout / experimentsDetails.Delay)).
		Wait(time.Duration(experimentsDetails.Delay) * time.Second).
		Try(func(attempt uint) error {
			diskState, err := GetDiskStatus(experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, diskName)
			if err != nil {
				return stacktrace.Propagate(err, "failed to get the disk status")
			}
			if diskState != "Attached" {
				log.Infof("[Status]: Disk %v is not yet attached, current state: %v", diskName, diskState)
				return cerrors.Error{
					ErrorCode: cerrors.ErrorTypeChaosRevert,
					Reason:    fmt.Sprintf("Disk is not attached within timeout, disk state: %s", diskState),
					Target:    fmt.Sprintf("{Azure Disk Name: %v, Resource Group: %v}", diskName, experimentsDetails.ResourceGroup),
				}
			}
			log.Infof("[Status]: Disk %v is Attached", diskName)
			return nil
		})
}

// WaitForDiskToDetach waits until the disks are detached
func WaitForDiskToDetach(experimentsDetails *types.ExperimentDetails, diskName string) error {
	return retry.
		Times(uint(experimentsDetails.Timeout / experimentsDetails.Delay)).
		Wait(time.Duration(experimentsDetails.Delay) * time.Second).
		Try(func(attempt uint) error {
			diskState, err := GetDiskStatus(experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, diskName)
			if err != nil {
				return stacktrace.Propagate(err, "failed to get the disk status")
			}
			if diskState != "Unattached" {
				log.Infof("[Status]: Disk %v is not yet detached, state: %v", diskName, diskState)
				return cerrors.Error{
					ErrorCode: cerrors.ErrorTypeChaosInject,
					Reason:    fmt.Sprintf("Disk is not detached within timeout, disk state: %s", diskState),
					Target:    fmt.Sprintf("{Azure Disk Name: %v, Resource Group: %v}", diskName, experimentsDetails.ResourceGroup),
				}
			}
			log.Infof("[Status]: Disk %v is Detached", diskName)
			return nil
		})
}
