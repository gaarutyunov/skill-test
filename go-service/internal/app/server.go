package app

import (
	"encoding/json"
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

	// Mount the generated API routes onto the mux. A custom error handler makes
	// request-binding failures (e.g. a non-integer id -> 400) return the same
	// JSON error envelope as every other error response.
	apiHandler := server.HandlerWithOptions(strict, server.StdHTTPServerOptions{
		BaseRouter: mux,
		ErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, err error) {
			writeJSONError(w, http.StatusBadRequest, err.Error())
		},
	})

	// Wrap everything with OpenTelemetry HTTP server instrumentation. It emits
	// the official http.*/url.*/server.*/network.* span attributes.
	return otelhttp.NewHandler(apiHandler, "report-api",
		otelhttp.WithTracerProvider(tp),
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			return fmt.Sprintf("%s %s", r.Method, r.URL.Path)
		}),
	)
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
