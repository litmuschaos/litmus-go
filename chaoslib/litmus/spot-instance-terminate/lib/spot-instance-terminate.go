package lib

import (
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	awslib "github.com/litmuschaos/litmus-go/pkg/cloud/aws"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/kube-aws/spot-instance-terminate/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/pkg/errors"
)

//PrepareSpotInstanceTerminate contains the prepration and injection steps for the experiment
func PrepareSpotInstanceTerminate(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	//Get spot instance ID
	instanceID, err := awslib.GetSpotFleetInstanceID(experimentsDetails.SpotFleetRequestID, experimentsDetails.Region)
	if err != nil {
		return errors.Errorf("fail to get the spot instace id, err: %v", err)
	}

	//Check spot instance status
	instanceStatus, err := awslib.GetEC2InstanceStatus(instanceID, experimentsDetails.Region)
	if err != nil {
		return errors.Errorf("fail to get the spot instance status, err: %v", err)
	}
	if instanceStatus != "running" {
		return errors.Errorf("fail to get the spot instance status in running state, current state is: %v ", instanceStatus)
	}

	if experimentsDetails.EngineName != "" {
		msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on spot instance"
		types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
		events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
	}

	//Cancelthe spot fleet request
	err = awslib.CancelSpotFleetRequest(experimentsDetails.SpotFleetRequestID, experimentsDetails.Region)
	if err != nil {
		return errors.Errorf("fail to cancel spot fleet request, err: %v", err)
	}

	// run the probes during chaos
	if len(resultDetails.ProbeDetails) != 0 {
		if err = probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
			return err
		}
	}

	// wait for spot instance to terminate
	err = awslib.WaitForEC2TargetState("terminated", instanceID, experimentsDetails.Region, experimentsDetails.Timeout, experimentsDetails.Delay)
	if err != nil {
		return errors.Errorf("fail to terminate spot instance, err: %v", err)
	}

	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	return nil
}

// PreChaosSpotFleetRequestStatusCheck will the spot-fleet-request before chaos injection
func PreChaosSpotFleetRequestStatusCheck(experimentsDetails *experimentTypes.ExperimentDetails) error {

	spotFleetRequestState, err := awslib.GetSpotFleetRequestState(experimentsDetails.SpotFleetRequestID, experimentsDetails.Region)
	if err != nil {
		return errors.Errorf("fail to get the spot fleet request, err: %v", err)
	}
	if spotFleetRequestState != "active" {
		return errors.Errorf("the spot fleet request is not in active state, the current state is: %v", spotFleetRequestState)
	}
	return nil
}

// PostChaosSpotFleetRequestStatusCheck will the spot-fleet-request after chaos injection
func PostChaosSpotFleetRequestStatusCheck(experimentsDetails *experimentTypes.ExperimentDetails) error {

	spotFleetRequestState, err := awslib.GetSpotFleetRequestState(experimentsDetails.SpotFleetRequestID, experimentsDetails.Region)
	if err != nil {
		return errors.Errorf("fail to get the spot fleet request, err: %v", err)
	}
	if spotFleetRequestState != "cancelled" && spotFleetRequestState != "cancelled_terminating" {
		return errors.Errorf("the spot fleet request is not cancelled, the current state is: %v", spotFleetRequestState)
	}
	return nil
}
