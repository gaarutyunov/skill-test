// Package api provides shared construction helpers for the generated API
// clients in ./upstream and ./report.
package api

import (
	"net/http"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
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

// NewHTTPClient builds an *http.Client with configurable retries (via
// hashicorp/go-retryablehttp) and OpenTelemetry outbound instrumentation. The
// returned client satisfies the generated clients' HttpRequestDoer interface.
func NewHTTPClient(cfg RetryConfig, tp trace.TracerProvider) *http.Client {
	rc := retryablehttp.NewClient()
	rc.RetryMax = cfg.MaxRetries
	if cfg.WaitMin > 0 {
		rc.RetryWaitMin = cfg.WaitMin
	}
	if cfg.WaitMax > 0 {
		rc.RetryWaitMax = cfg.WaitMax
	}
	// Silence retryablehttp's default stdlib logger; tracing covers observability.
	rc.Logger = nil
	// Trace each underlying HTTP attempt.
	rc.HTTPClient.Transport = otelhttp.NewTransport(
		http.DefaultTransport,
		otelhttp.WithTracerProvider(tp),
	)
	return rc.StandardClient()
}
