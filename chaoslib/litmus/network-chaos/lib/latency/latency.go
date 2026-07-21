package latency

import (
	"context"
	"fmt"
	"strconv"

	network_chaos "github.com/litmuschaos/litmus-go/chaoslib/litmus/network-chaos/lib"
	"github.com/litmuschaos/litmus-go/pkg/clients"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/network-chaos/types"
	"github.com/litmuschaos/litmus-go/pkg/telemetry"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"go.opentelemetry.io/otel"
)

// PodNetworkLatencyChaos contains the steps to prepare and inject chaos
func PodNetworkLatencyChaos(ctx context.Context, experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {
	ctx, span := otel.Tracer(telemetry.TracerName).Start(ctx, "PreparePodNetworkLatencyFault")
	defer span.End()

	args := "netem delay " + strconv.Itoa(experimentsDetails.NetworkLatency) + "ms " + strconv.Itoa(experimentsDetails.Jitter) + "ms"
	if experimentsDetails.Correlation > 0 {
		args = fmt.Sprintf("%s %d", args, experimentsDetails.Correlation)
	}

	return network_chaos.PrepareAndInjectChaos(ctx, experimentsDetails, clients, resultDetails, eventsDetails, chaosDetails, args)
}
