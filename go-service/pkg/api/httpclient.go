// Package api provides shared construction helpers for the generated API
// clients in ./upstream and ./report.
package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// RetryConfig configures the retry behaviour of the outbound HTTP client.
type RetryConfig struct {
	// MaxRetries is the maximum number of retries after the first attempt.
	MaxRetries int `koanf:"max_retries"`
	// WaitMin is the minimum backoff between retries.
	WaitMin time.Duration `koanf:"wait_min"`
	// WaitMax is the maximum backoff between retries.
	WaitMax time.Duration `koanf:"wait_max"`
}

// DefaultRetryConfig returns sensible retry defaults.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{MaxRetries: 3, WaitMin: 200 * time.Millisecond, WaitMax: 2 * time.Second}
}

// NewHTTPClient builds an *http.Client that emits all three OpenTelemetry
// signals for every outbound call:
//
//   - traces  — each attempt is a span, and the W3C `traceparent` header is
//     injected so the upstream continues the same trace;
//   - metrics — otelhttp records http.client.* duration/size histograms on the
//     provided MeterProvider;
//   - structured logs — retry attempts and errors are logged through slog
//     instead of being silenced.
//
// Retries are handled by hashicorp/go-retryablehttp. The returned client
// satisfies the generated clients' HttpRequestDoer interface.
func NewHTTPClient(cfg RetryConfig, tp trace.TracerProvider, mp metric.MeterProvider, logger *slog.Logger) *http.Client {
	if logger == nil {
		logger = slog.Default()
	}

	rc := retryablehttp.NewClient()
	rc.RetryMax = cfg.MaxRetries
	if cfg.WaitMin > 0 {
		rc.RetryWaitMin = cfg.WaitMin
	}
	if cfg.WaitMax > 0 {
		rc.RetryWaitMax = cfg.WaitMax
	}
	// Structured logs: route retryablehttp's diagnostics through slog rather than
	// silencing them, so retries/backoff are observable alongside traces.
	rc.Logger = &slogLeveledLogger{logger: logger.With("component", "http-client")}

	// Traces + metrics: instrument every underlying attempt and explicitly inject
	// the W3C trace context (traceparent/tracestate) so distributed traces span
	// the call into the upstream backend regardless of the global propagator.
	rc.HTTPClient.Transport = otelhttp.NewTransport(
		http.DefaultTransport,
		otelhttp.WithTracerProvider(tp),
		otelhttp.WithMeterProvider(mp),
		otelhttp.WithPropagators(propagation.TraceContext{}),
	)
	return rc.StandardClient()
}

// slogLeveledLogger adapts *slog.Logger to retryablehttp.LeveledLogger so the
// retrying client emits structured logs.
type slogLeveledLogger struct {
	logger *slog.Logger
}

func (l *slogLeveledLogger) Error(msg string, keysAndValues ...any) {
	l.logger.Error(msg, keysAndValues...)
}

func (l *slogLeveledLogger) Warn(msg string, keysAndValues ...any) {
	l.logger.Warn(msg, keysAndValues...)
}

func (l *slogLeveledLogger) Info(msg string, keysAndValues ...any) {
	l.logger.Info(msg, keysAndValues...)
}

func (l *slogLeveledLogger) Debug(msg string, keysAndValues ...any) {
	l.logger.Debug(msg, keysAndValues...)
}
