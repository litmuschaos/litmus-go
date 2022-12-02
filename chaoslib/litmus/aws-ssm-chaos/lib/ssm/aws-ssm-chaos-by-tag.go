package ssm

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/litmuschaos/litmus-go/chaoslib/litmus/aws-ssm-chaos/lib"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/aws-ssm/aws-ssm-chaos/types"
	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/cloud/aws/ssm"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/palantir/stacktrace"
)

// PrepareAWSSSMChaosByTag contains the prepration and injection steps for the experiment
func PrepareAWSSSMChaosByTag(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

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
		return stacktrace.Propagate(err, "could not create and upload the ssm document")
	}
	experimentsDetails.IsDocsUploaded = true
	log.Info("[Info]: SSM docs uploaded successfully")

	// watching for the abort signal and revert the chaos
	go lib.AbortWatcher(experimentsDetails, abort)
	instanceIDList := common.FilterBasedOnPercentage(experimentsDetails.InstanceAffectedPerc, experimentsDetails.TargetInstanceIDList)
	log.Infof("[Chaos]:Number of Instance targeted: %v", len(instanceIDList))

	if len(instanceIDList) == 0 {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeTargetSelection, Reason: "no instance id found for chaos injection"}
	}

	switch strings.ToLower(experimentsDetails.Sequence) {
	case "serial":
		if err = lib.InjectChaosInSerialMode(experimentsDetails, instanceIDList, clients, resultDetails, eventsDetails, chaosDetails, inject); err != nil {
			return stacktrace.Propagate(err, "could not run chaos in serial mode")
		}
	case "parallel":
		if err = lib.InjectChaosInParallelMode(experimentsDetails, instanceIDList, clients, resultDetails, eventsDetails, chaosDetails, inject); err != nil {
			return stacktrace.Propagate(err, "could not run chaos in parallel mode")
		}
	default:
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeTargetSelection, Reason: fmt.Sprintf("'%s' sequence is not supported", experimentsDetails.Sequence)}
	}

	//Delete the ssm document on the given aws service monitoring docs
	err = ssm.SSMDeleteDocument(experimentsDetails.DocumentName, experimentsDetails.Region)
	if err != nil {
		return stacktrace.Propagate(err, "failed to delete ssm doc")
	}

	//Waiting for the ramp time after chaos injection
	if experimentsDetails.RampTime != 0 {
		log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
		common.WaitForDuration(experimentsDetails.RampTime)
	}
	return nil
}
