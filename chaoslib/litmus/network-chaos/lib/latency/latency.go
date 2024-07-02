package latency

import (
	"strconv"

	network_chaos "github.com/litmuschaos/litmus-go/chaoslib/litmus/network-chaos/lib"
	"github.com/litmuschaos/litmus-go/pkg/clients"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/network-chaos/types"
	"github.com/litmuschaos/litmus-go/pkg/telemetry"
	"github.com/litmuschaos/litmus-go/pkg/types"
)

// PodNetworkLatencyChaos contains the steps to prepare and inject chaos
func PodNetworkLatencyChaos(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {
	span := telemetry.StartTracing(clients, "InjectPodNetworkLatencyChaos")
	defer span.End()

	args := "delay " + strconv.Itoa(experimentsDetails.NetworkLatency) + "ms " + strconv.Itoa(experimentsDetails.Jitter) + "ms"
	return network_chaos.PrepareAndInjectChaos(experimentsDetails, clients, resultDetails, eventsDetails, chaosDetails, args)
}
