package latency

import (
	"strconv"

	http_chaos "github.com/litmuschaos/litmus-go/chaoslib/litmus/http-chaos/lib"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/http-chaos/types"
	"github.com/litmuschaos/litmus-go/pkg/types"
)

//PodHttpLatencyChaos contains the steps to prepare and inject http latency chaos
func PodHttpLatencyChaos(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	args := "-t latency -a latency=" + strconv.Itoa(experimentsDetails.Latency)
	return http_chaos.PrepareAndInjectChaos(experimentsDetails, clients, resultDetails, eventsDetails, chaosDetails, args)
}
