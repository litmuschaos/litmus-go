package lib

import (
	"strings"
	"time"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	ebs "github.com/litmuschaos/litmus-go/pkg/cloud/aws"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/kube-aws/ebs-loss-by-id/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/pkg/errors"
)

//PrepareEBSLossByID contains the prepration and injection steps for the experiment
func PrepareEBSLossByID(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	var err error
	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	//get the volume id or list of instance ids
	volumeIDList := strings.Split(experimentsDetails.EBSVolumeID, ",")
	if len(volumeIDList) == 0 {
		return errors.Errorf("no volume id found to detach")
	}

	if strings.ToLower(experimentsDetails.Sequence) == "serial" {
		if err = injectChaosInSerialMode(experimentsDetails, volumeIDList, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
			return err
		}
	} else {
		if err = injectChaosInParallelMode(experimentsDetails, volumeIDList, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
			return err
		}
	}
	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	return nil
}

//injectChaosInSerialMode will inject the ec2 instance termination in serial mode that is one after other
func injectChaosInSerialMode(experimentsDetails *experimentTypes.ExperimentDetails, volumeIDList []string, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	//ChaosStartTimeStamp contains the start timestamp, when the chaos injection begin
	ChaosStartTimeStamp := time.Now().Unix()

loop:
	for {

		log.Infof("[Info]: Target instanceID list, %v", volumeIDList)

		if experimentsDetails.EngineName != "" {
			msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on ec2 instance"
			types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
		}

		//Detach the ebs volume
		for _, volumeID := range volumeIDList {

			//Get volume attachment details
			ec2InstanceID, device, err := ebs.GetVolumeAttachmentDetails(volumeID, "", experimentsDetails.Region)
			if err != nil {
				return errors.Errorf("fail to get the attachment info, err: %v", err)
			}

			//Detaching the ebs volume from the instance
			log.Info("[Chaos]: Detaching the EBS volume from the instance")
			err = ebs.EBSVolumeDetach(volumeID, experimentsDetails.Region)
			if err != nil {
				return errors.Errorf("ebs detachment failed, err: %v", err)
			}

			//Wait for ebs volume detachment
			log.Infof("[Wait]: Wait for EBS volume detachment for volume %v", volumeID)
			if err = ebs.WaitForVolumeDetachment(volumeID, ec2InstanceID, experimentsDetails.Region, experimentsDetails.Delay, experimentsDetails.Timeout); err != nil {
				return errors.Errorf("unable to detach the ebs volume to the ec2 instance, err: %v", err)
			}

			// run the probes during chaos
			if len(resultDetails.ProbeDetails) != 0 {
				if err = probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
					return err
				}
			}

			//Wait for chaos duration
			log.Infof("[Wait]: Waiting for the chaos interval of %vs", experimentsDetails.ChaosInterval)
			common.WaitForDuration(experimentsDetails.ChaosInterval)

			//Getting the EBS volume attachment status
			ebsState, err := ebs.GetEBSStatus(volumeID, ec2InstanceID, experimentsDetails.Region)
			if err != nil {
				return errors.Errorf("failed to get the ebs status, err: %v", err)
			}

			switch ebsState {
			case "attached":
				log.Info("[Skip]: The EBS volume is already attached")
			default:
				//Attaching the ebs volume from the instance
				log.Infof("[Chaos]: Attaching the EBS volume %v to the instance", volumeID)
				err = ebs.EBSVolumeAttach(volumeID, ec2InstanceID, device, experimentsDetails.Region)
				if err != nil {
					return errors.Errorf("ebs attachment failed, err: %v", err)
				}

				//Wait for ebs volume attachment
				log.Infof("[Wait]: Wait for EBS volume attachment for volume %v", volumeID)
				if err = ebs.WaitForVolumeAttachment(volumeID, ec2InstanceID, experimentsDetails.Region, experimentsDetails.Delay, experimentsDetails.Timeout); err != nil {
					return errors.Errorf("unable to attach the ebs volume to the ec2 instance, err: %v", err)
				}
			}

			//ChaosCurrentTimeStamp contains the current timestamp
			ChaosCurrentTimeStamp := time.Now().Unix()

			//ChaosDiffTimeStamp contains the difference of current timestamp and start timestamp
			//It will helpful to track the total chaos duration
			chaosDiffTimeStamp := ChaosCurrentTimeStamp - ChaosStartTimeStamp

			if int(chaosDiffTimeStamp) >= experimentsDetails.ChaosDuration {
				log.Infof("[Chaos]: Time is up for experiment: %v", experimentsDetails.ExperimentName)
				break loop
			}

		}
	}

	return nil
}

//injectChaosInParallelMode will inject the chaos in parallel mode that means all at once
func injectChaosInParallelMode(experimentsDetails *experimentTypes.ExperimentDetails, targetEBSVolumeIDList []string, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	var ec2InstanceIDList []string
	var deviceList []string

	//prepare the instaceIDs and device name for all the give volume
	for _, volumeID := range targetEBSVolumeIDList {
		ec2InstanceID, device, err := ebs.GetVolumeAttachmentDetails(volumeID, "", experimentsDetails.Region)
		if err != nil || ec2InstanceID == "" || device == "" {
			return errors.Errorf("fail to get the attachment info, err: %v", err)
		}
		ec2InstanceIDList = append(ec2InstanceIDList, ec2InstanceID)
		deviceList = append(deviceList, device)
	}

	for _, volumeID := range targetEBSVolumeIDList {
		//Detaching the ebs volume from the instance
		log.Info("[Chaos]: Detaching the EBS volume from the instance")
		err := ebs.EBSVolumeDetach(volumeID, experimentsDetails.Region)
		if err != nil {
			return errors.Errorf("ebs detachment failed, err: %v", err)
		}
	}

	for i, volumeID := range targetEBSVolumeIDList {
		//Wait for ebs volume detachment
		log.Infof("[Wait]: Wait for EBS volume detachment for volume %v", volumeID)
		if err := ebs.WaitForVolumeDetachment(volumeID, ec2InstanceIDList[i], experimentsDetails.Region, experimentsDetails.Delay, experimentsDetails.Timeout); err != nil {
			return errors.Errorf("unable to detach the ebs volume to the ec2 instance, err: %v", err)
		}
	}

	// run the probes during chaos
	if len(resultDetails.ProbeDetails) != 0 {
		if err := probe.RunProbes(chaosDetails, clients, resultDetails, "DuringChaos", eventsDetails); err != nil {
			return err
		}
	}

	//Wait for chaos interval
	log.Infof("[Wait]: Waiting for the chaos interval of %vs", experimentsDetails.ChaosInterval)
	common.WaitForDuration(experimentsDetails.ChaosInterval)

	for i, volumeID := range targetEBSVolumeIDList {

		//Getting the EBS volume attachment status
		ebsState, err := ebs.GetEBSStatus(volumeID, ec2InstanceIDList[i], experimentsDetails.Region)
		if err != nil {
			return errors.Errorf("failed to get the ebs status, err: %v", err)
		}

		switch ebsState {
		case "attached":
			log.Info("[Skip]: The EBS volume is already attached")
		default:
			//Attaching the ebs volume from the instance
			log.Infof("[Chaos]: Attaching the EBS volume %v to the instance", volumeID)
			err = ebs.EBSVolumeAttach(volumeID, ec2InstanceIDList[i], deviceList[i], experimentsDetails.Region)
			if err != nil {
				return errors.Errorf("ebs attachment failed, err: %v", err)
			}

			//Wait for ebs volume attachment
			log.Infof("[Wait]: Wait for EBS volume attachment for volume %v", volumeID)
			if err = ebs.WaitForVolumeAttachment(volumeID, ec2InstanceIDList[i], experimentsDetails.Region, experimentsDetails.Delay, experimentsDetails.Timeout); err != nil {
				return errors.Errorf("unable to attach the ebs volume to the ec2 instance, err: %v", err)
			}
		}
	}

	return nil
}
