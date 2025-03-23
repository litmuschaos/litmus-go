package latency

import (
	"context"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"strconv"

	http_chaos "github.com/litmuschaos/litmus-go/chaoslib/litmus/http-chaos/lib"
	"github.com/litmuschaos/litmus-go/pkg/clients"
	experimentTypes "github.com/litmuschaos/litmus-go/pkg/generic/http-chaos/types"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"github.com/litmuschaos/litmus-go/pkg/telemetry"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
)

// PodHttpLatencyChaos contains the steps to prepare and inject http latency chaos
func PodHttpLatencyChaos(ctx context.Context, experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {
	ctx, span := otel.Tracer(telemetry.TracerName).Start(ctx, "PreparePodHttpLatencyFault",
		trace.WithAttributes(attribute.Int("experiment.ramptime", experimentsDetails.RampTime)),
	)
	defer span.End()

	log.InfoWithValues("[Info]: The chaos tunables are:", logrus.Fields{
		"Target Port":      experimentsDetails.TargetServicePort,
		"Listen Port":      experimentsDetails.ProxyPort,
		"Sequence":         experimentsDetails.Sequence,
		"PodsAffectedPerc": experimentsDetails.PodsAffectedPerc,
		"Toxicity":         experimentsDetails.Toxicity,
		"Latency":          experimentsDetails.Latency,
	})

	args := "-t latency -a latency=" + strconv.Itoa(experimentsDetails.Latency)
	return http_chaos.PrepareAndInjectChaos(ctx, experimentsDetails, clients, resultDetails, eventsDetails, chaosDetails, args)
}
