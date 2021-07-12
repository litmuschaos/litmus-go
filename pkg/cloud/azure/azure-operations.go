package azure

import (
	"context"
	"time"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/compute/mgmt/compute"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/pkg/errors"
)

// AzureInstanceStop poweroff the target instance
func AzureInstanceStop(timeout, delay int, subscriptionID, resourceGroup, azureInstanceName string) error {
	vmClient := compute.NewVirtualMachinesClient(subscriptionID)

	authorizer, err := auth.NewAuthorizerFromFile(azure.PublicCloud.ResourceManagerEndpoint)
	if err == nil {
		vmClient.Authorizer = authorizer
	} else {
		return errors.Errorf("fail to setup authorization, err: %v")
	}

	log.Info("[Info]: Starting powerOff the instance")
	_, err = vmClient.PowerOff(context.TODO(), resourceGroup, azureInstanceName, &vmClient.SkipResourceProviderRegistration)
	if err != nil {
		return errors.Errorf("fail to stop the %v instance, err: %v", azureInstanceName, err)
	}

	return nil
}

// AzureInstanceStart starts the target instance
func AzureInstanceStart(timeout, delay int, subscriptionID, resourceGroup, azureInstanceName string) error {
	vmClient := compute.NewVirtualMachinesClient(subscriptionID)

	authorizer, err := auth.NewAuthorizerFromFile(azure.PublicCloud.ResourceManagerEndpoint)
	if err == nil {
		vmClient.Authorizer = authorizer
	} else {
		return errors.Errorf("fail to setup authorization, err: %v")
	}

	log.Info("[Info]: Starting back the instance to running state")
	_, err = vmClient.Start(context.TODO(), resourceGroup, azureInstanceName)
	if err != nil {
		return errors.Errorf("fail to start the %v instance, err: %v", azureInstanceName, err)
	}

	return nil
}

//WaitForAzureComputeDown will wait for the azure compute instance to get in stopped state
func WaitForAzureComputeDown(timeout, delay int, subscriptionID, resourceGroup, azureInstanceName string) error {

	log.Info("[Status]: Checking Azure instance status")
	return retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {

			instanceState, err := GetAzureInstanceStatus(subscriptionID, resourceGroup, azureInstanceName)
			if err != nil {
				return errors.Errorf("failed to get the instance status")
			}
			if instanceState != "VM stopped" {
				log.Infof("The instance state is %v", instanceState)
				return errors.Errorf("instance is not yet in stopped state")
			}
			log.Infof("The instance state is %v", instanceState)
			return nil
		})
}

//WaitForAzureComputeUp will wait for the azure compute instance to get in running state
func WaitForAzureComputeUp(timeout, delay int, subscriptionID, resourceGroup, azureInstanceName string) error {

	log.Info("[Status]: Checking Azure instance status")
	return retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {

			instanceState, err := GetAzureInstanceStatus(subscriptionID, resourceGroup, azureInstanceName)
			if err != nil {
				return errors.Errorf("failed to get instance status")
			}
			if instanceState != "VM running" {
				log.Infof("The instance state is %v", instanceState)
				return errors.Errorf("instance is not yet in running state")
			}
			log.Infof("The instance state is %v", instanceState)
			return nil
		})
}
