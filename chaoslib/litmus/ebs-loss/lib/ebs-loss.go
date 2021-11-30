package lib

import (
	"os"
	"time"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	ebs "github.com/litmuschaos/litmus-go/pkg/cloud/aws/ebs"
	"github.com/litmuschaos/litmus-go/pkg/events"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/kube-aws/ebs-loss/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/pkg/errors"
)

//InjectChaosInSerialMode will inject the ebs loss chaos in serial mode which means one after other
func InjectChaosInSerialMode(experimentsDetails *experimentTypes.ExperimentDetails, targetEBSVolumeIDList []string, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	//ChaosStartTimeStamp contains the start timestamp, when the chaos injection begin
	ChaosStartTimeStamp := time.Now()
	duration := int(time.Since(ChaosStartTimeStamp).Seconds())

	for duration < experimentsDetails.ChaosDuration {

		if experimentsDetails.EngineName != "" {
			msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on ec2 instance"
			types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
		}
		for i, volumeID := range targetEBSVolumeIDList {

			//Get volume attachment details
			ec2InstanceID, device, err := ebs.GetVolumeAttachmentDetails(volumeID, experimentsDetails.VolumeTag, experimentsDetails.Region)
			if err != nil {
				return errors.Errorf("fail to get the attachment info, err: %v", err)
			}

			//Detaching the ebs volume from the instance
			log.Info("[Chaos]: Detaching the EBS volume from the instance")
			if err = ebs.EBSVolumeDetach(volumeID, experimentsDetails.Region); err != nil {
				return errors.Errorf("ebs detachment failed, err: %v", err)
			}

			common.SetTargets(volumeID, "injected", "EBS", chaosDetails)

			//Wait for ebs volume detachment
			log.Infof("[Wait]: Wait for EBS volume detachment for volume %v", volumeID)
			if err = ebs.WaitForVolumeDetachment(volumeID, ec2InstanceID, experimentsDetails.Region, experimentsDetails.Delay, experimentsDetails.Timeout); err != nil {
				return errors.Errorf("unable to detach the ebs volume to the ec2 instance, err: %v", err)
			}

			// run the probes during chaos
			// the OnChaos probes execution will start in the first iteration and keep running for the entire chaos duration
			if len(resultDetails.ProbeDetails) != 0 && i == 0 {
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
				log.Info("[Chaos]: Attaching the EBS volume back to the instance")
				if err = ebs.EBSVolumeAttach(volumeID, ec2InstanceID, device, experimentsDetails.Region); err != nil {
					return errors.Errorf("ebs attachment failed, err: %v", err)
				}

				//Wait for ebs volume attachment
				log.Infof("[Wait]: Wait for EBS volume attachment for %v volume", volumeID)
				if err = ebs.WaitForVolumeAttachment(volumeID, ec2InstanceID, experimentsDetails.Region, experimentsDetails.Delay, experimentsDetails.Timeout); err != nil {
					return errors.Errorf("unable to attach the ebs volume to the ec2 instance, err: %v", err)
				}
			}
			common.SetTargets(volumeID, "reverted", "EBS", chaosDetails)
		}
		duration = int(time.Since(ChaosStartTimeStamp).Seconds())
	}
	return nil
}

//InjectChaosInParallelMode will inject the chaos in parallel mode that means all at once
func InjectChaosInParallelMode(experimentsDetails *experimentTypes.ExperimentDetails, targetEBSVolumeIDList []string, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	var ec2InstanceIDList, deviceList []string

	//ChaosStartTimeStamp contains the start timestamp, when the chaos injection begin
	ChaosStartTimeStamp := time.Now()
	duration := int(time.Since(ChaosStartTimeStamp).Seconds())

	for duration < experimentsDetails.ChaosDuration {

		if experimentsDetails.EngineName != "" {
			msg := "Injecting " + experimentsDetails.ExperimentName + " chaos on ec2 instance"
			types.SetEngineEventAttributes(eventsDetails, types.ChaosInject, msg, "Normal", chaosDetails)
			events.GenerateEvents(eventsDetails, clients, chaosDetails, "ChaosEngine")
		}

		//prepare the instaceIDs and device name for all the given volume
		for _, volumeID := range targetEBSVolumeIDList {
			ec2InstanceID, device, err := ebs.GetVolumeAttachmentDetails(volumeID, experimentsDetails.VolumeTag, experimentsDetails.Region)
			if err != nil || ec2InstanceID == "" || device == "" {
				return errors.Errorf("fail to get the attachment info, err: %v", err)
			}
			ec2InstanceIDList = append(ec2InstanceIDList, ec2InstanceID)
			deviceList = append(deviceList, device)
		}

		for _, volumeID := range targetEBSVolumeIDList {
			//Detaching the ebs volume from the instance
			log.Info("[Chaos]: Detaching the EBS volume from the instance")
			if err := ebs.EBSVolumeDetach(volumeID, experimentsDetails.Region); err != nil {
				return errors.Errorf("ebs detachment failed, err: %v", err)
			}
			common.SetTargets(volumeID, "injected", "EBS", chaosDetails)
		}

		log.Info("[Info]: Checking if the detachment process initiated")
		if err := ebs.CheckEBSDetachmentInitialisation(targetEBSVolumeIDList, ec2InstanceIDList, experimentsDetails.Region); err != nil {
			return errors.Errorf("fail to initialise the detachment")
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
				log.Info("[Chaos]: Attaching the EBS volume from the instance")
				if err = ebs.EBSVolumeAttach(volumeID, ec2InstanceIDList[i], deviceList[i], experimentsDetails.Region); err != nil {
					return errors.Errorf("ebs attachment failed, err: %v", err)
				}

				//Wait for ebs volume attachment
				log.Infof("[Wait]: Wait for EBS volume attachment for volume %v", volumeID)
				if err = ebs.WaitForVolumeAttachment(volumeID, ec2InstanceIDList[i], experimentsDetails.Region, experimentsDetails.Delay, experimentsDetails.Timeout); err != nil {
					return errors.Errorf("unable to attach the ebs volume to the ec2 instance, err: %v", err)
				}
			}
			common.SetTargets(volumeID, "reverted", "EBS", chaosDetails)
		}
		duration = int(time.Since(ChaosStartTimeStamp).Seconds())
	}
	return nil
}

// AbortWatcher will watching for the abort signal and revert the chaos
func AbortWatcher(experimentsDetails *experimentTypes.ExperimentDetails, volumeIDList []string, abort chan os.Signal, chaosDetails *types.ChaosDetails) {

	<-abort

	log.Info("[Abort]: Chaos Revert Started")
	for _, volumeID := range volumeIDList {
		//Get volume attachment details
		instanceID, deviceName, err := ebs.GetVolumeAttachmentDetails(volumeID, experimentsDetails.VolumeTag, experimentsDetails.Region)
		if err != nil {
			log.Errorf("fail to get the attachment info, err: %v", err)
		}

		//Getting the EBS volume attachment status
		ebsState, err := ebs.GetEBSStatus(experimentsDetails.EBSVolumeID, instanceID, experimentsDetails.Region)
		if err != nil {
			log.Errorf("failed to get the ebs status when an abort signal is received, err: %v", err)
		}
		if ebsState != "attached" {

			//Wait for ebs volume detachment
			//We first wait for the volume to get in detached state then we are attaching it.
			log.Info("[Abort]: Wait for EBS complete volume detachment")
			if err = ebs.WaitForVolumeDetachment(experimentsDetails.EBSVolumeID, instanceID, experimentsDetails.Region, experimentsDetails.Delay, experimentsDetails.Timeout); err != nil {
				log.Errorf("unable to detach the ebs volume, err: %v", err)
			}
			//Attaching the ebs volume from the instance
			log.Info("[Chaos]: Attaching the EBS volume from the instance")
			err = ebs.EBSVolumeAttach(experimentsDetails.EBSVolumeID, instanceID, deviceName, experimentsDetails.Region)
			if err != nil {
				log.Errorf("ebs attachment failed when an abort signal is received, err: %v", err)
			}
		}
		common.SetTargets(volumeID, "reverted", "EBS", chaosDetails)
	}
	log.Info("[Abort]: Chaos Revert Completed")
	os.Exit(1)
}
