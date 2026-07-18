package adapter

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"go.opentelemetry.io/otel/codes"
	otelsemconv "go.opentelemetry.io/otel/semconv/v1.41.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/gaarutyunov/skill-test/go-service/internal/domain"
	"github.com/gaarutyunov/skill-test/go-service/pkg/api"
	"github.com/gaarutyunov/skill-test/go-service/pkg/api/upstream"
	"github.com/gaarutyunov/skill-test/go-service/pkg/semconv"
)

// HTTPStudentRepositoryConfig configures the upstream-backed student repository.
type HTTPStudentRepositoryConfig struct {
	BaseURL  string          `koanf:"base_url"`
	Username string          `koanf:"username"`
	Password string          `koanf:"password"`
	Retry    api.RetryConfig `koanf:"retry"`
	// SessionTTL bounds how long a login session is reused before re-authenticating.
	SessionTTL time.Duration `koanf:"session_ttl"`
}

// HTTPStudentRepository fetches students from the Node.js backend over HTTP.
// It authenticates via /auth/login and forwards the resulting session cookies
// plus the CSRF header on each request. It never touches the database.
type HTTPStudentRepository struct {
	client *upstream.ClientWithResponses
	cfg    HTTPStudentRepositoryConfig
	tracer trace.Tracer

	mu      sync.Mutex
	session *session
}

type session struct {
	accessToken  string
	refreshToken string
	csrfToken    string
	expiresAt    time.Time
}

// NewHTTPStudentRepository builds the repository with a retrying, instrumented
// HTTP client.
func NewHTTPStudentRepository(cfg HTTPStudentRepositoryConfig, tp trace.TracerProvider) (*HTTPStudentRepository, error) {
	if cfg.BaseURL == "" {
		return nil, errors.New("http student repository: base_url is required")
	}
	if cfg.SessionTTL <= 0 {
		cfg.SessionTTL = 10 * time.Minute
	}
	doer := api.NewHTTPClient(cfg.Retry, tp)
	client, err := upstream.NewClientWithResponses(cfg.BaseURL, upstream.WithHTTPClient(doer))
	if err != nil {
		return nil, fmt.Errorf("http student repository: new client: %w", err)
	}
	return &HTTPStudentRepository{
		client: client,
		cfg:    cfg,
		tracer: tp.Tracer("adapter/http_student_repository"),
	}, nil
}

// FindByID fetches the student detail for the given id from the backend.
func (r *HTTPStudentRepository) FindByID(ctx context.Context, id int64) (*domain.Student, error) {
	ctx, span := r.tracer.Start(ctx, "HTTPStudentRepository.FindByID")
	defer span.End()
	span.SetAttributes(
		semconv.AdapterPort("student_repository"),
		semconv.AdapterName("http"),
		semconv.StudentId(id),
		semconv.UpstreamOperation("getStudentDetail"),
	)

	sess, err := r.ensureSession(ctx)
	if err != nil {
		span.SetStatus(codes.Error, "authentication failed")
		span.RecordError(err)
		return nil, err
	}

	resp, err := r.client.GetStudentDetailWithResponse(ctx, id, sess.authEditor())
	if err != nil {
		span.SetStatus(codes.Error, "upstream request failed")
		span.RecordError(err)
		return nil, fmt.Errorf("%w: get student detail: %v", domain.ErrUpstreamUnavailable, err)
	}
	span.SetAttributes(otelsemconv.HTTPResponseStatusCode(resp.StatusCode()))

	switch resp.StatusCode() {
	case http.StatusOK:
		if resp.JSON200 == nil {
			return nil, fmt.Errorf("%w: empty student payload", domain.ErrUpstreamUnavailable)
		}
		return mapStudent(resp.JSON200), nil
	case http.StatusNotFound:
		return nil, fmt.Errorf("%w: id=%d", domain.ErrStudentNotFound, id)
	case http.StatusUnauthorized, http.StatusForbidden:
		// Session may have expired; drop it so the next call re-authenticates.
		r.invalidateSession()
		return nil, fmt.Errorf("%w: upstream status %d", domain.ErrUnauthorized, resp.StatusCode())
	default:
		return nil, fmt.Errorf("%w: upstream status %d", domain.ErrUpstreamUnavailable, resp.StatusCode())
	}
}

func (r *HTTPStudentRepository) ensureSession(ctx context.Context) (*session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.session != nil && time.Now().Before(r.session.expiresAt) {
		return r.session, nil
	}

	ctx, span := r.tracer.Start(ctx, "HTTPStudentRepository.login")
	defer span.End()
	span.SetAttributes(semconv.UpstreamOperation("login"))

	resp, err := r.client.LoginWithResponse(ctx, upstream.LoginJSONRequestBody{
		Username: r.cfg.Username,
		Password: r.cfg.Password,
	})
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("%w: login: %v", domain.ErrUpstreamUnavailable, err)
	}
	span.SetAttributes(otelsemconv.HTTPResponseStatusCode(resp.StatusCode()))
	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("%w: login returned status %d", domain.ErrUnauthorized, resp.StatusCode())
	}

	sess := &session{expiresAt: time.Now().Add(r.cfg.SessionTTL)}
	for _, c := range resp.HTTPResponse.Cookies() {
		switch c.Name {
		case "accessToken":
			sess.accessToken = c.Value
		case "refreshToken":
			sess.refreshToken = c.Value
		case "csrfToken":
			sess.csrfToken = c.Value
		}
	}
	if sess.accessToken == "" || sess.csrfToken == "" {
		return nil, fmt.Errorf("%w: login did not return session cookies", domain.ErrUnauthorized)
	}
	r.session = sess
	return sess, nil
}

func (r *HTTPStudentRepository) invalidateSession() {
	r.mu.Lock()
	r.session = nil
	r.mu.Unlock()
}

// authEditor injects the session cookies and CSRF header on an outgoing request.
func (s *session) authEditor() upstream.RequestEditorFn {
	return func(_ context.Context, req *http.Request) error {
		req.AddCookie(&http.Cookie{Name: "accessToken", Value: s.accessToken})
		req.AddCookie(&http.Cookie{Name: "refreshToken", Value: s.refreshToken})
		req.AddCookie(&http.Cookie{Name: "csrfToken", Value: s.csrfToken})
		req.Header.Set("X-CSRF-Token", s.csrfToken)
		return nil
	}
}

func mapStudent(d *upstream.StudentDetail) *domain.Student {
	return &domain.Student{
		ID:                 d.Id,
		Name:               d.Name,
		Email:              d.Email,
		SystemAccess:       deref(d.SystemAccess),
		Phone:              deref(d.Phone),
		Gender:             deref(d.Gender),
		DateOfBirth:        deref(d.Dob),
		Class:              deref(d.Class),
		Section:            deref(d.Section),
		Roll:               deref(d.Roll),
		FatherName:         deref(d.FatherName),
		FatherPhone:        deref(d.FatherPhone),
		MotherName:         deref(d.MotherName),
		MotherPhone:        deref(d.MotherPhone),
		GuardianName:       deref(d.GuardianName),
		GuardianPhone:      deref(d.GuardianPhone),
		RelationOfGuardian: deref(d.RelationOfGuardian),
		CurrentAddress:     deref(d.CurrentAddress),
		PermanentAddress:   deref(d.PermanentAddress),
		AdmissionDate:      deref(d.AdmissionDate),
		ReporterName:       deref(d.ReporterName),
	}
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}
