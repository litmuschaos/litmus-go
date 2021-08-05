package azure

import (
	"context"
	"regexp"
	"strings"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/compute/mgmt/compute"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/litmuschaos/litmus-go/pkg/cloud/azure/common"

	experimentTypes "github.com/litmuschaos/litmus-go/pkg/azure/disk-loss/types"
	"github.com/pkg/errors"
)

// GetInstanceDiskList will fetch the disks attached to an instance
func GetInstanceDiskList(subscriptionID, resourceGroup, scaleSet, azureInstanceName string) (*[]compute.DataDisk, error) {

	// if the instance is of virtual machine scale set (aks node)
	if scaleSet == "enable" {
		vmClient := compute.NewVirtualMachineScaleSetVMsClient(subscriptionID)
		authorizer, err := auth.NewAuthorizerFromFile(azure.PublicCloud.ResourceManagerEndpoint)

		if err != nil {
			return nil, errors.Errorf("fail to setup authorization, err: %v", err)
		}
		vmClient.Authorizer = authorizer

		// Fetch the vm instance
		scaleSetName, vmId := common.GetScaleSetNameAndInstanceId(azureInstanceName)
		vm, err := vmClient.Get(context.TODO(), resourceGroup, scaleSetName, vmId, compute.InstanceViewTypes("instanceView"))
		if err != nil {
			return nil, errors.Errorf("fail get instance, err: %v", err)
		}

		// Get the disks attached to the instance
		list := vm.VirtualMachineScaleSetVMProperties.StorageProfile.DataDisks
		return list, nil
	} else {
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

	// Setup and authorize disk client
	diskClient := compute.NewDisksClient(experimentsDetails.SubscriptionID)
	authorizer, err := auth.NewAuthorizerFromFile(azure.PublicCloud.ResourceManagerEndpoint)

	if err != nil {
		return errors.Errorf("fail to setup authorization, err: %v", err)
	}
	diskClient.Authorizer = authorizer

	// Creating an array of the name of the attached disks
	diskNameList := strings.Split(experimentsDetails.VirtualDiskNames, ",")

	for _, diskName := range diskNameList {
		disk, err := diskClient.Get(context.Background(), experimentsDetails.ResourceGroup, diskName)
		if err != nil {
			return errors.Errorf("failed to get disk: %v, err: %v", diskName, err)
		}
		if disk.ManagedBy == nil {
			return errors.Errorf("disk %v not attached to any instance", diskName)
		}
	}
	return nil
}

// GetInstanceNameForDisks will extract the instance name from the disk properties
func GetInstanceNameForDisks(diskNameList []string, subscriptionID, resourceGroup string) (map[string][]string, error) {

	// Setup and authorize disk client
	diskClient := compute.NewDisksClient(subscriptionID)
	authorizer, err := auth.NewAuthorizerFromFile(azure.PublicCloud.ResourceManagerEndpoint)

	// Creating a map to store the instance name with attached disk(s) name
	instanceNameWithDiskMap := make(map[string][]string)

	if err != nil {
		return instanceNameWithDiskMap, errors.Errorf("fail to setup authorization, err: %v", err)
	}
	diskClient.Authorizer = authorizer

	// Using regex pattern match to extract instance name from disk.ManagedBy
	// /subscriptionID/<subscriptionID>/resourceGroup/<resourceGroup>/providers/Microsoft.Compute/virtualMachines/instanceName
	instanceNameRegex := regexp.MustCompile(`virtualMachines/`)

	for _, diskName := range diskNameList {
		disk, err := diskClient.Get(context.TODO(), resourceGroup, diskName)
		if err != nil {
			return instanceNameWithDiskMap, nil
		}
		res := instanceNameRegex.FindStringIndex(*disk.ManagedBy)
		i := res[1]
		instanceName := (*disk.ManagedBy)[i:len(*disk.ManagedBy)]
		instanceNameWithDiskMap[instanceName] = append(instanceNameWithDiskMap[instanceName], strings.TrimSpace(*disk.Name))
	}

	return instanceNameWithDiskMap, nil
}
