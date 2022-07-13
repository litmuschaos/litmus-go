package latency

import (
	http_chaos "github.com/litmuschaos/litmus-go/chaoslib/litmus/http-chaos/lib"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/http-chaos/types"
	"github.com/litmuschaos/litmus-go/pkg/types"
)

//PodHttpModifyHeaderChaos contains the steps to prepare and inject http latency chaos
func PodHttpModifyHeaderChaos(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {
	var stream string
	if experimentsDetails.HeaderMode == "request" {
		stream = "upstream"
	} else {
		stream = "downstream"
	}
	args := "-t header --" + stream + " -a headers='" + (experimentsDetails.HeadersMap) + "' -a mode=" + experimentsDetails.HeaderMode
	return http_chaos.PrepareAndInjectChaos(experimentsDetails, clients, resultDetails, eventsDetails, chaosDetails, args)
}
