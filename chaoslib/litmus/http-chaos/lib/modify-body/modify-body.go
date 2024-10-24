package modifybody

import (
	"context"
	"fmt"
	"math"
	"strings"

	http_chaos "github.com/litmuschaos/litmus-go/chaoslib/litmus/http-chaos/lib"
	"github.com/litmuschaos/litmus-go/pkg/clients"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/http-chaos/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/telemetry"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
)

// PodHttpModifyBodyChaos contains the steps to prepare and inject http modify body chaos
func PodHttpModifyBodyChaos(ctx context.Context, experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {
	ctx, span := otel.Tracer(telemetry.TracerName).Start(ctx, "PreparePodHTTPModifyBodyFault")
	defer span.End()

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
		`-t modify_body -a body="%v" -a content_type=%v -a content_encoding=%v`,
		EscapeQuotes(experimentsDetails.ResponseBody), experimentsDetails.ContentType, experimentsDetails.ContentEncoding)
	return http_chaos.PrepareAndInjectChaos(ctx, experimentsDetails, clients, resultDetails, eventsDetails, chaosDetails, args)
}

// EscapeQuotes escapes the quotes in the given string
func EscapeQuotes(input string) string {
	output := strings.ReplaceAll(input, `\`, `\\`)
	output = strings.ReplaceAll(output, `"`, `\"`)
	return output
}
