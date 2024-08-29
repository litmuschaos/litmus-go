package telemetry

import (
	"context"
	"errors"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.25.0"
)

const (
	OTELExperimentJobServiceName       = "chaos_experiment_job"
	OTELExperimentJobHelperServiceName = "chaos_experiment_job_helper"
)

func InitOTelSDK(ctx context.Context, isExperiment bool, endpoint string) (shutdown func(context.Context) error, err error) {
	var shutdownFuncs []func(context.Context) error

	shutdown = func(ctx context.Context) error {
		var err error
		for _, fn := range shutdownFuncs {
			err = errors.Join(err, fn(ctx))
		}
		shutdownFuncs = nil
		return err
	}

	handleErr := func(inErr error) {
		err = errors.Join(inErr, shutdown(ctx))
	}

	tracerProvider, err := newTracerProvider(ctx, isExperiment, endpoint)
	if err != nil {
		handleErr(err)
		return
	}

	prop := newPropagator()
	otel.SetTextMapPropagator(prop)

	shutdownFuncs = append(shutdownFuncs, tracerProvider.Shutdown)
	otel.SetTracerProvider(tracerProvider)

	// TODO: need to add metrics & logging provider
	return
}

func newPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}

func newTracerProvider(ctx context.Context, isExperiment bool, endpoint string) (*trace.TracerProvider, error) {
	serviceName := OTELExperimentJobHelperServiceName
	if isExperiment {
		serviceName = OTELExperimentJobServiceName
	}
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
		),
	)
	traceExporter, err := otlptrace.New(
		ctx,
		otlptracegrpc.NewClient(
			// TODO: add secure option
			otlptracegrpc.WithInsecure(),
			otlptracegrpc.WithEndpoint(endpoint),
		),
	)
	if err != nil {
		return nil, err
	}

	batchSpanProcessor := trace.NewBatchSpanProcessor(traceExporter)
	tracerProvider := trace.NewTracerProvider(
		trace.WithSampler(trace.AlwaysSample()),
		trace.WithResource(res),
		trace.WithSpanProcessor(batchSpanProcessor),
	)

	return tracerProvider, nil
}
