package adapter

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/gaarutyunov/skill-test/go-service/internal/domain"
	"github.com/gaarutyunov/skill-test/go-service/pkg/pdf"
	"github.com/gaarutyunov/skill-test/go-service/pkg/semconv"
)

// PDFReportGeneratorConfig configures the PDF report layout.
type PDFReportGeneratorConfig struct {
	Title        string `koanf:"title"`
	Organization string `koanf:"organization"`
}

// PDFReportGenerator renders a student into a PDF report. It owns the report
// layout; the low-level PDF mechanics live in pkg/pdf.
type PDFReportGenerator struct {
	pdf    *pdf.Client
	cfg    PDFReportGeneratorConfig
	tracer trace.Tracer
}

// NewPDFReportGenerator builds the PDF report generator.
func NewPDFReportGenerator(cfg PDFReportGeneratorConfig, pdfClient *pdf.Client, tp trace.TracerProvider) *PDFReportGenerator {
	if cfg.Title == "" {
		cfg.Title = "Student Report"
	}
	if cfg.Organization == "" {
		cfg.Organization = "School Management System"
	}
	return &PDFReportGenerator{
		pdf:    pdfClient,
		cfg:    cfg,
		tracer: tp.Tracer("adapter/pdf_report_generator"),
	}
}

// Generate renders the student into a PDF report artifact.
func (g *PDFReportGenerator) Generate(ctx context.Context, student *domain.Student) (*domain.Report, error) {
	ctx, span := g.tracer.Start(ctx, "PDFReportGenerator.Generate")
	defer span.End()
	span.SetAttributes(
		semconv.AdapterPort("report_generator"),
		semconv.AdapterName("pdf"),
		semconv.StudentId(student.ID),
		semconv.ReportFormat("pdf"),
	)

	doc := g.pdf.NewDocument(ctx)
	doc.Title(g.cfg.Organization).
		Subtitle(g.cfg.Title).
		Line()

	doc.SectionHeader("Student Information")
	doc.Field("Student ID", fmt.Sprintf("%d", student.ID)).
		Field("Name", student.Name).
		Field("Email", student.Email).
		Field("Class", student.Class).
		Field("Section", student.Section).
		Field("Roll", rollString(student.Roll)).
		Field("Gender", student.Gender).
		Field("Date of Birth", student.DateOfBirth).
		Field("Phone", student.Phone).
		Field("System Access", boolString(student.SystemAccess))

	doc.SectionHeader("Guardian Information")
	doc.Field("Father", withPhone(student.FatherName, student.FatherPhone)).
		Field("Mother", withPhone(student.MotherName, student.MotherPhone)).
		Field("Guardian", withPhone(student.GuardianName, student.GuardianPhone)).
		Field("Relation of Guardian", student.RelationOfGuardian)

	doc.SectionHeader("Address")
	doc.Field("Current Address", student.CurrentAddress).
		Field("Permanent Address", student.PermanentAddress)

	doc.SectionHeader("Administrative")
	doc.Field("Admission Date", student.AdmissionDate).
		Field("Reporter", student.ReporterName)

	doc.Line()
	doc.Subtitle(fmt.Sprintf("Generated on %s", time.Now().UTC().Format(time.RFC1123)))

	content, err := doc.Render(ctx)
	if err != nil {
		span.SetStatus(codes.Error, "render failed")
		span.RecordError(err)
		return nil, fmt.Errorf("render report: %w", err)
	}

	report := &domain.Report{
		StudentID:   student.ID,
		StudentName: student.Name,
		FileName:    fmt.Sprintf("student-%d-report.pdf", student.ID),
		ContentType: "application/pdf",
		Content:     content,
		PageCount:   doc.PageCount(),
	}
	span.SetAttributes(
		semconv.ReportSizeBytes(int64(report.Size())),
		semconv.ReportFileName(report.FileName),
		semconv.ReportPageCount(int64(report.PageCount)),
	)
	return report, nil
}

func rollString(roll int64) string {
	if roll == 0 {
		return ""
	}
	return fmt.Sprintf("%d", roll)
}

func boolString(b bool) string {
	if b {
		return "Enabled"
	}
	return "Disabled"
}

func withPhone(name, phone string) string {
	switch {
	case name == "" && phone == "":
		return ""
	case phone == "":
		return name
	case name == "":
		return phone
	default:
		return fmt.Sprintf("%s (%s)", name, phone)
	}
}
