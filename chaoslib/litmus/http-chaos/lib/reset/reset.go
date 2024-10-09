package reset

import (
	"context"
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

// PodHttpResetPeerChaos contains the steps to prepare and inject http reset peer chaos
func PodHttpResetPeerChaos(ctx context.Context, experimentsDetails *experimentTypes.ExperimentDetails, clients clients.ClientSets, resultDetails *types.ResultDetails, eventsDetails *types.EventDetails, chaosDetails *types.ChaosDetails) error {
	ctx, span := otel.Tracer(telemetry.TracerName).Start(ctx, "PreparePodHTTPResetPeerFault")
	defer span.End()

	log.InfoWithValues("[Info]: The chaos tunables are:", logrus.Fields{
		"Target Port":      experimentsDetails.TargetServicePort,
		"Listen Port":      experimentsDetails.ProxyPort,
		"Sequence":         experimentsDetails.Sequence,
		"PodsAffectedPerc": experimentsDetails.PodsAffectedPerc,
		"Toxicity":         experimentsDetails.Toxicity,
		"Reset Timeout":    experimentsDetails.ResetTimeout,
	})

	args := "-t reset_peer -a timeout=" + strconv.Itoa(experimentsDetails.ResetTimeout)
	return http_chaos.PrepareAndInjectChaos(ctx, experimentsDetails, clients, resultDetails, eventsDetails, chaosDetails, args)
}
