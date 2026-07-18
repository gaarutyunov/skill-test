//go:build integration

// Package integration contains the end-to-end integration suite for the report
// service. It uses NO mocks: it launches real Postgres, the real Node.js backend
// (built in Docker), and this service (built in Docker) via testcontainers, then
// exercises the report API and validates responses against the OpenAPI spec.
package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	mobynet "github.com/moby/moby/api/types/network"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"

	reportclient "github.com/gaarutyunov/skill-test/go-service/pkg/api/report"
)

// Shared state established by TestMain.
var (
	reportBaseURL string
	pgDB          *sql.DB
)

const artifactsDir = "artifacts"

func TestMain(m *testing.M) {
	ctx := context.Background()
	teardown, err := setupStack(ctx)
	if err != nil {
		log.Fatalf("integration stack setup failed: %v", err)
	}
	code := m.Run()
	teardown()
	os.Exit(code)
}

// stdoutLogConsumer streams full container logs to stdout, prefixed by name.
type stdoutLogConsumer struct{ name string }

func (c *stdoutLogConsumer) Accept(l testcontainers.Log) {
	fmt.Printf("[%s] %s", c.name, string(l.Content))
}

// logConfig returns a log-streaming config that mirrors full container output to
// the test log. It is enabled only when TC_STREAM_CONTAINER_LOGS is truthy,
// because streaming requires a Docker logging driver that supports reading logs
// (json-file/local) — some local daemons don't, and enabling it there fails
// container startup with "configured logging driver does not support reading".
// CI (json-file driver) sets the flag so full container logs are captured there.
func logConfig(name string) *testcontainers.LogConsumerConfig {
	switch os.Getenv("TC_STREAM_CONTAINER_LOGS") {
	case "1", "true", "yes":
		return &testcontainers.LogConsumerConfig{
			Consumers: []testcontainers.LogConsumer{&stdoutLogConsumer{name: name}},
		}
	default:
		return nil
	}
}

func setupStack(ctx context.Context) (func(), error) {
	var cleanups []func()
	teardown := func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i]()
		}
	}
	fail := func(err error) (func(), error) {
		teardown()
		return nil, err
	}

	log.Println("=== [ARRANGE] launching integration stack (postgres -> node backend -> report service) ===")

	net, err := network.New(ctx)
	if err != nil {
		return fail(fmt.Errorf("create network: %w", err))
	}
	cleanups = append(cleanups, func() { _ = net.Remove(ctx) })

	// --- Postgres, seeded with schema + seed data + student fixtures ---
	log.Println("--- [ARRANGE] starting postgres with schema, seed and fixtures ---")
	// Mount the init scripts with explicit numeric prefixes so Postgres runs
	// them in the right order: schema -> seed -> fixtures. (WithInitScripts
	// keeps the original basenames, which would sort fixtures before the schema.)
	orderedInit := testcontainers.CustomizeRequestOption(func(req *testcontainers.GenericContainerRequest) error {
		req.Files = append(req.Files,
			testcontainers.ContainerFile{
				HostFilePath:      filepath.Join("..", "..", "..", "seed_db", "tables.sql"),
				ContainerFilePath: "/docker-entrypoint-initdb.d/01-tables.sql",
				FileMode:          0o644,
			},
			testcontainers.ContainerFile{
				HostFilePath:      filepath.Join("..", "..", "..", "seed_db", "seed-db.sql"),
				ContainerFilePath: "/docker-entrypoint-initdb.d/02-seed-db.sql",
				FileMode:          0o644,
			},
			testcontainers.ContainerFile{
				HostFilePath:      filepath.Join("testdata", "03-fixtures.sql"),
				ContainerFilePath: "/docker-entrypoint-initdb.d/03-fixtures.sql",
				FileMode:          0o644,
			},
		)
		return nil
	})

	pg, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("school_mgmt"),
		postgres.WithUsername("postgres"),
		postgres.WithPassword("postgres"),
		orderedInit,
		network.WithNetwork([]string{"postgres"}, net),
		// Wait via a real SQL ping (not container logs) so the suite works even
		// when the Docker logging driver does not support reading logs. A
		// successful external connection also implies the init scripts ran.
		testcontainers.WithWaitStrategy(
			wait.ForSQL("5432/tcp", "pgx", func(host string, port mobynet.Port) string {
				return fmt.Sprintf("postgres://postgres:postgres@%s:%s/school_mgmt?sslmode=disable", host, port.Port())
			}).WithStartupTimeout(120*time.Second),
		),
	)
	if err != nil {
		return fail(fmt.Errorf("start postgres: %w", err))
	}
	cleanups = append(cleanups, func() { _ = pg.Terminate(ctx) })

	connStr, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return fail(fmt.Errorf("postgres conn string: %w", err))
	}
	pgDB, err = sql.Open("pgx", connStr)
	if err != nil {
		return fail(fmt.Errorf("open db: %w", err))
	}
	cleanups = append(cleanups, func() { _ = pgDB.Close() })

	// --- Node.js backend, built from ../../backend ---
	log.Println("--- [ARRANGE] building & starting the Node.js backend ---")
	backend, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			FromDockerfile: testcontainers.FromDockerfile{
				Context:       filepath.Join("..", "..", "..", "backend"),
				PrintBuildLog: true,
			},
			ExposedPorts:   []string{"5007/tcp"},
			Networks:       []string{net.Name},
			NetworkAliases: map[string][]string{net.Name: {"backend"}},
			Env:            backendEnv(),
			// HTTP wait (any response < 500 means the server is up) instead of a
			// log wait, so we don't depend on a readable Docker logging driver.
			WaitingFor: wait.ForHTTP("/").WithPort("5007/tcp").
				WithStatusCodeMatcher(func(status int) bool { return status < 500 }).
				WithStartupTimeout(240 * time.Second),
			LogConsumerCfg: logConfig("backend"),
		},
		Started: true,
	})
	if err != nil {
		return fail(fmt.Errorf("start backend: %w", err))
	}
	cleanups = append(cleanups, func() { _ = backend.Terminate(ctx) })

	// --- Report service, built from ../.. (this service) ---
	log.Println("--- [ARRANGE] building & starting the report service (docker) ---")
	service, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			FromDockerfile: testcontainers.FromDockerfile{
				Context:       filepath.Join("..", ".."),
				PrintBuildLog: true,
				BuildArgs:     map[string]*string{"VERSION": strPtr("integration")},
			},
			ExposedPorts:   []string{"8080/tcp"},
			Networks:       []string{net.Name},
			NetworkAliases: map[string][]string{net.Name: {"service"}},
			Env: map[string]string{
				"REPORT_ADAPTERS__STUDENT_REPOSITORY__HTTP__BASE_URL": "http://backend:5007",
				"REPORT_TELEMETRY__EXPORTER":                          "stdout",
				// Upstream credentials are injected via env (never config files).
				"REPORT_UPSTREAM_USERNAME": "admin@school-admin.com",
				"REPORT_UPSTREAM_PASSWORD": "3OU4zn3q6Zh9",
			},
			WaitingFor: wait.ForHTTP("/healthz").WithPort("8080/tcp").
				WithStatusCodeMatcher(func(status int) bool { return status == http.StatusOK }).
				WithStartupTimeout(120 * time.Second),
			LogConsumerCfg: logConfig("service"),
		},
		Started: true,
	})
	if err != nil {
		return fail(fmt.Errorf("start service: %w", err))
	}
	cleanups = append(cleanups, func() { _ = service.Terminate(ctx) })

	host, err := service.Host(ctx)
	if err != nil {
		return fail(fmt.Errorf("service host: %w", err))
	}
	port, err := service.MappedPort(ctx, "8080")
	if err != nil {
		return fail(fmt.Errorf("service port: %w", err))
	}
	reportBaseURL = fmt.Sprintf("http://%s:%s", host, port.Port())
	log.Printf("=== [ARRANGE] stack ready. report service at %s ===", reportBaseURL)

	if err := os.MkdirAll(artifactsDir, 0o755); err != nil {
		return fail(fmt.Errorf("mkdir artifacts: %w", err))
	}
	return teardown, nil
}

func backendEnv() map[string]string {
	return map[string]string{
		"PORT":                                "5007",
		"DATABASE_URL":                        "postgres://postgres:postgres@postgres:5432/school_mgmt",
		"JWT_ACCESS_TOKEN_SECRET":             "integration_access_secret",
		"JWT_REFRESH_TOKEN_SECRET":            "integration_refresh_secret",
		"CSRF_TOKEN_SECRET":                   "integration_csrf_secret",
		"JWT_ACCESS_TOKEN_TIME_IN_MS":         "900000",
		"JWT_REFRESH_TOKEN_TIME_IN_MS":        "28800000",
		"CSRF_TOKEN_TIME_IN_MS":               "950000",
		"EMAIL_VERIFICATION_TOKEN_SECRET":     "integration_email_secret",
		"EMAIL_VERIFICATION_TOKEN_TIME_IN_MS": "18000000",
		"PASSWORD_SETUP_TOKEN_SECRET":         "integration_pwd_secret",
		"PASSWORD_SETUP_TOKEN_TIME_IN_MS":     "300000",
		"MAIL_FROM_USER":                      "noreply@school-example.com",
		"RESEND_API_KEY":                      "re_integration_dummy",
		"UI_URL":                              "http://localhost:5173",
		"API_URL":                             "http://localhost:5007",
		"COOKIE_DOMAIN":                       "localhost",
		"NODE_ENV":                            "production",
	}
}

func strPtr(s string) *string { return &s }

// studentIDByEmail looks up a seeded student's id directly in Postgres.
func studentIDByEmail(t *testing.T, email string) int64 {
	t.Helper()
	var id int64
	err := pgDB.QueryRowContext(context.Background(),
		"SELECT id FROM users WHERE email = $1", email).Scan(&id)
	require.NoError(t, err, "look up student id for %s", email)
	return id
}

func TestGenerateReport_Success(t *testing.T) {
	client, err := reportclient.NewClient(reportBaseURL)
	require.NoError(t, err)

	cases := []struct {
		name     string
		email    string
		artifact string
	}{
		{name: "full profile", email: "alice.johnson@school-example.com", artifact: "student-alice-report.pdf"},
		{name: "minimal profile", email: "bob.smith@school-example.com", artifact: "student-bob-report.pdf"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			id := studentIDByEmail(t, tc.email)
			t.Logf("[ARRANGE] student %q has id=%d", tc.email, id)

			// Act
			t.Logf("[ACT] GET %s/api/v1/students/%d/report", reportBaseURL, id)
			resp, err := client.GetStudentReport(context.Background(), id)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			// Assert
			t.Logf("[ASSERT] status=%d content-type=%s size=%d", resp.StatusCode, resp.Header.Get("Content-Type"), len(body))
			assert.Equal(t, http.StatusOK, resp.StatusCode)
			assert.Equal(t, "application/pdf", resp.Header.Get("Content-Type"))
			assert.Contains(t, resp.Header.Get("Content-Disposition"), "attachment")
			require.NotEmpty(t, body)
			assert.True(t, len(body) >= 4 && string(body[:4]) == "%PDF", "response body must be a PDF")

			// Persist the PDF as a test artifact for different use cases.
			path := filepath.Join(artifactsDir, tc.artifact)
			require.NoError(t, os.WriteFile(path, body, 0o644))
			t.Logf("[ASSERT] wrote artifact %s (%d bytes)", path, len(body))
		})
	}
}

func TestGenerateReport_NotFound(t *testing.T) {
	client, err := reportclient.NewClient(reportBaseURL)
	require.NoError(t, err)

	// Arrange: an id that does not exist.
	const missingID = int64(999999)

	// Act
	t.Logf("[ACT] GET report for missing id=%d", missingID)
	resp, err := client.GetStudentReport(context.Background(), missingID)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	// Assert
	t.Logf("[ASSERT] status=%d body=%s", resp.StatusCode, string(body))
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assertValidErrorEnvelope(t, body)
}

func TestGenerateReport_BadRequest(t *testing.T) {
	// Arrange: a non-integer id must be rejected at request binding.
	url := reportBaseURL + "/api/v1/students/not-a-number/report"

	// Act
	t.Logf("[ACT] GET %s", url)
	resp, err := http.Get(url)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	// Assert
	t.Logf("[ASSERT] status=%d body=%s", resp.StatusCode, string(body))
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assertValidErrorEnvelope(t, body)
}

func TestHealthz(t *testing.T) {
	resp, err := http.Get(reportBaseURL + "/healthz")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// assertValidErrorEnvelope validates the body against the Error schema defined
// in api/openapi.yaml, using go-openapi/validate.
func assertValidErrorEnvelope(t *testing.T, body []byte) {
	t.Helper()
	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload), "error body must be JSON")

	schema := loadErrorSchema(t)
	err := validateAgainstSchema(schema, payload)
	assert.NoError(t, err, "error body must conform to the OpenAPI Error schema")
	assert.NotEmpty(t, payload["error"], "error field must be present and non-empty")
}
