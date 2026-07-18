package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	metricapi "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"github.com/gaarutyunov/skill-test/go-service/internal/config"
	"github.com/gaarutyunov/skill-test/go-service/pkg/semconv"
)

// MetricsHandler is the Prometheus scrape handler exposed at /metrics. It is a
// distinct type (not a bare http.Handler) so the wire DI graph can tell it apart
// from the main API handler.
type MetricsHandler http.Handler

// Telemetry bundles the configured OpenTelemetry providers plus the Prometheus
// scrape handler that exposes the metrics registered on the MeterProvider.
type Telemetry struct {
	TracerProvider trace.TracerProvider
	MeterProvider  metricapi.MeterProvider
	MetricsHandler MetricsHandler

	shutdowns []func(context.Context) error
}

// Shutdown flushes and stops every provider, most-recently-added first.
func (t *Telemetry) Shutdown(ctx context.Context) error {
	var err error
	for i := len(t.shutdowns) - 1; i >= 0; i-- {
		err = errors.Join(err, t.shutdowns[i](ctx))
	}
	return err
}

// --- Provider builder registry ---
//
// Each telemetry backend ("stdout", "otlp", "none") registers a builder for
// every provider it supports. Selecting a backend is then a map lookup rather
// than a growing switch, and adding a new backend is a single registration.

type (
	spanExporterBuilder func(ctx context.Context, cfg *config.Config) (sdktrace.SpanExporter, error)
	metricReaderBuilder func(ctx context.Context, cfg *config.Config) (sdkmetric.Reader, error)
)

type providerRegistry struct {
	spanExporters map[string]spanExporterBuilder
	metricReaders map[string]metricReaderBuilder
}

func newProviderRegistry() *providerRegistry {
	r := &providerRegistry{
		spanExporters: map[string]spanExporterBuilder{},
		metricReaders: map[string]metricReaderBuilder{},
	}

	// "none": providers still emit signals, but nothing is exported.
	r.spanExporters["none"] = func(context.Context, *config.Config) (sdktrace.SpanExporter, error) { return nil, nil }
	r.metricReaders["none"] = func(context.Context, *config.Config) (sdkmetric.Reader, error) { return nil, nil }

	// "stdout": human-readable local development output.
	r.spanExporters["stdout"] = func(context.Context, *config.Config) (sdktrace.SpanExporter, error) {
		return stdouttrace.New(stdouttrace.WithPrettyPrint())
	}
	r.metricReaders["stdout"] = func(_ context.Context, _ *config.Config) (sdkmetric.Reader, error) {
		exp, err := stdoutmetric.New()
		if err != nil {
			return nil, err
		}
		return sdkmetric.NewPeriodicReader(exp), nil
	}

	// "otlp": push to an OTLP/gRPC collector.
	r.spanExporters["otlp"] = func(ctx context.Context, cfg *config.Config) (sdktrace.SpanExporter, error) {
		return otlptracegrpc.New(ctx,
			otlptracegrpc.WithEndpoint(cfg.Telemetry.OTLPEndpoint),
			otlptracegrpc.WithInsecure(),
		)
	}
	r.metricReaders["otlp"] = func(ctx context.Context, cfg *config.Config) (sdkmetric.Reader, error) {
		exp, err := otlpmetricgrpc.New(ctx,
			otlpmetricgrpc.WithEndpoint(cfg.Telemetry.OTLPEndpoint),
			otlpmetricgrpc.WithInsecure(),
		)
		if err != nil {
			return nil, err
		}
		return sdkmetric.NewPeriodicReader(exp), nil
	}

	return r
}

// NewTelemetry builds the TracerProvider and MeterProvider from configuration,
// registers them (and the W3C propagator) globally, and returns them together
// with a Prometheus /metrics handler. A Prometheus reader is always attached to
// the MeterProvider so /metrics works regardless of the configured push
// exporter.
func NewTelemetry(ctx context.Context, cfg *config.Config) (*Telemetry, error) {
	reg := newProviderRegistry()

	build := semconv.Build()
	res, err := resource.New(ctx, resource.WithAttributes(build.ResourceAttributes()...))
	if err != nil {
		return nil, fmt.Errorf("build otel resource: %w", err)
	}

	t := &Telemetry{}

	// --- Traces ---
	spanBuild, ok := reg.spanExporters[cfg.Telemetry.Exporter]
	if !ok {
		spanBuild = reg.spanExporters["stdout"]
	}
	spanExp, err := spanBuild(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("build %q span exporter: %w", cfg.Telemetry.Exporter, err)
	}
	traceOpts := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.Telemetry.SampleRatio))),
	}
	if spanExp != nil {
		traceOpts = append(traceOpts, sdktrace.WithBatcher(spanExp))
	}
	tp := sdktrace.NewTracerProvider(traceOpts...)
	t.TracerProvider = tp
	t.shutdowns = append(t.shutdowns, tp.Shutdown)

	// --- Metrics ---
	// A dedicated Prometheus registry backs the /metrics endpoint; the otel
	// Prometheus exporter is a MeterProvider reader that scrapes on demand.
	promReg := prometheus.NewRegistry()
	promReader, err := otelprom.New(otelprom.WithRegisterer(promReg))
	if err != nil {
		return nil, fmt.Errorf("build prometheus exporter: %w", err)
	}
	meterOpts := []sdkmetric.Option{
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(promReader),
	}
	if readerBuild, ok := reg.metricReaders[cfg.Telemetry.Exporter]; ok {
		reader, err := readerBuild(ctx, cfg)
		if err != nil {
			return nil, fmt.Errorf("build %q metric reader: %w", cfg.Telemetry.Exporter, err)
		}
		if reader != nil {
			meterOpts = append(meterOpts, sdkmetric.WithReader(reader))
		}
	}
	mp := sdkmetric.NewMeterProvider(meterOpts...)
	t.MeterProvider = mp
	t.MetricsHandler = promhttp.HandlerFor(promReg, promhttp.HandlerOpts{})
	t.shutdowns = append(t.shutdowns, mp.Shutdown)

	// Register the providers and the composite W3C propagator globally so the
	// three signals share trace context (traceparent) end to end.
	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return t, nil
}
