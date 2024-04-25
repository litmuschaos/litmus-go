package latency

import (
	"strconv"

	http_chaos "github.com/litmuschaos/litmus-go/chaoslib/litmus/http-chaos/lib"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/http-chaos/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/sirupsen/logrus"
)

// PodHttpLatencyChaos contains the steps to prepare and inject http latency chaos
func PodHttpLatencyChaos(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	log.InfoWithValues("[Info]: The chaos tunables are:", logrus.Fields{
		"Target Port":      experimentsDetails.TargetServicePort,
		"Listen Port":      experimentsDetails.ProxyPort,
		"Sequence":         experimentsDetails.Sequence,
		"PodsAffectedPerc": experimentsDetails.PodsAffectedPerc,
		"Toxicity":         experimentsDetails.Toxicity,
		"Latency":          experimentsDetails.Latency,
	})

	args := "-t latency -a latency=" + strconv.Itoa(experimentsDetails.Latency)
	return http_chaos.PrepareAndInjectChaos(experimentsDetails, clients, resultDetails, eventsDetails, chaosDetails, args)
}
