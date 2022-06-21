package reset

import (
	"strconv"

	http_chaos "github.com/litmuschaos/litmus-go/chaoslib/litmus/http-chaos/lib"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/http-chaos/types"
	"github.com/litmuschaos/litmus-go/pkg/types"
)

//PodHttpResetPeerChaos contains the steps to prepare and inject http latency chaos
func PodHttpResetPeerChaos(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	args := "-t reset_peer -a timeout=" + strconv.Itoa(experimentsDetails.ResetTimeout)
	return http_chaos.PrepareAndInjectChaos(experimentsDetails, clients, resultDetails, eventsDetails, chaosDetails, args)
}
