// Package server contains the generated report API server (server.gen.go) and
// the hand-written strict handler that binds it to the use case.
package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/gaarutyunov/skill-test/go-service/internal/domain"
	"github.com/gaarutyunov/skill-test/go-service/internal/usecase"
)

// ReportHandler implements the generated StrictServerInterface by delegating to
// the GenerateReport use case and mapping domain errors to HTTP responses.
type ReportHandler struct {
	generate *usecase.GenerateReport
}

// NewReportHandler builds the report handler.
func NewReportHandler(generate *usecase.GenerateReport) *ReportHandler {
	return &ReportHandler{generate: generate}
}

// GetStudentReport handles GET /api/v1/students/{id}/report.
func (h *ReportHandler) GetStudentReport(ctx context.Context, request GetStudentReportRequestObject) (GetStudentReportResponseObject, error) {
	report, err := h.generate.Execute(ctx, request.Id)
	if err != nil {
		return mapError(err), nil
	}

	disposition := fmt.Sprintf("attachment; filename=%q", report.FileName)
	return GetStudentReport200ApplicationpdfResponse{
		Body:          bytes.NewReader(report.Content),
		ContentLength: int64(report.Size()),
		Headers: GetStudentReport200ResponseHeaders{
			ContentDisposition: &disposition,
		},
	}, nil
}

// mapError translates a use-case error into the appropriate HTTP response.
func mapError(err error) GetStudentReportResponseObject {
	switch {
	case errors.Is(err, domain.ErrStudentNotFound):
		return GetStudentReport404JSONResponse{NotFoundJSONResponse{Error: "student not found"}}
	case errors.Is(err, domain.ErrUnauthorized):
		return GetStudentReport401JSONResponse{UnauthorizedJSONResponse{Error: "upstream authentication failed"}}
	case errors.Is(err, domain.ErrUpstreamUnavailable):
		return GetStudentReport502JSONResponse{BadGatewayJSONResponse{Error: "upstream backend unavailable"}}
	default:
		return GetStudentReport500JSONResponse{InternalErrorJSONResponse{Error: "failed to generate report"}}
	}
}
