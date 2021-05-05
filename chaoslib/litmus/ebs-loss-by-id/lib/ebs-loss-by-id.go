package lib

import (
	"time"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	ebs "github.com/litmuschaos/litmus-go/pkg/cloud/aws"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/kube-aws/ebs-loss-by-id/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/pkg/errors"
)

//InjectEBSLoss contains the chaos injection steps for ebs loss
func InjectEBSLoss(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	var err error
	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	//Detaching the ebs volume from the instance
	log.Info("[Chaos]: Detaching the EBS volume from the instance")
	err = ebs.EBSVolumeDetach(experimentsDetails.EBSVolumeID, experimentsDetails.Region)
	if err != nil {
		return errors.Errorf("ebs detachment failed, err: %v", err)
	}

	//Wait for ebs volume detachment
	log.Info("[Wait]: Wait for EBS volume detachment")
	if err = ebs.WaitForVolumeDetachment(experimentsDetails.EBSVolumeID, experimentsDetails.Ec2InstanceID, experimentsDetails.Region, experimentsDetails.Delay, experimentsDetails.Timeout); err != nil {
		return errors.Errorf("unable to detach the ebs volume to the ec2 instance, err: %v", err)
	}

	// run the probes during chaos
	if len(resultDetails.ProbeDetails) != 0 {
		if err = probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
			return err
		}
	}

	//Wait for chaos duration
	log.Infof("[Wait]: Waiting for the chaos duration of %vs", experimentsDetails.ChaosDuration)
	time.Sleep(time.Duration(experimentsDetails.ChaosDuration) * time.Second)

	//Getting the EBS volume attachment status
	EBSStatus, err := ebs.GetEBSStatus(experimentsDetails.EBSVolumeID, experimentsDetails.Ec2InstanceID, experimentsDetails.Region)
	if err != nil {
		return errors.Errorf("failed to get the ebs status, err: %v", err)
	}

	if EBSStatus != "attached" {
		//Attaching the ebs volume from the instance
		log.Info("[Chaos]: Attaching the EBS volume from the instance")
		err = ebs.EBSVolumeAttach(experimentsDetails.EBSVolumeID, experimentsDetails.Ec2InstanceID, experimentsDetails.DeviceName, experimentsDetails.Region)
		if err != nil {
			return errors.Errorf("ebs attachment failed, err: %v", err)
		}

		//Wait for ebs volume attachment
		log.Info("[Wait]: Wait for EBS volume attachment")
		if err = ebs.WaitForVolumeAttachment(experimentsDetails.EBSVolumeID, experimentsDetails.Ec2InstanceID, experimentsDetails.Region, experimentsDetails.Delay, experimentsDetails.Timeout); err != nil {
			return errors.Errorf("unable to attach the ebs volume to the ec2 instance, err: %v", err)
		}
	} else {
		log.Info("[Skip]: The EBS volume is already attached")
	}

	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	return nil
}
