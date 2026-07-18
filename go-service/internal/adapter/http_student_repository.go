package adapter

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/codes"
	metricapi "go.opentelemetry.io/otel/metric"
	otelsemconv "go.opentelemetry.io/otel/semconv/v1.41.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/gaarutyunov/skill-test/go-service/internal/domain"
	"github.com/gaarutyunov/skill-test/go-service/pkg/api"
	"github.com/gaarutyunov/skill-test/go-service/pkg/api/upstream"
	"github.com/gaarutyunov/skill-test/go-service/pkg/semconv"
)

// HTTPStudentRepositoryConfig configures the upstream-backed student repository.
type HTTPStudentRepositoryConfig struct {
	BaseURL string `koanf:"base_url"`
	// UsernameEnv / PasswordEnv name the environment variables the upstream
	// credentials are read from. Credentials are NEVER stored in config files.
	UsernameEnv string          `koanf:"username_env"`
	PasswordEnv string          `koanf:"password_env"`
	Retry       api.RetryConfig `koanf:"retry"`
	// SessionTTL bounds how long a login session is reused before re-authenticating.
	SessionTTL time.Duration `koanf:"session_ttl"`

	// username/password are resolved from the environment at construction time;
	// they are never populated from the config file.
	username string
	password string
}

// HTTPStudentRepository fetches students from the Node.js backend over HTTP.
// It authenticates via /auth/login and forwards the resulting session cookies
// plus the CSRF header on each request. It never touches the database.
//
// A background goroutine (started via Start) keeps the session fresh: it logs in
// once at startup, proactively re-authenticates before the TTL elapses, and
// performs an ad-hoc session upgrade whenever the upstream rejects a request.
type HTTPStudentRepository struct {
	client *upstream.ClientWithResponses
	cfg    HTTPStudentRepositoryConfig
	tracer trace.Tracer
	logger *slog.Logger

	// loginMu serialises logins so concurrent callers don't stampede the
	// upstream with parallel authentications.
	loginMu sync.Mutex

	mu      sync.RWMutex
	session *session

	// ready latches to true after the FIRST successful login and never flips
	// back — later refresh failures must not make the service un-ready.
	ready atomic.Bool
	// refresh signals the background goroutine to perform an ad-hoc upgrade.
	refresh chan struct{}
}

type session struct {
	accessToken  string
	refreshToken string
	csrfToken    string
	expiresAt    time.Time
}

// NewHTTPStudentRepository builds the repository with a retrying, instrumented
// HTTP client. Upstream credentials are read from the environment variables
// named by the config (username_env / password_env).
func NewHTTPStudentRepository(cfg HTTPStudentRepositoryConfig, tp trace.TracerProvider, mp metricapi.MeterProvider, logger *slog.Logger) (*HTTPStudentRepository, error) {
	if cfg.BaseURL == "" {
		return nil, errors.New("http student repository: base_url is required")
	}
	if cfg.UsernameEnv == "" || cfg.PasswordEnv == "" {
		return nil, errors.New("http student repository: username_env and password_env are required")
	}
	cfg.username = os.Getenv(cfg.UsernameEnv)
	cfg.password = os.Getenv(cfg.PasswordEnv)
	if cfg.username == "" || cfg.password == "" {
		return nil, fmt.Errorf("http student repository: credentials env %s/%s are not set", cfg.UsernameEnv, cfg.PasswordEnv)
	}
	if cfg.SessionTTL <= 0 {
		cfg.SessionTTL = 10 * time.Minute
	}
	if logger == nil {
		logger = slog.Default()
	}
	doer := api.NewHTTPClient(cfg.Retry, tp, mp, logger)
	client, err := upstream.NewClientWithResponses(cfg.BaseURL, upstream.WithHTTPClient(doer))
	if err != nil {
		return nil, fmt.Errorf("http student repository: new client: %w", err)
	}
	return &HTTPStudentRepository{
		client:  client,
		cfg:     cfg,
		tracer:  tp.Tracer("adapter/http_student_repository"),
		logger:  logger.With("component", "http-student-repository"),
		refresh: make(chan struct{}, 1),
	}, nil
}

// Start launches the background session refresher. It returns immediately; the
// goroutine stops when ctx is cancelled.
func (r *HTTPStudentRepository) Start(ctx context.Context) {
	go r.run(ctx)
}

// Ready reports whether the upstream session has been established at least once.
// It latches to ready after the first successful login and never reverts, so a
// transient failure during a later refresh does not take the service offline.
func (r *HTTPStudentRepository) Ready(context.Context) error {
	if r.ready.Load() {
		return nil
	}
	return errors.New("upstream session not yet established")
}

func (r *HTTPStudentRepository) run(ctx context.Context) {
	// Establish the first session as soon as possible so readiness flips.
	if _, err := r.login(ctx, true); err != nil {
		r.logger.WarnContext(ctx, "initial upstream login failed", "err", err)
	}

	ticker := time.NewTicker(r.refreshInterval())
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := r.login(ctx, true); err != nil {
				r.logger.WarnContext(ctx, "scheduled session refresh failed", "err", err)
			}
		case <-r.refresh:
			if _, err := r.login(ctx, true); err != nil {
				r.logger.WarnContext(ctx, "ad-hoc session refresh failed", "err", err)
			}
			ticker.Reset(r.refreshInterval())
		}
	}
}

// refreshInterval schedules a proactive refresh before the TTL elapses.
func (r *HTTPStudentRepository) refreshInterval() time.Duration {
	iv := r.cfg.SessionTTL * 3 / 4
	if iv <= 0 {
		iv = r.cfg.SessionTTL
	}
	return iv
}

// triggerRefresh asks the background goroutine to perform an ad-hoc session
// upgrade. It never blocks: a pending request already covers this one.
func (r *HTTPStudentRepository) triggerRefresh() {
	select {
	case r.refresh <- struct{}{}:
	default:
	}
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
		// Session was rejected upstream; ask the background refresher to upgrade
		// it so subsequent requests recover.
		r.triggerRefresh()
		return nil, fmt.Errorf("%w: upstream status %d", domain.ErrUnauthorized, resp.StatusCode())
	default:
		return nil, fmt.Errorf("%w: upstream status %d", domain.ErrUpstreamUnavailable, resp.StatusCode())
	}
}

// ensureSession returns the cached session, logging in synchronously as a
// fallback if the background goroutine has not produced one yet (or it expired).
func (r *HTTPStudentRepository) ensureSession(ctx context.Context) (*session, error) {
	r.mu.RLock()
	sess := r.session
	r.mu.RUnlock()
	if sess != nil && time.Now().Before(sess.expiresAt) {
		return sess, nil
	}
	return r.login(ctx, false)
}

// login authenticates against the upstream and caches the resulting session.
// When force is false it is a no-op if a valid session already exists (another
// goroutine may have refreshed while this one waited on loginMu).
func (r *HTTPStudentRepository) login(ctx context.Context, force bool) (*session, error) {
	r.loginMu.Lock()
	defer r.loginMu.Unlock()

	if !force {
		r.mu.RLock()
		current := r.session
		r.mu.RUnlock()
		if current != nil && time.Now().Before(current.expiresAt) {
			return current, nil
		}
	}

	ctx, span := r.tracer.Start(ctx, "HTTPStudentRepository.login")
	defer span.End()
	span.SetAttributes(semconv.UpstreamOperation("login"))

	resp, err := r.client.LoginWithResponse(ctx, upstream.LoginJSONRequestBody{
		Username: r.cfg.username,
		Password: r.cfg.password,
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

	r.mu.Lock()
	r.session = sess
	r.mu.Unlock()
	// Latch readiness on the first successful login; never revert.
	r.ready.Store(true)
	return sess, nil
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
