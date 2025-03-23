package loss

import (
	"context"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	network_chaos "github.com/litmuschaos/litmus-go/chaoslib/litmus/network-chaos/lib"
	"github.com/litmuschaos/litmus-go/pkg/clients"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/network-chaos/types"
	"github.com/litmuschaos/litmus-go/pkg/telemetry"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"go.opentelemetry.io/otel"
)

// PodNetworkLossChaos contains the steps to prepare and inject chaos
func PodNetworkLossChaos(ctx context.Context, experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {
	ctx, span := otel.Tracer(telemetry.TracerName).Start(ctx, "PreparePodNetworkLossFault",
		trace.WithAttributes(attribute.Int("experiment.ramptime", experimentsDetails.RampTime)),
	)
	defer span.End()

	args := "loss " + experimentsDetails.NetworkPacketLossPercentage
	return network_chaos.PrepareAndInjectChaos(ctx, experimentsDetails, clients, resultDetails, eventsDetails, chaosDetails, args)
}
