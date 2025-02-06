package lib

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	ebsloss "github.com/litmuschaos/litmus-go/chaoslib/litmus/ebs-loss/lib"
	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/litmuschaos/litmus-go/pkg/clients"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/kube-aws/ebs-loss/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/telemetry"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/litmuschaos/litmus-go/pkg/utils/common"
	"github.com/palantir/stacktrace"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
)

var (
	err           error
	inject, abort chan os.Signal
)

// PrepareEBSLossByID contains the prepration and injection steps for the experiment
func PrepareEBSLossByID(ctx context.Context, experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {
	ctx, span := otel.Tracer(telemetry.TracerName).Start(ctx, "PrepareAWSEBSLossFaultByID")
	defer span.End()

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
		// stopping the chaos execution, if abort signal received
		os.Exit(0)
	default:

		//get the volume id or list of instance ids
		volumeIDList := strings.Split(experimentsDetails.EBSVolumeID, ",")
		if len(volumeIDList) == 0 {
			return cerrors.Error{ErrorCode: cerrors.ErrorTypeTargetSelection, Reason: "no volume id found to detach"}
		}
		// watching for the abort signal and revert the chaos
		go ebsloss.AbortWatcher(experimentsDetails, volumeIDList, abort, chaosDetails)

		switch strings.ToLower(experimentsDetails.Sequence) {
		case "serial":
			if err = ebsloss.InjectChaosInSerialMode(ctx, experimentsDetails, volumeIDList, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
				span.SetStatus(codes.Error, "could not run chaos in serial mode")
				span.RecordError(err)
				return stacktrace.Propagate(err, "could not run chaos in serial mode")
			}
		case "parallel":
			if err = ebsloss.InjectChaosInParallelMode(ctx, experimentsDetails, volumeIDList, clients, resultDetails, eventsDetails, chaosDetails); err != nil {
				span.SetStatus(codes.Error, "could not run chaos in parallel mode")
				span.RecordError(err)
				return stacktrace.Propagate(err, "could not run chaos in parallel mode")
			}
		default:
			span.SetStatus(codes.Error, "sequence is not supported")
			err := cerrors.Error{ErrorCode: cerrors.ErrorTypeTargetSelection, Reason: fmt.Sprintf("'%s' sequence is not supported", experimentsDetails.Sequence)}
			span.RecordError(err)
			return err
		}

		//Waiting for the ramp time after chaos injection
		if experimentsDetails.RampTime != 0 {
			log.Infof("[Ramp]: Waiting for the %vs ramp time after injecting chaos", experimentsDetails.RampTime)
			common.WaitForDuration(experimentsDetails.RampTime)
		}
	}
	return nil
}
