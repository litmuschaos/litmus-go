package rate

import (
	"context"
	"fmt"

	network_chaos "github.com/litmuschaos/litmus-go/chaoslib/litmus/network-chaos/lib"
	"github.com/litmuschaos/litmus-go/pkg/clients"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/network-chaos/types"
	"github.com/litmuschaos/litmus-go/pkg/telemetry"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"go.opentelemetry.io/otel"
)

// PodNetworkRateChaos contains the steps to prepare and inject chaos
func PodNetworkRateChaos(ctx context.Context, experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {
	ctx, span := otel.Tracer(telemetry.TracerName).Start(ctx, "PreparePodNetworkRateLimit")
	defer span.End()

	args := fmt.Sprintf("tbf rate %s burst %s limit %s", experimentsDetails.NetworkBandwidth, experimentsDetails.Burst, experimentsDetails.Limit)
	if experimentsDetails.PeakRate != "" {
		args = fmt.Sprintf("%s peakrate %s", args, experimentsDetails.PeakRate)
	}
	if experimentsDetails.MinBurst != "" {
		args = fmt.Sprintf("%s mtu %s", args, experimentsDetails.MinBurst)
	}

	return network_chaos.PrepareAndInjectChaos(ctx, experimentsDetails, clients, resultDetails, eventsDetails, chaosDetails, args)
}
