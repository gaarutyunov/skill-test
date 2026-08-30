// Package app wires the application together (composition root) and runs it.
package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	metricapi "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/gaarutyunov/skill-test/go-service/internal/adapter"
	"github.com/gaarutyunov/skill-test/go-service/internal/config"
	"github.com/gaarutyunov/skill-test/go-service/internal/port"
	"github.com/gaarutyunov/skill-test/go-service/internal/usecase"
	"github.com/gaarutyunov/skill-test/go-service/pkg/pdf"
)

// BackgroundStarter is an optional capability for components that run a
// background goroutine bound to the application lifecycle (e.g. the upstream
// session refresher).
type BackgroundStarter interface {
	Start(ctx context.Context)
}

// App is the composed application: an HTTP server, its configuration and the
// background components started alongside it.
type App struct {
	Server   *http.Server
	Config   *config.Config
	Starters []BackgroundStarter
}

// Run starts the background components and the HTTP server, then blocks until
// the context is cancelled (an OS signal handled by the root command) and shuts
// down gracefully.
func (a *App) Run(ctx context.Context) error {
	// Start background components (e.g. the session refresher) so the service can
	// become ready before it starts accepting the first requests.
	for _, s := range a.Starters {
		s.Start(ctx)
	}

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

// ProvideLoadedConfig loads the configuration through goga/config.
//
// It is a thin wrapper over config.Load, which holds the one generic
// instantiation the project has to write itself: config.Load[Config] cannot be
// a wire provider, because wire generates code from concrete types and cannot
// name a type argument. Everything below this line is wired normally.
func ProvideLoadedConfig(ctx context.Context, path string) (*config.Loaded, error) {
	return config.Load(ctx, path)
}

// ProvideConfig exposes the decoded value, so that every component that only
// needs the typed configuration keeps depending on *config.Config rather than on
// the loader. The merged koanf handle stays available to the adapter registry,
// which cuts a subtree per adapter.
func ProvideConfig(l *config.Loaded) *config.Config {
	return &l.Value
}

// ProvideLogger returns the structured logger shared across the application.
func ProvideLogger() *slog.Logger {
	return slog.Default()
}

// ProvideTelemetry builds the OTel providers (traces + metrics) with a
// wire-style cleanup that flushes and shuts them down.
func ProvideTelemetry(ctx context.Context, cfg *config.Config) (*Telemetry, func(), error) {
	t, err := NewTelemetry(ctx, cfg)
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() {
		_ = t.Shutdown(context.Background())
	}
	return t, cleanup, nil
}

// ProvideTracerProvider exposes the tracer provider from the telemetry bundle.
func ProvideTracerProvider(t *Telemetry) trace.TracerProvider { return t.TracerProvider }

// ProvideMeterProvider exposes the meter provider from the telemetry bundle.
func ProvideMeterProvider(t *Telemetry) metricapi.MeterProvider { return t.MeterProvider }

// ProvideMetricsHandler exposes the Prometheus scrape handler.
func ProvideMetricsHandler(t *Telemetry) MetricsHandler { return t.MetricsHandler }

// ProvidePDFClient builds the instrumented PDF client.
func ProvidePDFClient(tp trace.TracerProvider) *pdf.Client {
	return pdf.NewClient(tp)
}

// ProvideDependencies assembles the shared adapter dependencies.
func ProvideDependencies(tp trace.TracerProvider, mp metricapi.MeterProvider, logger *slog.Logger, pdfClient *pdf.Client) adapter.Dependencies {
	return adapter.Dependencies{TracerProvider: tp, MeterProvider: mp, Logger: logger, PDF: pdfClient}
}

// ProvideRegistry builds the adapter registry.
func ProvideRegistry(deps adapter.Dependencies) *adapter.Registry {
	return adapter.NewRegistry(deps)
}

// ProvideStudentRepository builds the configured StudentRepository adapter.
func ProvideStudentRepository(reg *adapter.Registry, cfg *config.Loaded) (port.StudentRepository, error) {
	return reg.BuildStudentRepository(cfg)
}

// ProvideReportGenerator builds the configured ReportGenerator adapter.
func ProvideReportGenerator(reg *adapter.Registry, cfg *config.Loaded) (port.ReportGenerator, error) {
	return reg.BuildReportGenerator(cfg)
}

// ProvideGenerateReport builds the GenerateReport use case.
func ProvideGenerateReport(repo port.StudentRepository, gen port.ReportGenerator, tp trace.TracerProvider) *usecase.GenerateReport {
	return usecase.NewGenerateReport(repo, gen, tp)
}

// ProvideStarters collects the background components that must run for the
// application's lifetime. Adapters opt in by implementing BackgroundStarter.
func ProvideStarters(repo port.StudentRepository) []BackgroundStarter {
	var starters []BackgroundStarter
	if s, ok := repo.(BackgroundStarter); ok {
		starters = append(starters, s)
	}
	return starters
}

// ProvideReadinessCheckers collects the readiness checks surfaced by /readyz.
// Adapters opt in by implementing ReadinessChecker.
func ProvideReadinessCheckers(repo port.StudentRepository) []ReadinessChecker {
	var checkers []ReadinessChecker
	if c, ok := repo.(ReadinessChecker); ok {
		checkers = append(checkers, c)
	}
	return checkers
}

// ProvideApp assembles the App from the HTTP server, config and starters.
func ProvideApp(server *http.Server, cfg *config.Config, starters []BackgroundStarter) *App {
	return &App{Server: server, Config: cfg, Starters: starters}
}
