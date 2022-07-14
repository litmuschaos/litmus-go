package statuscode

import (
	"fmt"

	http_chaos "github.com/litmuschaos/litmus-go/chaoslib/litmus/http-chaos/lib"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/http-chaos/types"
	"github.com/litmuschaos/litmus-go/pkg/types"
)

//PodHttpStatusCodeChaos contains the steps to prepare and inject http status code chaos
func PodHttpStatusCodeChaos(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	args := fmt.Sprintf("-t status_code -a status_code=%d -a modify_response_body=%d", experimentsDetails.StatusCode, experimentsDetails.ModifyResponseBody)
	return http_chaos.PrepareAndInjectChaos(experimentsDetails, clients, resultDetails, eventsDetails, chaosDetails, args)
}