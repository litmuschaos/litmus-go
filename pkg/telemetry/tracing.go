package telemetry

import (
	"context"
	"encoding/json"
	"os"

	"github.com/litmuschaos/litmus-go/pkg/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

const (
	TracerName  = "litmuschaos.io/litmus-go"
	TraceParent = "TRACE_PARENT"
)

func GetTraceParentContext() context.Context {
	traceParent := os.Getenv(TraceParent)

	if traceParent == "" {
		return context.Background()
	}

	pro := otel.GetTextMapPropagator()
	carrier := make(map[string]string)
	if err := json.Unmarshal([]byte(traceParent), &carrier); err != nil {
		log.Fatal(err.Error())
	}

	return pro.Extract(context.Background(), propagation.MapCarrier(carrier))
}

// GetMarshalledSpanFromContext Extract spanContext from the context and return it as json encoded string
func GetMarshalledSpanFromContext(ctx context.Context) string {
	carrier := make(map[string]string)
	pro := otel.GetTextMapPropagator()

	pro.Inject(ctx, propagation.MapCarrier(carrier))

	if len(carrier) == 0 {
		log.Error("spanContext not present in the context, unable to marshall")
		return ""
	}

	marshalled, err := json.Marshal(carrier)
	if err != nil {
		log.Error(err.Error())
		return ""
	}
	if len(marshalled) >= 1024 {
		log.Error("marshalled span context is too large, unable to marshall")
		return ""
	}
	return string(marshalled)
}
