package azure

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/compute/mgmt/compute"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/azure/instance-terminate/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/pkg/errors"
)

//GetAzureInstanceStatus will verify and give the ec2 instance details along with ebs volume idetails.
func GetAzureInstanceStatus(experimentsDetails *experimentTypes.ExperimentDetails) (string, error) {

	vmClient := compute.NewVirtualMachinesClient(experimentsDetails.SubscriptionID)

	authorizer, err := auth.NewAuthorizerFromFile(azure.PublicCloud.ResourceManagerEndpoint)
	if err == nil {
		vmClient.Authorizer = authorizer
	} else {
		return "", errors.Errorf("fail to setup authorization, err: %v", err)
	}

	instanceDetails, err := vmClient.InstanceView(context.TODO(), experimentsDetails.ResourceGroup, experimentsDetails.AzureInstanceName)
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
			log.Infof("[Status]: The instance %v state is: '%s'", experimentsDetails.AzureInstanceName, *instance.DisplayStatus)
			return *instance.DisplayStatus, nil
		}
	}
	return "", nil
}

// SetupSubsciptionID fetch the subscription id from the auth file and export it in experiment struct variable
func SetupSubsciptionID(experimentsDetails *experimentTypes.ExperimentDetails) error {

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
