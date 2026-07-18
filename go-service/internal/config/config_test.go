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
	cfg, k, err := config.Load(path)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1", cfg.Server.Host)
	assert.Equal(t, 9090, cfg.Server.Port)
	assert.Equal(t, 5*time.Second, cfg.Server.ReadTimeout)
	assert.Equal(t, "none", cfg.Telemetry.Exporter)

	// Adapter binding + settings subtree resolution.
	settings, name, err := cfg.AdapterSettings(k, "student_repository")
	require.NoError(t, err)
	assert.Equal(t, "http", name)
	assert.Equal(t, "http://backend:5007", settings.String("base_url"))
	assert.Equal(t, 5, settings.Int("retry.max_retries"))
}

func TestLoad_EnvOverride(t *testing.T) {
	// Arrange
	path := writeConfig(t)
	t.Setenv("REPORT_SERVER__PORT", "7777")

	// Act
	cfg, _, err := config.Load(path)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 7777, cfg.Server.Port)
}

func TestParse_AppliesDefaults(t *testing.T) {
	// Arrange
	path := filepath.Join(t.TempDir(), "empty.yaml")
	require.NoError(t, os.WriteFile(path, []byte("{}"), 0o644))

	// Act
	cfg, _, err := config.Load(path)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "0.0.0.0", cfg.Server.Host)
	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, "stdout", cfg.Telemetry.Exporter)
	assert.InEpsilon(t, 1.0, cfg.Telemetry.SampleRatio, 0.0001)
}
