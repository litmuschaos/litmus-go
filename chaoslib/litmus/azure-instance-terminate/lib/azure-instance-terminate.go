package lib

import (
	"context"
	"time"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/compute/mgmt/compute"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/azure/instance-terminate/types"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	azureStatus "github.com/litmuschaos/litmus-go/pkg/cloud/azure"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

//InjectAzureInstanceTerminate contains the chaos injection steps for azure instance terminate
func InjectAzureInstanceTerminate(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	var err error
	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	log.InfoWithValues("Azure instance details:", logrus.Fields{
		"Instance Name":  experimentsDetails.AzureInstanceName,
		"Resource Group": experimentsDetails.ResourceGroup,
	})

	//Stoping the azure target instance
	log.Info("[Chaos]: Stoping the target instance")
	err = AzureInstanceStop(experimentsDetails)
	if err != nil {
		return errors.Errorf("fail to stop the %v instance, err: %v", experimentsDetails.AzureInstanceName, err)
	}

	//Wait for the Total Chaos Duration
	log.Infof("[Wait]: Waiting for chaos duration of %vs before starting the instance", experimentsDetails.ChaosDuration)
	time.Sleep(time.Duration(experimentsDetails.ChaosDuration) * time.Second)

	//Starting the azure target instance
	log.Info("[Chaos]: Starting the target instance")
	err = AzureInstanceStart(experimentsDetails)
	if err != nil {
		return errors.Errorf("fail to start the %v instance, err: %v", experimentsDetails.AzureInstanceName, err)
	}

	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	return nil
}

// AzureInstanceStop poweroff the target instance
func AzureInstanceStop(experimentsDetails *experimentTypes.ExperimentDetails) error {
	vmClient := compute.NewVirtualMachinesClient(experimentsDetails.SubscriptionID)

	authorizer, err := auth.NewAuthorizerFromFile(azure.PublicCloud.ResourceManagerEndpoint)
	if err == nil {
		vmClient.Authorizer = authorizer
	} else {
		return errors.Errorf("fail to setup authorization, err: %v")
	}

	log.Info("[Info]: Starting powerOff the instance")
	_, err = vmClient.PowerOff(context.TODO(), experimentsDetails.ResourceGroup, experimentsDetails.AzureInstanceName, &vmClient.SkipResourceProviderRegistration)
	if err != nil {
		return errors.Errorf("fail to stop the %v instance, err: %v", experimentsDetails.AzureInstanceName, err)
	}

	err = retry.
		Times(uint(experimentsDetails.Timeout / experimentsDetails.Delay)).
		Wait(time.Duration(experimentsDetails.Delay) * time.Second).
		Try(func(attempt uint) error {

			instanceState, err := azureStatus.GetAzureInstanceStatus(experimentsDetails)
			if err != nil {
				return errors.Errorf("failed to get the instance status")
			}
			if instanceState != "VM stopped" {
				return errors.Errorf("instance is not yet in stoping state")
			}
			log.Infof("[Info]: The instance state is %v", instanceState)
			return nil
		})
	return err
}

// AzureInstanceStart starts the target instance
func AzureInstanceStart(experimentsDetails *experimentTypes.ExperimentDetails) error {
	vmClient := compute.NewVirtualMachinesClient(experimentsDetails.SubscriptionID)

	authorizer, err := auth.NewAuthorizerFromFile(azure.PublicCloud.ResourceManagerEndpoint)
	if err == nil {
		vmClient.Authorizer = authorizer
	} else {
		return errors.Errorf("fail to setup authorization, err: %v")
	}

	log.Info("[Info]: Starting back the instance to running state")
	_, err = vmClient.Start(context.TODO(), experimentsDetails.ResourceGroup, experimentsDetails.AzureInstanceName)
	if err != nil {
		return errors.Errorf("fail to stop the %v instance, err: %v", experimentsDetails.AzureInstanceName, err)
	}

	err = retry.
		Times(uint(experimentsDetails.Timeout / experimentsDetails.Delay)).
		Wait(time.Duration(experimentsDetails.Delay) * time.Second).
		Try(func(attempt uint) error {

			instanceState, err := azureStatus.GetAzureInstanceStatus(experimentsDetails)
			if err != nil {
				return errors.Errorf("failed to get the instance status")
			}
			if instanceState != "VM running" {
				return errors.Errorf("instance is not yet in running state")
			}
			log.Infof("[Info]: The instance state is %v", instanceState)
			return nil
		})
	return err
}
