// Package app wires the application together (composition root) and runs it.
package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/knadh/koanf/v2"
	"go.opentelemetry.io/otel/trace"

	"github.com/gaarutyunov/skill-test/go-service/internal/adapter"
	"github.com/gaarutyunov/skill-test/go-service/internal/config"
	"github.com/gaarutyunov/skill-test/go-service/internal/port"
	"github.com/gaarutyunov/skill-test/go-service/internal/usecase"
	"github.com/gaarutyunov/skill-test/go-service/pkg/pdf"
)

// App is the composed application: an HTTP server plus its configuration.
type App struct {
	Server *http.Server
	Config *config.Config
}

// Run starts the HTTP server and blocks until the context is cancelled or an
// OS interrupt is received, then shuts down gracefully.
func (a *App) Run(ctx context.Context) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		slog.Info("http server listening", "addr", a.Server.Addr)
		if err := a.Server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		slog.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), a.Config.Server.ShutdownTimeout)
	defer cancel()
	return a.Server.Shutdown(shutdownCtx)
}

// --- Wire providers ---

// ProvideKoanf loads the raw koanf instance from the given config path.
func ProvideKoanf(path string) (*koanf.Koanf, error) {
	return config.LoadKoanf(path)
}

// ProvideConfig parses the typed config from the koanf instance.
func ProvideConfig(k *koanf.Koanf) (*config.Config, error) {
	return config.Parse(k)
}

// ProvideTracerProvider builds the OTel tracer provider with a wire-style cleanup.
func ProvideTracerProvider(ctx context.Context, cfg *config.Config) (trace.TracerProvider, func(), error) {
	tp, shutdown, err := NewTracerProvider(ctx, cfg)
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() {
		_ = shutdown(context.Background())
	}
	return tp, cleanup, nil
}

// ProvidePDFClient builds the instrumented PDF client.
func ProvidePDFClient(tp trace.TracerProvider) *pdf.Client {
	return pdf.NewClient(tp)
}

// ProvideDependencies assembles the shared adapter dependencies.
func ProvideDependencies(tp trace.TracerProvider, pdfClient *pdf.Client) adapter.Dependencies {
	return adapter.Dependencies{TracerProvider: tp, PDF: pdfClient}
}

// ProvideRegistry builds the adapter registry.
func ProvideRegistry(deps adapter.Dependencies) *adapter.Registry {
	return adapter.NewRegistry(deps)
}

// ProvideStudentRepository builds the configured StudentRepository adapter.
func ProvideStudentRepository(reg *adapter.Registry, cfg *config.Config, k *koanf.Koanf) (port.StudentRepository, error) {
	return reg.BuildStudentRepository(cfg, k)
}

// ProvideReportGenerator builds the configured ReportGenerator adapter.
func ProvideReportGenerator(reg *adapter.Registry, cfg *config.Config, k *koanf.Koanf) (port.ReportGenerator, error) {
	return reg.BuildReportGenerator(cfg, k)
}

// ProvideGenerateReport builds the GenerateReport use case.
func ProvideGenerateReport(repo port.StudentRepository, gen port.ReportGenerator, tp trace.TracerProvider) *usecase.GenerateReport {
	return usecase.NewGenerateReport(repo, gen, tp)
}

// ProvideApp assembles the App from the HTTP server and config.
func ProvideApp(server *http.Server, cfg *config.Config) *App {
	return &App{Server: server, Config: cfg}
}
