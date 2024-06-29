package telemetry

import (
	"context"
	"encoding/json"
	"os"

	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const (
	TracerName  = "litmuschaos.io/litmus-go"
	TraceParent = "TRACE_PARENT"
)

func StartTracing(clients clients.ClientSets, spanName string) trace.Span {
	ctx, span := otel.Tracer(TracerName).Start(clients.Context, spanName)
	clients.Context = ctx
	return span
}

func GetTraceParentContext() context.Context {
	traceParent := os.Getenv(TraceParent)

	pro := otel.GetTextMapPropagator()
	carrier := make(map[string]string)
	if err := json.Unmarshal([]byte(traceParent), &carrier); err != nil {
		log.Fatal(err.Error())
	}

	return pro.Extract(context.Background(), propagation.MapCarrier(carrier))
}
