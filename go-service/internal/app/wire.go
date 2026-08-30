//go:build wireinject
// +build wireinject

package app

import (
	"context"

	"github.com/goforj/wire"
)

// providerSet is the full dependency graph of the application.
var providerSet = wire.NewSet(
	ProvideLoadedConfig,
	ProvideConfig,
	ProvideLogger,
	ProvideTelemetry,
	ProvideTracerProvider,
	ProvideMeterProvider,
	ProvideMetricsHandler,
	ProvidePDFClient,
	ProvideDependencies,
	ProvideRegistry,
	ProvideStudentRepository,
	ProvideReportGenerator,
	ProvideGenerateReport,
	ProvideStarters,
	ProvideReadinessCheckers,
	NewHTTPHandler,
	NewHTTPServer,
	ProvideApp,
)

// InitializeApp builds a fully wired App from a config path. The returned
// cleanup function shuts the tracer provider down.
func InitializeApp(ctx context.Context, configPath string) (*App, func(), error) {
	wire.Build(providerSet)
	return nil, nil, nil
}
