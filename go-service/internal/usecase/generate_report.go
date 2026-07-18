// Package usecase contains the application's business use cases.
package usecase

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/gaarutyunov/skill-test/go-service/internal/domain"
	"github.com/gaarutyunov/skill-test/go-service/internal/port"
	"github.com/gaarutyunov/skill-test/go-service/pkg/semconv"
)

// GenerateReport is the use case that fetches a student and renders a report.
type GenerateReport struct {
	students  port.StudentRepository
	generator port.ReportGenerator
	tracer    trace.Tracer
}

// NewGenerateReport constructs the GenerateReport use case.
func NewGenerateReport(students port.StudentRepository, generator port.ReportGenerator, tp trace.TracerProvider) *GenerateReport {
	return &GenerateReport{
		students:  students,
		generator: generator,
		tracer:    tp.Tracer("usecase.generate_report"),
	}
}

// Execute fetches the student with the given id and generates its report.
func (uc *GenerateReport) Execute(ctx context.Context, id int64) (*domain.Report, error) {
	ctx, span := uc.tracer.Start(ctx, "GenerateReport.Execute")
	defer span.End()
	span.SetAttributes(semconv.StudentId(id))

	student, err := uc.students.FindByID(ctx, id)
	if err != nil {
		span.SetStatus(codes.Error, "fetch student failed")
		span.RecordError(err)
		return nil, fmt.Errorf("fetch student %d: %w", id, err)
	}

	report, err := uc.generator.Generate(ctx, student)
	if err != nil {
		span.SetStatus(codes.Error, "generate report failed")
		span.RecordError(err)
		return nil, fmt.Errorf("generate report for student %d: %w", id, err)
	}

	span.SetAttributes(
		semconv.ReportSizeBytes(int64(report.Size())),
		semconv.ReportFileName(report.FileName),
	)
	return report, nil
}
