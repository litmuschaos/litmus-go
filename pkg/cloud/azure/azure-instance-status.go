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
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/azure/instance-stop/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/pkg/errors"
)

//GetAzureInstanceStatus will verify the azure instance state details
func GetAzureInstanceStatus(subscriptionID, resourceGroup, azureInstanceName string) (string, error) {

	vmClient := compute.NewVirtualMachinesClient(subscriptionID)

	authorizer, err := auth.NewAuthorizerFromFile(azure.PublicCloud.ResourceManagerEndpoint)
	if err == nil {
		vmClient.Authorizer = authorizer
	} else {
		return "", errors.Errorf("fail to setup authorization, err: %v", err)
	}

	instanceDetails, err := vmClient.InstanceView(context.TODO(), resourceGroup, azureInstanceName)
	if err != nil {
		return "", errors.Errorf("fail to get the instance to check status, err: %v", err)
	}
	// The *instanceDetails.Statuses list contains the instance status details as shown below
	// Item 1: Provisioning succeeded
	// Item 2: VM running
	if len(*instanceDetails.Statuses) < 2 {
		return "", errors.Errorf("fail to get the instatus vm status")
	}

	for i, instance := range *instanceDetails.Statuses {
		// For VM status only
		if i == 1 {
			log.Infof("[Status]: The instance %v state is: '%s'", azureInstanceName, *instance.DisplayStatus)
			return *instance.DisplayStatus, nil
		}
	}
	return "", nil

	// To print VM status
	// TODO: Decide on using the method for display after discussion
	// log.Infof("[Status]: The instance %v state is: '%s'", azureInstanceName, *(*instanceDetails.Statuses)[1].DisplayStatus)
	// return *(*instanceDetails.Statuses)[1].DisplayStatus, nil
}

// SetupSubsciptionID fetch the subscription id from the auth file and export it in experiment struct variable
func SetupSubscriptionID(experimentsDetails *experimentTypes.ExperimentDetails) error {

	var err error
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

// InstanceStatusCheckByName is used to check the instance status of all the instance under chaos
func InstanceStatusCheckByName(experimentsDetails *experimentTypes.ExperimentDetails) error {
	instanceNameList := strings.Split(experimentsDetails.AzureInstanceName, ",")
	if len(instanceNameList) == 0 {
		return errors.Errorf("no instance found to terminate")
	}
	log.Infof("[Info]: The instance under chaos(IUC) are: %v", instanceNameList)
	return InstanceStatusCheck(instanceNameList, experimentsDetails.SubscriptionID, experimentsDetails.ResourceGroup)
}

// InstanceStatusCheckByName is used to check the instance status of given list of instances
func InstanceStatusCheck(targetInstanceNameList []string, subscriptionID, resourceGroup string) error {

	for _, vmName := range targetInstanceNameList {
		instanceState, err := GetAzureInstanceStatus(subscriptionID, resourceGroup, vmName)
		if err != nil {
			return err
		}
		if instanceState != "VM running" {
			return errors.Errorf("failed to get the azure instance '%v' in running state, current state: %v", vmName, instanceState)
		}
	}
	return nil
}
