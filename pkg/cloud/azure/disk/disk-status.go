package azure

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/compute/mgmt/compute"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/litmuschaos/litmus-go/pkg/cloud/azure/common"
)

// GetInstanceDiskList will fetch the disks attached to an instance
func GetInstanceDiskList(subscriptionID, resourceGroup, scaleSet, azureInstanceName string) (*[]compute.DataDisk, error) {

	authorizer, err := auth.NewAuthorizerFromFile(azure.PublicCloud.ResourceManagerEndpoint)
	if err != nil {
		return nil, cerrors.Error{
			ErrorCode: cerrors.ErrorTypeGeneric,
			Reason:    fmt.Sprintf("authorization set up failed: %v", err),
			Target:    fmt.Sprintf("{Azure Instance Name: %v, Resource Group: %v}", azureInstanceName, resourceGroup),
		}
	}

	// if the instance is of virtual machine scale set (aks node)
	if scaleSet == "enable" {
		vmClient := compute.NewVirtualMachineScaleSetVMsClient(subscriptionID)

		vmClient.Authorizer = authorizer

		// Fetch the vm instance
		scaleSetName, vmId := common.GetScaleSetNameAndInstanceId(azureInstanceName)
		vm, err := vmClient.Get(context.TODO(), resourceGroup, scaleSetName, vmId, compute.InstanceViewTypes("instanceView"))
		if err != nil {
			return nil, cerrors.Error{
				ErrorCode: cerrors.ErrorTypeTargetSelection,
				Reason:    fmt.Sprintf("failed to get instance: %v", err),
				Target:    fmt.Sprintf("{Azure Instance Name: %v, Resource Group: %v}", azureInstanceName, resourceGroup),
			}
		}

		// Get the disks attached to the instance
		list := vm.VirtualMachineScaleSetVMProperties.StorageProfile.DataDisks
		return list, nil
	} else {
		// Setup and authorize vm client
		vmClient := compute.NewVirtualMachinesClient(subscriptionID)

		vmClient.Authorizer = authorizer

		// Fetch the vm instance
		vm, err := vmClient.Get(context.TODO(), resourceGroup, azureInstanceName, compute.InstanceViewTypes("instanceView"))
		if err != nil {
			return nil, cerrors.Error{
				ErrorCode: cerrors.ErrorTypeTargetSelection,
				Reason:    fmt.Sprintf("failed to get instance: %v", err),
				Target:    fmt.Sprintf("{Azure Instance Name: %v, Resource Group: %v}", azureInstanceName, resourceGroup),
			}
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
		return "", cerrors.Error{
			ErrorCode: cerrors.ErrorTypeGeneric,
			Reason:    fmt.Sprintf("authorization set up failed: %v", err),
			Target:    fmt.Sprintf("{Azure Disk Name: %v, Resource Group: %v}", diskName, resourceGroup),
		}
	}
	diskClient.Authorizer = authorizer

	// Get the disk status
	disk, err := diskClient.Get(context.TODO(), resourceGroup, diskName)
	if err != nil {
		return "", cerrors.Error{
			ErrorCode: cerrors.ErrorTypeStatusChecks,
			Reason:    fmt.Sprintf("failed to get disk: %v", err),
			Target:    fmt.Sprintf("{Azure Disk Name: %v, Resource Group: %v}", diskName, resourceGroup),
		}
	}
	return disk.DiskProperties.DiskState, nil
}

// CheckVirtualDiskWithInstance checks whether the given list of disk are attached to the provided VM instance
func CheckVirtualDiskWithInstance(subscriptionID, virtualDiskNames, resourceGroup string) error {

	// Setup and authorize disk client
	diskClient := compute.NewDisksClient(subscriptionID)
	authorizer, err := auth.NewAuthorizerFromFile(azure.PublicCloud.ResourceManagerEndpoint)

	if err != nil {
		return cerrors.Error{
			ErrorCode: cerrors.ErrorTypeGeneric,
			Reason:    fmt.Sprintf("authorization set up failed: %v", err),
			Target:    fmt.Sprintf("{Resource Group: %v}", resourceGroup),
		}
	}
	diskClient.Authorizer = authorizer

	// Creating an array of the name of the attached disks
	diskNameList := strings.Split(virtualDiskNames, ",")
	if virtualDiskNames == "" || len(diskNameList) == 0 {
		return cerrors.Error{
			ErrorCode: cerrors.ErrorTypeStatusChecks,
			Reason:    "no disk name provided",
			Target:    fmt.Sprintf("{Resource Group: %v}", resourceGroup),
		}
	}

	for _, diskName := range diskNameList {
		disk, err := diskClient.Get(context.Background(), resourceGroup, diskName)
		if err != nil {
			return cerrors.Error{
				ErrorCode: cerrors.ErrorTypeStatusChecks,
				Reason:    fmt.Sprintf("failed to get disk: %v", err),
				Target:    fmt.Sprintf("{Azure Disk Name: %v, Resource Group: %v}", diskName, resourceGroup)}
		}
		if disk.ManagedBy == nil {
			return cerrors.Error{
				ErrorCode: cerrors.ErrorTypeStatusChecks,
				Reason:    "disk is not attached to any instance",
				Target:    fmt.Sprintf("{Azure Disk Name: %v, Resource Group: %v}", diskName, resourceGroup)}
		}
	}
	return nil
}

// GetInstanceNameForDisks will extract the instance name from the disk properties
func GetInstanceNameForDisks(diskNameList []string, subscriptionID, resourceGroup string) (map[string][]string, error) {

	// Setup and authorize disk client
	diskClient := compute.NewDisksClient(subscriptionID)
	authorizer, err := auth.NewAuthorizerFromFile(azure.PublicCloud.ResourceManagerEndpoint)
	if err != nil {
		return nil, cerrors.Error{
			ErrorCode: cerrors.ErrorTypeTargetSelection,
			Reason:    fmt.Sprintf("authorization set up failed: %v", err),
			Target:    fmt.Sprintf("{Azure Disk Names: %v, Resource Group: %v}", diskNameList, resourceGroup),
		}
	}
	diskClient.Authorizer = authorizer

	// Creating a map to store the instance name with attached disk(s) name
	instanceNameWithDiskMap := make(map[string][]string)

	// Using regex pattern match to extract instance name from disk.ManagedBy
	// /subscriptionID/<subscriptionID>/resourceGroup/<resourceGroup>/providers/Microsoft.Compute/virtualMachines/instanceName
	instanceNameRegex := regexp.MustCompile(`virtualMachines/`)

	for _, diskName := range diskNameList {
		disk, err := diskClient.Get(context.TODO(), resourceGroup, diskName)
		if err != nil {
			return nil, cerrors.Error{
				ErrorCode: cerrors.ErrorTypeTargetSelection,
				Reason:    fmt.Sprintf("failed to get disk: %v", err),
				Target:    fmt.Sprintf("{Azure Disk Name: %v, Resource Group: %v}", diskName, resourceGroup),
			}
		}
		res := instanceNameRegex.FindStringIndex(*disk.ManagedBy)
		i := res[1]
		instanceName := (*disk.ManagedBy)[i:len(*disk.ManagedBy)]
		instanceNameWithDiskMap[instanceName] = append(instanceNameWithDiskMap[instanceName], strings.TrimSpace(*disk.Name))
	}

	return instanceNameWithDiskMap, nil
}
