package app

import (
	"fmt"
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/trace"

	"github.com/gaarutyunov/skill-test/go-service/internal/config"
	"github.com/gaarutyunov/skill-test/go-service/internal/server"
	"github.com/gaarutyunov/skill-test/go-service/internal/usecase"
)

// NewHTTPHandler builds the fully instrumented http.Handler for the report API:
// generated strict server -> generated router -> otelhttp middleware, plus a
// health endpoint.
func NewHTTPHandler(generate *usecase.GenerateReport, tp trace.TracerProvider) http.Handler {
	strict := server.NewStrictHandler(server.NewReportHandler(generate), nil)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	// Mount the generated API routes onto the mux.
	apiHandler := server.HandlerFromMux(strict, mux)

	// Wrap everything with OpenTelemetry HTTP server instrumentation. It emits
	// the official http.*/url.*/server.*/network.* span attributes.
	return otelhttp.NewHandler(apiHandler, "report-api",
		otelhttp.WithTracerProvider(tp),
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			return fmt.Sprintf("%s %s", r.Method, r.URL.Path)
		}),
	)
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
