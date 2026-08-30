package adapter_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/gaarutyunov/skill-test/go-service/internal/adapter"
	"github.com/gaarutyunov/skill-test/go-service/internal/domain"
	"github.com/gaarutyunov/skill-test/go-service/pkg/pdf"
)

func TestPDFReportGenerator_Generate(t *testing.T) {
	// Arrange
	tp := noop.NewTracerProvider()
	gen := adapter.NewPDFReportGenerator(adapter.PDFReportGeneratorConfig{
		Title:        "Student Report",
		Organization: "Greenwood High School",
	}, pdf.NewClient(tp), tp)

	student := &domain.Student{
		ID: 42, Name: "Ada Lovelace", Email: "ada@example.com",
		Class: "Grade 10", Section: "A", Roll: 12, Gender: "Female",
		FatherName: "Byron", CurrentAddress: "1 Analytical Way",
	}

	// Act
	report, err := gen.Generate(context.Background(), student)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, int64(42), report.StudentID)
	assert.Equal(t, "student-42-report.pdf", report.FileName)
	assert.Equal(t, "application/pdf", report.ContentType)
	assert.GreaterOrEqual(t, report.PageCount, 1)
	require.NotEmpty(t, report.Content)
	assert.Equal(t, "%PDF", string(report.Content[:4]), "must be a PDF document")
}
