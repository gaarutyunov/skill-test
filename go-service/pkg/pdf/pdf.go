// Package pdf is a light, telemetry-instrumented wrapper around a PDF
// generation library. It exposes generic layout primitives only and holds no
// knowledge of reports, students, or any business concept — callers compose the
// document. Every rendering operation is wrapped in an OpenTelemetry span.
package pdf

import (
	"bytes"
	"context"
	"fmt"

	"github.com/go-pdf/fpdf"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/gaarutyunov/skill-test/go-service/pkg/semconv"
)

// Client builds instrumented PDF documents.
type Client struct {
	tracer trace.Tracer
}

// NewClient returns a PDF client that records spans on the given provider.
func NewClient(tp trace.TracerProvider) *Client {
	return &Client{tracer: tp.Tracer("pkg/pdf")}
}

// Document is a thin, chainable builder over an fpdf document.
type Document struct {
	fpdf   *fpdf.Fpdf
	tracer trace.Tracer
}

// NewDocument starts a new A4 portrait document.
func (c *Client) NewDocument(ctx context.Context) *Document {
	_, span := c.tracer.Start(ctx, "pdf.new_document")
	defer span.End()

	f := fpdf.New("P", "mm", "A4", "")
	f.SetMargins(20, 20, 20)
	f.AddPage()
	return &Document{fpdf: f, tracer: c.tracer}
}

// Title writes a large centered title.
func (d *Document) Title(text string) *Document {
	d.fpdf.SetFont("Helvetica", "B", 20)
	d.fpdf.CellFormat(0, 12, tr(d.fpdf, text), "", 1, "C", false, 0, "")
	d.fpdf.Ln(4)
	return d
}

// Subtitle writes a smaller centered subtitle.
func (d *Document) Subtitle(text string) *Document {
	d.fpdf.SetFont("Helvetica", "I", 11)
	d.fpdf.SetTextColor(90, 90, 90)
	d.fpdf.CellFormat(0, 8, tr(d.fpdf, text), "", 1, "C", false, 0, "")
	d.fpdf.SetTextColor(0, 0, 0)
	d.fpdf.Ln(4)
	return d
}

// SectionHeader writes a bold section heading with an underline.
func (d *Document) SectionHeader(text string) *Document {
	d.fpdf.Ln(2)
	d.fpdf.SetFont("Helvetica", "B", 13)
	d.fpdf.SetFillColor(230, 236, 245)
	d.fpdf.CellFormat(0, 9, tr(d.fpdf, text), "", 1, "L", true, 0, "")
	d.fpdf.Ln(1)
	return d
}

// Field writes a label/value row.
func (d *Document) Field(label, value string) *Document {
	if value == "" {
		value = "-"
	}
	d.fpdf.SetFont("Helvetica", "B", 10)
	d.fpdf.CellFormat(55, 7, tr(d.fpdf, label), "", 0, "L", false, 0, "")
	d.fpdf.SetFont("Helvetica", "", 10)
	d.fpdf.MultiCell(0, 7, tr(d.fpdf, value), "", "L", false)
	return d
}

// Line draws a horizontal separator.
func (d *Document) Line() *Document {
	y := d.fpdf.GetY() + 1
	d.fpdf.SetDrawColor(200, 200, 200)
	d.fpdf.Line(20, y, 190, y)
	d.fpdf.Ln(3)
	return d
}

// PageCount returns the number of pages currently in the document.
func (d *Document) PageCount() int { return d.fpdf.PageCount() }

// Render finalizes the document and returns the encoded PDF bytes.
func (d *Document) Render(ctx context.Context) ([]byte, error) {
	_, span := d.tracer.Start(ctx, "pdf.render")
	defer span.End()

	var buf bytes.Buffer
	if err := d.fpdf.Output(&buf); err != nil {
		span.SetStatus(codes.Error, "pdf output failed")
		span.RecordError(err)
		return nil, fmt.Errorf("render pdf: %w", err)
	}
	span.SetAttributes(
		semconv.ReportSizeBytes(int64(buf.Len())),
		semconv.ReportPageCount(int64(d.fpdf.PageCount())),
	)
	return buf.Bytes(), nil
}

// tr transliterates UTF-8 text to the document's code page so that non-latin1
// runes do not corrupt the output.
func tr(f *fpdf.Fpdf, s string) string {
	return f.UnicodeTranslatorFromDescriptor("")(s)
}
