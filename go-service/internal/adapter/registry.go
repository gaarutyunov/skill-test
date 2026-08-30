package adapter

import (
	"fmt"
	"log/slog"

	"github.com/knadh/koanf/v2"
	metricapi "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/gaarutyunov/skill-test/go-service/internal/config"
	"github.com/gaarutyunov/skill-test/go-service/internal/port"
	"github.com/gaarutyunov/skill-test/go-service/pkg/pdf"
)

// Port names — the hexagonal ports that adapters bind to. These match the keys
// under `adapters:` in the config file.
const (
	PortStudentRepository = "student_repository"
	PortReportGenerator   = "report_generator"
)

// Dependencies are the shared collaborators injected into every adapter factory.
type Dependencies struct {
	TracerProvider trace.TracerProvider
	MeterProvider  metricapi.MeterProvider
	Logger         *slog.Logger
	PDF            *pdf.Client
}

// StudentRepositoryFactory builds a StudentRepository from its settings subtree.
type StudentRepositoryFactory func(deps Dependencies, settings *koanf.Koanf) (port.StudentRepository, error)

// ReportGeneratorFactory builds a ReportGenerator from its settings subtree.
type ReportGeneratorFactory func(deps Dependencies, settings *koanf.Koanf) (port.ReportGenerator, error)

// Registry maps adapter names to their factories, per port. It is the single
// place that knows every available adapter implementation, so the config file
// can switch between them by name.
type Registry struct {
	deps                Dependencies
	studentRepositories map[string]StudentRepositoryFactory
	reportGenerators    map[string]ReportGeneratorFactory
}

// NewRegistry returns a registry pre-populated with the built-in adapters.
func NewRegistry(deps Dependencies) *Registry {
	r := &Registry{
		deps:                deps,
		studentRepositories: map[string]StudentRepositoryFactory{},
		reportGenerators:    map[string]ReportGeneratorFactory{},
	}
	r.RegisterStudentRepository("http", httpStudentRepositoryFactory)
	r.RegisterReportGenerator("pdf", pdfReportGeneratorFactory)
	return r
}

// RegisterStudentRepository registers a named StudentRepository adapter.
func (r *Registry) RegisterStudentRepository(name string, f StudentRepositoryFactory) {
	r.studentRepositories[name] = f
}

// RegisterReportGenerator registers a named ReportGenerator adapter.
func (r *Registry) RegisterReportGenerator(name string, f ReportGeneratorFactory) {
	r.reportGenerators[name] = f
}

// BuildStudentRepository constructs the StudentRepository adapter named by the
// config binding for the student_repository port.
func (r *Registry) BuildStudentRepository(cfg *config.Loaded) (port.StudentRepository, error) {
	settings, name, err := config.AdapterSettings(cfg, PortStudentRepository)
	if err != nil {
		return nil, err
	}
	f, ok := r.studentRepositories[name]
	if !ok {
		return nil, fmt.Errorf("unknown %s adapter %q", PortStudentRepository, name)
	}
	repo, err := f(r.deps, settings)
	if err != nil {
		return nil, fmt.Errorf("build %s adapter %q: %w", PortStudentRepository, name, err)
	}
	return repo, nil
}

// BuildReportGenerator constructs the ReportGenerator adapter named by the
// config binding for the report_generator port.
func (r *Registry) BuildReportGenerator(cfg *config.Loaded) (port.ReportGenerator, error) {
	settings, name, err := config.AdapterSettings(cfg, PortReportGenerator)
	if err != nil {
		return nil, err
	}
	f, ok := r.reportGenerators[name]
	if !ok {
		return nil, fmt.Errorf("unknown %s adapter %q", PortReportGenerator, name)
	}
	gen, err := f(r.deps, settings)
	if err != nil {
		return nil, fmt.Errorf("build %s adapter %q: %w", PortReportGenerator, name, err)
	}
	return gen, nil
}

func httpStudentRepositoryFactory(deps Dependencies, settings *koanf.Koanf) (port.StudentRepository, error) {
	cfg := HTTPStudentRepositoryConfig{}
	if settings != nil {
		if err := config.Unmarshal(settings, "", &cfg); err != nil {
			return nil, fmt.Errorf("decode http settings: %w", err)
		}
	}
	return NewHTTPStudentRepository(cfg, deps.TracerProvider, deps.MeterProvider, deps.Logger)
}

func pdfReportGeneratorFactory(deps Dependencies, settings *koanf.Koanf) (port.ReportGenerator, error) {
	cfg := PDFReportGeneratorConfig{}
	if settings != nil {
		if err := config.Unmarshal(settings, "", &cfg); err != nil {
			return nil, fmt.Errorf("decode pdf settings: %w", err)
		}
	}
	return NewPDFReportGenerator(cfg, deps.PDF, deps.TracerProvider), nil
}
