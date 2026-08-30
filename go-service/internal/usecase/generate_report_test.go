package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/mock/gomock"

	"github.com/gaarutyunov/skill-test/go-service/internal/domain"
	"github.com/gaarutyunov/skill-test/go-service/internal/port/mock"
	"github.com/gaarutyunov/skill-test/go-service/internal/usecase"
)

func TestGenerateReport_Execute_Success(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	repo := mock.NewMockStudentRepository(ctrl)
	gen := mock.NewMockReportGenerator(ctrl)

	student := &domain.Student{ID: 7, Name: "Ada Lovelace", Email: "ada@example.com"}
	want := &domain.Report{StudentID: 7, FileName: "student-7-report.pdf", ContentType: "application/pdf", Content: []byte("%PDF-1.4")}

	repo.EXPECT().FindByID(gomock.Any(), int64(7)).Return(student, nil)
	gen.EXPECT().Generate(gomock.Any(), student).Return(want, nil)

	uc := usecase.NewGenerateReport(repo, gen, noop.NewTracerProvider())

	// Act
	got, err := uc.Execute(context.Background(), 7)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestGenerateReport_Execute_StudentNotFound(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	repo := mock.NewMockStudentRepository(ctrl)
	gen := mock.NewMockReportGenerator(ctrl)

	repo.EXPECT().FindByID(gomock.Any(), int64(404)).Return(nil, domain.ErrStudentNotFound)
	// The generator must NOT be called when the student cannot be fetched.

	uc := usecase.NewGenerateReport(repo, gen, noop.NewTracerProvider())

	// Act
	_, err := uc.Execute(context.Background(), 404)

	// Assert
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrStudentNotFound)
}

func TestGenerateReport_Execute_GeneratorError(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	repo := mock.NewMockStudentRepository(ctrl)
	gen := mock.NewMockReportGenerator(ctrl)

	student := &domain.Student{ID: 1, Name: "Grace Hopper"}
	boom := errors.New("render exploded")

	repo.EXPECT().FindByID(gomock.Any(), int64(1)).Return(student, nil)
	gen.EXPECT().Generate(gomock.Any(), student).Return(nil, boom)

	uc := usecase.NewGenerateReport(repo, gen, noop.NewTracerProvider())

	// Act
	_, err := uc.Execute(context.Background(), 1)

	// Assert
	require.Error(t, err)
	assert.ErrorIs(t, err, boom)
}
