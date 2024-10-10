package duplication

import (
	"context"

	network_chaos "github.com/litmuschaos/litmus-go/chaoslib/litmus/network-chaos/lib"
	"github.com/litmuschaos/litmus-go/pkg/clients"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/network-chaos/types"
	"github.com/litmuschaos/litmus-go/pkg/telemetry"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"go.opentelemetry.io/otel"
)

// PodNetworkDuplicationChaos contains the steps to prepare and inject chaos
func PodNetworkDuplicationChaos(ctx context.Context, experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {
	ctx, span := otel.Tracer(telemetry.TracerName).Start(ctx, "PreparePodNetworkDuplicationFault")
	defer span.End()

	args := "duplicate " + experimentsDetails.NetworkPacketDuplicationPercentage
	return network_chaos.PrepareAndInjectChaos(ctx, experimentsDetails, clients, resultDetails, eventsDetails, chaosDetails, args)
}
