package ssm

import (
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/litmuschaos/litmus-go/chaoslib/litmus/aws-ssm-chaos/lib"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/aws-ssm/aws-ssm-chaos/types"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/cloud/aws/ssm"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/pkg/errors"
)

var (
	err           error
	inject, abort chan os.Signal
)

//PrepareAWSSSMChaosByID contains the prepration and injection steps for the experiment
func PrepareAWSSSMChaosByID(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

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

	//create and upload the ssm document on the given aws service monitoring docs
	if err = ssm.CreateAndUploadDocument(experimentsDetails.DocumentName, experimentsDetails.DocumentType, experimentsDetails.DocumentFormat, experimentsDetails.DocumentPath, experimentsDetails.Region); err != nil {
		return errors.Errorf("fail to create and upload ssm doc, err: %v", err)
	}
	experimentsDetails.IsDocsUploaded = true
	log.Info("[Info]: SSM docs uploaded successfully")

	// watching for the abort signal and revert the chaos
	go lib.AbortWatcher(experimentsDetails, abort)

	//get the instance id or list of instance ids
	instanceIDList := strings.Split(experimentsDetails.EC2InstanceID, ",")
	if len(instanceIDList) == 0 {
		return errors.Errorf("no instance id found for chaos injection")
	}

	switch strings.ToLower(experimentsDetails.Sequence) {
	case "serial":
		if err = lib.InjectChaosInSerialMode(experimentsDetails, instanceIDList, clients, resultDetails, eventsDetails, chaosDetails, inject); err != nil {
			return err
		}
	case "parallel":
		if err = lib.InjectChaosInParallelMode(experimentsDetails, instanceIDList, clients, resultDetails, eventsDetails, chaosDetails, inject); err != nil {
			return err
		}
	default:
		return errors.Errorf("%v sequence is not supported", experimentsDetails.Sequence)
	}

	//Delete the ssm document on the given aws service monitoring docs
	err = ssm.SSMDeleteDocument(experimentsDetails.DocumentName, experimentsDetails.Region)
	if err != nil {
		return errors.Errorf("fail to delete ssm doc, err: %v", err)
	}

	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	return nil
}
