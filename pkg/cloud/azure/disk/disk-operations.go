package azure

import (
	"context"
	"time"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/compute/mgmt/compute"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/litmuschaos/litmus-go/pkg/azure/disk-loss/types"
	"github.com/litmuschaos/litmus-go/pkg/cloud/azure/common"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/pkg/errors"
)

// DetachDisks will detach the list of disk provided for the specific VM instance
func DetachDisks(subscriptionID, resourceGroup, azureInstanceName string, diskNameList []string) error {

	// Setup and authorize vm client
	vmClient := compute.NewVirtualMachinesClient(subscriptionID)

	authorizer, err := auth.NewAuthorizerFromFile(azure.PublicCloud.ResourceManagerEndpoint)

	if err != nil {
		return errors.Errorf("fail to setup authorization, err: %v", err)
	}
	vmClient.Authorizer = authorizer

	// Fetch the vm instance
	vm, err := vmClient.Get(context.TODO(), resourceGroup, azureInstanceName, compute.InstanceViewTypes("instanceView"))
	if err != nil {
		return errors.Errorf("fail get instance, err: %v", err)
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
		return errors.Errorf("cannot detach disk, err: %v", err)
	}

	return nil
}

// AttachDisk will attach the list of disk provided for the specific VM instance
func AttachDisk(subscriptionID, resourceGroup, azureInstanceName string, diskList *[]compute.DataDisk) error {

	// Setup and authorize vm client
	vmClient := compute.NewVirtualMachinesClient(subscriptionID)

	authorizer, err := auth.NewAuthorizerFromFile(azure.PublicCloud.ResourceManagerEndpoint)

	if err != nil {
		return errors.Errorf("fail to setup authorization, err: %v", err)
	}

	vmClient.Authorizer = authorizer

	// Fetch the vm instance
	vm, err := vmClient.Get(context.TODO(), resourceGroup, azureInstanceName, compute.InstanceViewTypes("instanceView"))
	if err != nil {
		return errors.Errorf("fail get instance, err: %v", err)
	}

	// Attach the disk to VM properties
	vm.VirtualMachineProperties.StorageProfile.DataDisks = diskList

	// Update the VM properties
	_, err = vmClient.CreateOrUpdate(context.TODO(), resourceGroup, azureInstanceName, vm)
	if err != nil {
		return errors.Errorf("cannot attach disk, err: %v", err)
	}

	// Wait for VM update to complete
	// err = future.WaitForCompletionRef(context.TODO(), vmClient.Client)
	// if err != nil {
	// 	return errors.Errorf("cannot get the vm create or update future response, err: %v", err)
	// }

	return nil
}

// WaitForDiskToAttach waits until the disks are attached
func WaitForDiskToAttach(experimentsDetails *types.ExperimentDetails, diskName string) error {
	//Getting the virtual disk status
	retry.
		Times(uint(experimentsDetails.Timeout / experimentsDetails.Delay)).
		Wait(time.Duration(experimentsDetails.Delay) * time.Second).
		Try(func(attempt uint) error {
			diskState, err := GetDiskStatus(experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, diskName)
			if err != nil {
				errors.Errorf("failed to get the disk status, err: %v", err)
			}
			if diskState != "Attached" {
				log.Infof("[Status]: Disk %v is not yet attached, state: %v", diskName, diskState)
				return errors.Errorf("Disk is not yet attached, state: %v", diskState)
			}
			log.Infof("Disk %v attached", diskName)
			return nil
		})
	return nil
}

// WaitForDiskToDetach waits until the disks are detached
func WaitForDiskToDetach(experimentsDetails *types.ExperimentDetails, diskName string) error {
	//Getting the virtual disk status
	retry.
		Times(uint(experimentsDetails.Timeout / experimentsDetails.Delay)).
		Wait(time.Duration(experimentsDetails.Delay) * time.Second).
		Try(func(attempt uint) error {
			diskState, err := GetDiskStatus(experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, diskName)
			if err != nil {
				errors.Errorf("failed to get the disk status, err: %v", err)
			}
			if diskState != "Unattached" {
				log.Infof("[Status]: Disk %v is not yet detached, state: %v", diskName, diskState)
				return errors.Errorf("Disk is not yet detached, state: %v", diskState)
			}
			log.Infof("Disk %v detached", diskName)
			return nil
		})
	return nil
}
