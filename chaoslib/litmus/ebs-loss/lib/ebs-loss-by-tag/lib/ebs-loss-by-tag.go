package lib

import (
	"os"
	"os/signal"
	"strings"
	"syscall"

	ebsloss "github.com/litmuschaos/litmus-go/chaoslib/litmus/ebs-loss/lib"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	ebs "github.com/litmuschaos/litmus-go/pkg/cloud/aws"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/kube-aws/ebs-loss/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/pkg/errors"
)

var (
	err           error
	inject, abort chan os.Signal
)

//PrepareEBSLossByTag contains the prepration and injection steps for the experiment
func PrepareEBSLossByTag(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	// inject channel is used to transmit signal notifications.
	inject = make(chan os.Signal, 1)
	// Catch and relay certain signal(s) to inject channel.
	signal.Notify(inject, os.Interrupt, syscall.SIGTERM)

	// abort channel is used to transmit signal notifications.
	abort = make(chan os.Signal, 1)
	// Catch and relay certain signal(s) to abort channel.
	signal.Notify(abort, os.Interrupt, syscall.SIGTERM)

	//Waiting for the ramp time before chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time before injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}

	select {
	case <-inject:
		// stopping the chaos execution, if abort signal recieved
		os.Exit(0)
	default:

		targetEBSVolumeIDList := common.CalculateVolumeAffPerc(experimentsDetails.VolumeAffectedPerc, experimentsDetails.TargetVolumeIDList)
		log.Infof("[Chaos]:Number of volumes targeted: %v", len(targetEBSVolumeIDList))

		// watching for the abort signal and revert the chaos
		go abortWatcher(experimentsDetails, targetEBSVolumeIDList)

		switch strings.ToLower(experimentsDetails.Sequence) {
		case "serial":
			if err = ebsloss.InjectChaosInSerialMode(experimentsDetails, targetEBSVolumeIDList, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
				return err
			}
		case "parallel":
			if err = ebsloss.InjectChaosInParallelMode(experimentsDetails, targetEBSVolumeIDList, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
				return err
			}
		default:
			return errors.Errorf("%v sequence is not supported", experimentsDetails.Sequence)
		}
		//Waiting for the ramp time after chaos injection
		if experimentsDetails.RampTime != 0 {
			log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
			common.WaitForDuration(experimentsDetails.RampTime)
		}
	}
	return nil
}

// watching for the abort signal and revert the chaos
func abortWatcher(experimentsDetails *experimentTypes.ExperimentDetails, volumeIDList []string) {

	<-abort
	for _, volumeID := range volumeIDList {

		log.Info("[Abort]: Chaos Revert Started")
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
	}
	os.Exit(1)
}
