// Package port declares the hexagonal ports (interfaces) of the application.
// Ports are defined here, on the consumer side; adapters in internal/adapter
// implement them. Interface satisfaction is enforced at wiring time by the
// compiler — do NOT add explicit `var _ Port = (*Impl)(nil)` assertions.
package port

import (
	"context"

	"github.com/gaarutyunov/skill-test/go-service/internal/domain"
)

// StudentRepository fetches student data from a backing source (the Node.js
// backend, in production).
type StudentRepository interface {
	FindByID(ctx context.Context, id int64) (*domain.Student, error)
}

// ReportGenerator renders a student into a downloadable report artifact.
type ReportGenerator interface {
	Generate(ctx context.Context, student *domain.Student) (*domain.Report, error)
}
