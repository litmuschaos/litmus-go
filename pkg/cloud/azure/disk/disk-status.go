package azure

import (
	"context"
	"strings"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/compute/mgmt/compute"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/azure/disk-loss/types"
	"github.com/litmuschaos/litmus-go/pkg/cloud/azure/common"
	"github.com/pkg/errors"
)

// GetInstanceDiskList will fetch the disks attached to an instance
func GetInstanceDiskList(subscriptionID, resourceGroup, azureInstanceName string) (*[]compute.DataDisk, error) {

	// Setup and authorize vm client
	vmClient := compute.NewVirtualMachinesClient(subscriptionID)

	authorizer, err := auth.NewAuthorizerFromFile(azure.PublicCloud.ResourceManagerEndpoint)

	if err != nil {
		return nil, errors.Errorf("fail to setup authorization, err: %v", err)
	}
	vmClient.Authorizer = authorizer

	// Fetch the vm instance
	vm, err := vmClient.Get(context.TODO(), resourceGroup, azureInstanceName, compute.InstanceViewTypes("instanceView"))
	if err != nil {
		return nil, errors.Errorf("fail get instance, err: %v", err)
	}

	// Get the disks attached to the instance
	list := vm.VirtualMachineProperties.StorageProfile.DataDisks

	return list, nil
}

// GetDiskStatus will get the status of disk (attached/unattached)
func GetDiskStatus(subscriptionID, resourceGroup, diskName string) (compute.DiskState, error) {

	// Setup and authorize disk client
	diskClient := compute.NewDisksClient(subscriptionID)
	authorizer, err := auth.NewAuthorizerFromFile(azure.PublicCloud.ResourceManagerEndpoint)

	if err != nil {
		return "", errors.Errorf("fail to setup authorization, err: %v", err)
	}
	diskClient.Authorizer = authorizer

	// Get the disk status
	disk, err := diskClient.Get(context.TODO(), resourceGroup, diskName)
	if err != nil {
		return "", errors.Errorf("failed to get disk, err:%v", err)
	}
	return disk.DiskProperties.DiskState, nil
}

// CheckVirtualDiskWithInstance checks whether the given list of disk are attached to the provided VM instance
func CheckVirtualDiskWithInstance(experimentsDetails experimentTypes.ExperimentDetails) error {

	// Get the attached disks with the instance
	diskList, err := GetInstanceDiskList(experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup, experimentsDetails.AzureInstanceName)
	if err != nil {
		return errors.Errorf("failed to get disk status, err: %v", err)
	}

	// Creating an array of the name of the attached disks
	diskNameList := strings.Split(experimentsDetails.VirtualDiskNames, ",")
	var diskListInstance []string
	for _, disk := range *diskList {
		diskListInstance = append(diskListInstance, *disk.Name)
	}
	// Checking whether the provided disk are attached to the instance
	for _, diskName := range diskNameList {
		if !common.StringInSlice(diskName, diskListInstance) {
			return errors.Errorf("'%v' is not attached to vm '%v' instance", diskName, experimentsDetails.AzureInstanceName)
		}
	}
	return nil
}
