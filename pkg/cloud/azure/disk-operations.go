package azure

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"strings"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/compute/mgmt/compute"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/azure/azure-disk-loss/types"
	"github.com/pkg/errors"
)

// DetachDisk will detach the disk from the vm instance
func DetachDisk(subscriptionID, resourceGroup, azureInstanceName, diskName string) error {

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
		if *disk.Name != diskName {
			keepAttachedList = append(keepAttachedList, disk)
		}
	}

	// Update the VM with the keepAttachedList to detach the specified disks
	vm.VirtualMachineProperties.StorageProfile.DataDisks = &keepAttachedList
	future, err := vmClient.CreateOrUpdate(context.TODO(), resourceGroup, azureInstanceName, vm)
	if err != nil {
		return errors.Errorf("cannot detach disk, err: %v", err)
	}

	// Wait for VM update to complete
	err = future.WaitForCompletionRef(context.TODO(), vmClient.Client)
	if err != nil {
		return errors.Errorf("cannot get the vm create or update future response, err: %v", err)
	}

	return nil
}

// DetachMultipleDisks will detach the list of disk provided for the specific VM instance
func DetachMultipleDisks(subscriptionID, resourceGroup, azureInstanceName string, diskNameList []string) error {

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
		if !stringInSlice(*disk.Name, diskNameList) {
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
	future, err := vmClient.CreateOrUpdate(context.TODO(), resourceGroup, azureInstanceName, vm)
	if err != nil {
		return errors.Errorf("cannot detach disk, err: %v", err)
	}

	// Wait for VM update to complete
	err = future.WaitForCompletionRef(context.TODO(), vmClient.Client)
	if err != nil {
		return errors.Errorf("cannot get the vm create or update future response, err: %v", err)
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
	future, err := vmClient.CreateOrUpdate(context.TODO(), resourceGroup, azureInstanceName, vm)
	if err != nil {
		return errors.Errorf("cannot attach disk, err: %v", err)
	}

	// Wait for VM update to complete
	err = future.WaitForCompletionRef(context.TODO(), vmClient.Client)
	if err != nil {
		return errors.Errorf("cannot get the vm create or update future response, err: %v", err)
	}

	return nil
}

// GetInstanceDiskList will fetch the disk attached to an instance
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
		if !stringInSlice(diskName, diskListInstance) {
			return errors.Errorf("'%v' is not attached to vm '%v' instance", diskName, experimentsDetails.AzureInstanceName)
		}
	}
	return nil
}

// SetupSubsciptionID fetch the subscription id from the auth file and export it in experiment struct variable
func SetupSubscriptionID(experimentsDetails *experimentTypes.ExperimentDetails) error {

	authFile, err := os.Open(os.Getenv("AZURE_AUTH_LOCATION"))
	if err != nil {
		return errors.Errorf("fail to open auth file, err: %v", err)
	}

	authFileContent, err := ioutil.ReadAll(authFile)
	if err != nil {
		return errors.Errorf("fail to read auth file, err: %v", err)
	}

	details := make(map[string]string)
	if err := json.Unmarshal(authFileContent, &details); err != nil {
		return errors.Errorf("fail to unmarshal file, err: %v", err)
	}

	if id, contains := details["subscriptionId"]; contains {
		experimentsDetails.SubscriptionID = id
	} else {
		return errors.Errorf("The auth file does not have a subscriptionId field")
	}
	return nil
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
