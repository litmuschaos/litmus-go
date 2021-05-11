package lib

import (
	"strings"

	ebsloss "github.com/litmuschaos/litmus-go/chaoslib/litmus/ebs-loss/lib"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/kube-aws/ebs-loss/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
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
		if err = ebsloss.InjectChaosInSerialMode(experimentsDetails, volumeIDList, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
			return err
		}
	} else {
		if err = ebsloss.InjectChaosInParallelMode(experimentsDetails, volumeIDList, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
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
