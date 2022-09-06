package modifybody

import (
	"fmt"
	"math"

	http_chaos "github.com/litmuschaos/litmus-go/chaoslib/litmus/http-chaos/lib"
	clients "github.com/litmuschaos/litmus-go/pkg/clients"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/http-chaos/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/sirupsen/logrus"
)

// PodHttpModifyBodyChaos contains the steps to prepare and inject http modify body chaos
func PodHttpModifyBodyChaos(experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {

	// responseBodyMaxLength defines the max length of response body string to be printed. It is taken as
	// the min of length of body and 120 characters to avoid printing large response body.
	responseBodyMaxLength := int(math.Min(float64(len(experimentsDetails.ResponseBody)), 120))

	log.InfoWithValues("[Info]: The chaos tunables are:", logrus.Fields{
		"Target Port":      experimentsDetails.TargetServicePort,
		"Listen Port":      experimentsDetails.ProxyPort,
		"Sequence":         experimentsDetails.Sequence,
		"PodsAffectedPerc": experimentsDetails.PodsAffectedPerc,
		"Toxicity":         experimentsDetails.Toxicity,
		"ResponseBody":     experimentsDetails.ResponseBody[0:responseBodyMaxLength],
		"Content Type":     experimentsDetails.ContentType,
		"Content Encoding": experimentsDetails.ContentEncoding,
	})

	args := fmt.Sprintf(
		"-t modify_body -a body=\"%v\" -a content_type=%v -a content_encoding=%v",
		experimentsDetails.ResponseBody, experimentsDetails.ContentType, experimentsDetails.ContentEncoding)
	return http_chaos.PrepareAndInjectChaos(experimentsDetails, clients, resultDetails, eventsDetails, chaosDetails, args)
}
