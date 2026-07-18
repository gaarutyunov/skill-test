package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	metricapi "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/gaarutyunov/skill-test/go-service/internal/config"
	"github.com/gaarutyunov/skill-test/go-service/internal/server"
	"github.com/gaarutyunov/skill-test/go-service/internal/usecase"
)

// ReadinessChecker reports whether a dependency is ready to serve traffic. It
// backs the /readyz probe. A nil error means ready.
type ReadinessChecker interface {
	Ready(ctx context.Context) error
}

// NewHTTPHandler builds the fully instrumented http.Handler for the service:
// generated strict server -> generated router -> otelhttp middleware for the API
// routes, plus operational endpoints (Kubernetes health probes and a Prometheus
// scrape endpoint) that are intentionally served outside the otel wrapper so
// probes and scrapes don't pollute request traces/metrics.
func NewHTTPHandler(generate *usecase.GenerateReport, tp trace.TracerProvider, mp metricapi.MeterProvider, metrics MetricsHandler, readiness []ReadinessChecker) http.Handler {
	strict := server.NewStrictHandler(server.NewReportHandler(generate), nil)

	// API mux: generated routes only.
	apiMux := http.NewServeMux()
	apiHandler := server.HandlerWithOptions(strict, server.StdHTTPServerOptions{
		BaseRouter: apiMux,
		ErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, err error) {
			writeJSONError(w, http.StatusBadRequest, err.Error())
		},
	})

	// Wrap the API with OpenTelemetry HTTP server instrumentation. It emits the
	// official http.*/url.*/server.*/network.* span attributes on the tracer
	// provider and http.server.* metrics on the meter provider.
	instrumentedAPI := otelhttp.NewHandler(apiHandler, "report-api",
		otelhttp.WithTracerProvider(tp),
		otelhttp.WithMeterProvider(mp),
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			return fmt.Sprintf("%s %s", r.Method, r.URL.Path)
		}),
	)

	// Root mux: operational endpoints first, everything else to the API.
	root := http.NewServeMux()

	// Kubernetes health-check conventions:
	// https://kubernetes.io/docs/reference/using-api/health-checks/
	//   /livez  — liveness: the process is up and the event loop is responsive.
	//   /readyz — readiness: every dependency is ready to serve traffic (here,
	//             the upstream session has been established at least once).
	root.HandleFunc("GET /livez", writeStatus("ok"))
	root.HandleFunc("GET /readyz", readyzHandler(readiness))
	// /healthz is retained as a backwards-compatible liveness alias.
	root.HandleFunc("GET /healthz", writeStatus("ok"))

	// Prometheus scrape endpoint, bound to the otel MeterProvider's metrics.
	root.Handle("GET /metrics", metrics)

	root.Handle("/", instrumentedAPI)
	return root
}

// readyzHandler runs every readiness check and returns 503 until they all pass.
func readyzHandler(checkers []ReadinessChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		for _, c := range checkers {
			if err := c.Ready(req.Context()); err != nil {
				writeJSONError(w, http.StatusServiceUnavailable, err.Error())
				return
			}
		}
		writeStatus("ok")(w, req)
	}
}

func writeStatus(status string) http.HandlerFunc {
	body := []byte(fmt.Sprintf(`{"status":%q}`, status))
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// NewHTTPServer builds the *http.Server bound to the configured address.
func NewHTTPServer(cfg *config.Config, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      handler,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}
}
