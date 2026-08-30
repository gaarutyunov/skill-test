package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gaarutyunov/skill-test/go-service/internal/config"
)

const sampleYAML = `
server:
  host: 127.0.0.1
  port: 9090
  read_timeout: 5s
telemetry:
  exporter: none
adapters:
  student_repository:
    type: http
    http:
      base_url: http://backend:5007
      session_ttl: 90s
      retry:
        max_retries: 5
        wait_min: 100ms
        wait_max: 1s
  report_generator:
    type: pdf
    pdf:
      title: My Report
`

func writeConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(sampleYAML), 0o644))
	return path
}

func TestLoad(t *testing.T) {
	// Arrange
	path := writeConfig(t)

	// Act
	loaded, err := config.Load(t.Context(), path)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1", loaded.Value.Server.Host)
	assert.Equal(t, 9090, loaded.Value.Server.Port)
	assert.Equal(t, 5*time.Second, loaded.Value.Server.ReadTimeout)
	assert.Equal(t, "none", loaded.Value.Telemetry.Exporter)

	// Adapter binding + settings subtree resolution.
	settings, name, err := config.AdapterSettings(loaded, "student_repository")
	require.NoError(t, err)
	assert.Equal(t, "http", name)
	assert.Equal(t, "http://backend:5007", settings.String("base_url"))
	assert.Equal(t, 5, settings.Int("retry.max_retries"))
}

// TestAdapterSettings_DecodesSubtree pins the pattern goga/config's raw koanf
// handle exists for: an adapter is configured from its own subtree and never
// learns the application's configuration struct. Cut returns a bare koanf, so
// the decode hooks (session_ttl, wait_min) come from config.Unmarshal.
func TestAdapterSettings_DecodesSubtree(t *testing.T) {
	// Arrange
	path := writeConfig(t)
	loaded, err := config.Load(t.Context(), path)
	require.NoError(t, err)

	// Act
	settings, _, err := config.AdapterSettings(loaded, "student_repository")
	require.NoError(t, err)

	var sub struct {
		BaseURL    string        `koanf:"base_url"`
		SessionTTL time.Duration `koanf:"session_ttl"`
		Retry      struct {
			MaxRetries int           `koanf:"max_retries"`
			WaitMin    time.Duration `koanf:"wait_min"`
		} `koanf:"retry"`
	}
	err = config.Unmarshal(settings, "", &sub)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "http://backend:5007", sub.BaseURL)
	assert.Equal(t, 90*time.Second, sub.SessionTTL)
	assert.Equal(t, 5, sub.Retry.MaxRetries)
	assert.Equal(t, 100*time.Millisecond, sub.Retry.WaitMin)
}

// TestAdapterSettings_EnvReachesSubtree is the reason AdapterSettings cuts the
// MERGED handle: an operator supplying a credential from the environment must
// reach the adapter exactly like a value written in the file.
func TestAdapterSettings_EnvReachesSubtree(t *testing.T) {
	// Arrange
	path := writeConfig(t)
	t.Setenv("REPORT__ADAPTERS__STUDENT_REPOSITORY__HTTP__USERNAME", "admin@school-admin.com")

	// Act
	loaded, err := config.Load(t.Context(), path)
	require.NoError(t, err)
	settings, _, err := config.AdapterSettings(loaded, "student_repository")

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "admin@school-admin.com", settings.String("username"))
}

func TestLoad_EnvOverride(t *testing.T) {
	// Arrange
	path := writeConfig(t)
	t.Setenv("REPORT__SERVER__PORT", "7777")

	// Act
	loaded, err := config.Load(t.Context(), path)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 7777, loaded.Value.Server.Port)
}

func TestLoad_AppliesDefaults(t *testing.T) {
	// Arrange
	path := filepath.Join(t.TempDir(), "empty.yaml")
	require.NoError(t, os.WriteFile(path, []byte("{}"), 0o644))

	// Act
	loaded, err := config.Load(t.Context(), path)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "0.0.0.0", loaded.Value.Server.Host)
	assert.Equal(t, 8080, loaded.Value.Server.Port)
	assert.Equal(t, 15*time.Second, loaded.Value.Server.ReadTimeout)
	assert.Equal(t, "stdout", loaded.Value.Telemetry.Exporter)
	assert.InEpsilon(t, 1.0, loaded.Value.Telemetry.SampleRatio, 0.0001)
}

// TestLoad_MissingFileIsNotAnError pins the goga/config contract that a config
// file which is not there is an absence, not a misconfiguration: the common
// production deployment is pure environment variables.
func TestLoad_MissingFileIsNotAnError(t *testing.T) {
	// Arrange
	path := filepath.Join(t.TempDir(), "absent.yaml")
	t.Setenv("REPORT__SERVER__PORT", "7788")

	// Act
	loaded, err := config.Load(t.Context(), path)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 7788, loaded.Value.Server.Port)
	assert.Equal(t, "0.0.0.0", loaded.Value.Server.Host)
}
