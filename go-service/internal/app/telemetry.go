package app

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"github.com/gaarutyunov/skill-test/go-service/internal/config"
	"github.com/gaarutyunov/skill-test/go-service/pkg/semconv"
)

// NewTracerProvider builds an OpenTelemetry TracerProvider from configuration,
// registers it (and the W3C propagator) globally, and returns it together with
// a shutdown function.
func NewTracerProvider(ctx context.Context, cfg *config.Config) (trace.TracerProvider, func(context.Context) error, error) {
	build := semconv.Build()
	res, err := resource.New(ctx,
		resource.WithAttributes(build.ResourceAttributes()...),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("build otel resource: %w", err)
	}

	opts := []sdktrace.TracerProviderOption{
		resourceOption(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.Telemetry.SampleRatio))),
	}

	switch cfg.Telemetry.Exporter {
	case "none":
		// No exporter: spans are created but dropped. Useful for tests.
	case "otlp":
		exp, err := otlptracegrpc.New(ctx, otlptracegrpc.WithEndpoint(cfg.Telemetry.OTLPEndpoint), otlptracegrpc.WithInsecure())
		if err != nil {
			return nil, nil, fmt.Errorf("build otlp exporter: %w", err)
		}
		opts = append(opts, sdktrace.WithBatcher(exp))
	default: // "stdout"
		exp, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
		if err != nil {
			return nil, nil, fmt.Errorf("build stdout exporter: %w", err)
		}
		opts = append(opts, sdktrace.WithBatcher(exp))
	}

	tp := sdktrace.NewTracerProvider(opts...)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	return tp, tp.Shutdown, nil
}

func resourceOption(res *resource.Resource) sdktrace.TracerProviderOption {
	return sdktrace.WithResource(res)
}
