package latency

import (
	http_chaos "github.com/litmuschaos/litmus-go/chaoslib/litmus/http-chaos/lib"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/http-chaos/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/sirupsen/logrus"
)

//PodHttpModifyHeaderChaos contains the steps to prepare and inject http latency chaos
func PodHttpModifyHeaderChaos(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	log.InfoWithValues("[Info]: The chaos tunables are:", logrus.Fields{
		"Target Port":      experimentsDetails.TargetServicePort,
		"Listen Port":      experimentsDetails.ProxyPort,
		"Sequence":         experimentsDetails.Sequence,
		"PodsAffectedPerc": experimentsDetails.PodsAffectedPerc,
		"Headers":          experimentsDetails.HeadersMap,
		"Header Mode":      experimentsDetails.HeaderMode,
	})

	stream := "downstream"
	if experimentsDetails.HeaderMode == "request" {
		stream = "upstream"
	}
	args := "-t header --" + stream + " -a headers='" + (experimentsDetails.HeadersMap) + "' -a mode=" + experimentsDetails.HeaderMode
	return http_chaos.PrepareAndInjectChaos(experimentsDetails, clients, resultDetails, eventsDetails, chaosDetails, args)
}
