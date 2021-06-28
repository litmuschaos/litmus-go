package azure

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/compute/mgmt/compute"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/pkg/errors"
)

func DetachDisk(subscriptionID, resourceGroup, azureInstanceName, diskName string) error {

	vmClient := compute.NewVirtualMachinesClient(subscriptionID)

	authorizer, err := auth.NewAuthorizerFromFile(azure.PublicCloud.ResourceManagerEndpoint)

	if err == nil {
		vmClient.Authorizer = authorizer
	} else {
		return errors.Errorf("fail to setup authorization, err: %v", err)
	}
	vm, err := vmClient.Get(context.TODO(), resourceGroup, azureInstanceName, compute.InstanceViewTypes("instanceView"))
	if err != nil {
		return errors.Errorf("fail get instance, err: %v", err)
	}

	// Create list of Disks that are not to be detached
	var keepAttachedList []compute.DataDisk

	for _, disk := range *vm.VirtualMachineProperties.StorageProfile.DataDisks {
		if *disk.Name != diskName {
			keepAttachedList = append(keepAttachedList, disk)
		}
	}
	// Detach
	vm.VirtualMachineProperties.StorageProfile.DataDisks = &keepAttachedList
	future, err := vmClient.CreateOrUpdate(context.Background(), resourceGroup, azureInstanceName, vm)
	if err != nil {
		return errors.Errorf("cannot detach disk, err: %v", err)
	}

	err = future.WaitForCompletionRef(context.TODO(), vmClient.Client)
	if err != nil {
		fmt.Printf("cannot get the vm create or update future response, err: %v", err)
	}

	return nil
}

// DetachDiskMultiple will detach the list of disk provided for the specific VM instance
func DetachDiskMultiple(subscriptionID, resourceGroup, azureInstanceName string, diskNameList []string) error {

	vmClient := compute.NewVirtualMachinesClient(subscriptionID)

	authorizer, err := auth.NewAuthorizerFromFile(azure.PublicCloud.ResourceManagerEndpoint)

	if err == nil {
		vmClient.Authorizer = authorizer
	} else {
		return errors.Errorf("fail to setup authorization, err: %v", err)
	}
	vm, err := vmClient.Get(context.TODO(), resourceGroup, azureInstanceName, compute.InstanceViewTypes("instanceView"))
	if err != nil {
		return errors.Errorf("fail get instance, err: %v", err)
	}

	// Create list of Disks that are not to be detached
	var keepAttachedList []compute.DataDisk

	for _, disk := range *vm.VirtualMachineProperties.StorageProfile.DataDisks {
		if !stringInSlice(*disk.Name, diskNameList) {
			keepAttachedList = append(keepAttachedList, disk)
		}
	}
	// Detach
	vm.VirtualMachineProperties.StorageProfile.DataDisks = &keepAttachedList
	future, err := vmClient.CreateOrUpdate(context.Background(), resourceGroup, azureInstanceName, vm)
	if err != nil {
		return errors.Errorf("cannot detach disk, err: %v", err)
	}

	err = future.WaitForCompletionRef(context.TODO(), vmClient.Client)
	if err != nil {
		fmt.Printf("cannot get the vm create or update future response, err: %v", err)
	}

	return nil
}

func AttachDisk(subscriptionID, resourceGroup, azureInstanceName string, diskList *[]compute.DataDisk) error {
	vmClient := compute.NewVirtualMachinesClient(subscriptionID)

	authorizer, err := auth.NewAuthorizerFromFile(azure.PublicCloud.ResourceManagerEndpoint)

	if err == nil {
		vmClient.Authorizer = authorizer
	} else {
		return errors.Errorf("fail to setup authorization, err: %v", err)
	}
	vm, err := vmClient.Get(context.TODO(), resourceGroup, azureInstanceName, compute.InstanceViewTypes("instanceView"))
	if err != nil {
		return errors.Errorf("fail get instance, err: %v", err)
	}

	// Attach
	vm.VirtualMachineProperties.StorageProfile.DataDisks = diskList

	future, err := vmClient.CreateOrUpdate(context.Background(), resourceGroup, azureInstanceName, vm)
	if err != nil {
		fmt.Printf("cannot attach disk, err: %v", err)
	}

	err = future.WaitForCompletionRef(context.Background(), vmClient.Client)
	if err != nil {
		fmt.Printf("cannot get the vm create or update future response, err: %v", err)
	}

	return nil
}

// GetInstanceDiskList will fetch the disk attached to an instance
func GetInstanceDiskList(subscriptionID, resourceGroup, azureInstanceName string) (*[]compute.DataDisk, error) {
	vmClient := compute.NewVirtualMachinesClient(subscriptionID)

	authorizer, err := auth.NewAuthorizerFromFile(azure.PublicCloud.ResourceManagerEndpoint)

	if err == nil {
		vmClient.Authorizer = authorizer
	} else {
		return nil, errors.Errorf("fail to setup authorization, err: %v", err)
	}
	vm, err := vmClient.Get(context.TODO(), resourceGroup, azureInstanceName, compute.InstanceViewTypes("instanceView"))
	if err != nil {
		return nil, errors.Errorf("fail get instance, err: %v", err)
	}

	list := vm.VirtualMachineProperties.StorageProfile.DataDisks

	return list, nil
}

func GetDiskStatus(subscriptionID, resourceGroup, diskName string) (compute.DiskState, error) {
	diskClient := compute.NewDisksClient(subscriptionID)
	authorizer, err := auth.NewAuthorizerFromFile(azure.PublicCloud.ResourceManagerEndpoint)

	if err == nil {
		diskClient.Authorizer = authorizer
	} else {
		return "", errors.Errorf("fail to setup authorization, err: %v", err)
	}
	disk, err := diskClient.Get(context.TODO(), resourceGroup, diskName)
	if err != nil {
		return "", errors.Errorf("failed to get disk, err:%v", err)
	}
	return disk.DiskProperties.DiskState, nil
}

// stringInSlice will check and return whether a string is present inside a slice or not
func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}
