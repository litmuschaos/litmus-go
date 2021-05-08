package lib

import (
	"math/rand"
	"strings"
	"time"

	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	ebs "github.com/litmuschaos/litmus-go/pkg/cloud/aws"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/kube-aws/ebs-loss-by-tag/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/math"
	"github.com/litmuschaos/litmus-go/pkg/probe"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/pkg/errors"
)

//PrepareEBSLossByTag contains the prepration and injection steps for the experiment
func PrepareEBSLossByTag(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	var err error
	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	targetEBSVolumeIDList := CalculateVolumeAffPerc(experimentsDetails.VolumeAffectedPerc, experimentsDetails.TargetVolumeIDList)
	log.Infof("[Chaos]:Number of volumes targeted: %v", len(targetEBSVolumeIDList))

	switch strings.ToLower(experimentsDetails.Sequence) {
	case "serial":
		if err = injectChaosInSerialMode(experimentsDetails, targetEBSVolumeIDList, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
			return err
		}
	case "parallel":
		if err = injectChaosInParallelMode(experimentsDetails, targetEBSVolumeIDList, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
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

//injectChaosInSerialMode will inject the ebs loss chaos in serial mode which means one after other
func injectChaosInSerialMode(experimentsDetails *experimentTypes.ExperimentDetails, targetEBSVolumeIDList []string, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	for _, volumeID := range targetEBSVolumeIDList {

		//Get volume attachment details
		ec2InstanceID, device, err := ebs.GetVolumeAttachmentDetails(volumeID, experimentsDetails.VolumeTag, experimentsDetails.Region)
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
			log.Info("[Chaos]: Attaching the EBS volume back to the instance")
			err = ebs.EBSVolumeAttach(volumeID, ec2InstanceID, device, experimentsDetails.Region)
			if err != nil {
				return errors.Errorf("ebs attachment failed, err: %v", err)
			}

			//Wait for ebs volume attachment
			log.Infof("[Wait]: Wait for EBS volume attachment for %v volume", volumeID)
			if err = ebs.WaitForVolumeAttachment(volumeID, ec2InstanceID, experimentsDetails.Region, experimentsDetails.Delay, experimentsDetails.Timeout); err != nil {
				return errors.Errorf("unable to attach the ebs volume to the ec2 instance, err: %v", err)
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
			log.Info("[Chaos]: Attaching the EBS volume from the instance")
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

//CalculateVolumeAffPerc will calculate the target volume ids according to the volume affected percentage provided.
func CalculateVolumeAffPerc(volumeAffPerc int, volumeList []string) []string {

	var newVolumeIDList []string
	newInstanceListLength := math.Maximum(1, math.Adjustment(volumeAffPerc, len(volumeList)))
	rand.Seed(time.Now().UnixNano())

	// it will generate the random instanceList
	// it starts from the random index and choose requirement no of volumeID next to that index in a circular way.
	index := rand.Intn(len(volumeList))
	for i := 0; i < newInstanceListLength; i++ {
		newVolumeIDList = append(newVolumeIDList, volumeList[index])
		index = (index + 1) % len(volumeList)
	}
	return newVolumeIDList
}
