// Package config loads the service configuration from a YAML file using koanf.
package config

import (
	"fmt"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

// Config is the top-level, strongly-typed service configuration.
type Config struct {
	Server    ServerConfig    `koanf:"server"`
	Telemetry TelemetryConfig `koanf:"telemetry"`
	// Adapters binds each hexagonal port name to a selected adapter. The
	// per-adapter settings live under adapters.<port>.<type> and are decoded by
	// the adapter registry.
	Adapters map[string]AdapterBinding `koanf:"adapters"`
}

// ServerConfig configures the HTTP server.
type ServerConfig struct {
	Host            string        `koanf:"host"`
	Port            int           `koanf:"port"`
	ReadTimeout     time.Duration `koanf:"read_timeout"`
	WriteTimeout    time.Duration `koanf:"write_timeout"`
	ShutdownTimeout time.Duration `koanf:"shutdown_timeout"`
}

// TelemetryConfig configures OpenTelemetry tracing.
type TelemetryConfig struct {
	// Exporter selects the trace exporter: "stdout", "otlp", or "none".
	Exporter string `koanf:"exporter"`
	// OTLPEndpoint is the OTLP/gRPC collector endpoint (used when exporter=otlp).
	OTLPEndpoint string `koanf:"otlp_endpoint"`
	// SampleRatio is the parent-based trace sampling ratio (0.0 - 1.0).
	SampleRatio float64 `koanf:"sample_ratio"`
}

// AdapterBinding selects an adapter implementation for a port.
type AdapterBinding struct {
	Type string `koanf:"type"`
}

// LoadKoanf reads the YAML file and REPORT_-prefixed environment overrides into
// a koanf instance. The raw koanf is needed by the adapter registry to decode
// per-adapter settings.
func LoadKoanf(path string) (*koanf.Koanf, error) {
	k := koanf.New(".")

	if path != "" {
		if err := k.Load(file.Provider(path), yaml.Parser()); err != nil {
			return nil, fmt.Errorf("load config file %q: %w", path, err)
		}
	}

	// Environment overrides: REPORT_SERVER__PORT=9090 -> server.port
	if err := k.Load(env.Provider("REPORT_", ".", normalizeEnvKey), nil); err != nil {
		return nil, fmt.Errorf("load env overrides: %w", err)
	}
	return k, nil
}

// Parse decodes a koanf instance into the strongly-typed Config, applying defaults.
func Parse(k *koanf.Koanf) (*Config, error) {
	var cfg Config
	if err := Unmarshal(k, "", &cfg); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}
	cfg.applyDefaults()
	return &cfg, nil
}

// Load reads configuration from the given YAML file and returns both the typed
// config and the raw koanf instance. Convenience wrapper over LoadKoanf + Parse.
func Load(path string) (*Config, *koanf.Koanf, error) {
	k, err := LoadKoanf(path)
	if err != nil {
		return nil, nil, err
	}
	cfg, err := Parse(k)
	if err != nil {
		return nil, nil, err
	}
	return cfg, k, nil
}

// Unmarshal decodes the koanf subtree at path into out, applying the standard
// decode hooks (string -> time.Duration, comma-separated -> slice).
func Unmarshal(k *koanf.Koanf, path string, out any) error {
	return k.UnmarshalWithConf(path, out, koanf.UnmarshalConf{
		Tag: "koanf",
		DecoderConfig: &mapstructure.DecoderConfig{
			DecodeHook: mapstructure.ComposeDecodeHookFunc(
				mapstructure.StringToTimeDurationHookFunc(),
				mapstructure.StringToSliceHookFunc(","),
			),
			Metadata:         nil,
			Result:           out,
			WeaklyTypedInput: true,
		},
	})
}

// AdapterSettings returns the koanf subtree holding the settings for the adapter
// bound to the given port (adapters.<port>.<type>).
func (c *Config) AdapterSettings(k *koanf.Koanf, port string) (*koanf.Koanf, string, error) {
	binding, ok := c.Adapters[port]
	if !ok {
		return nil, "", fmt.Errorf("no adapter configured for port %q", port)
	}
	if binding.Type == "" {
		return nil, "", fmt.Errorf("adapter for port %q has no type", port)
	}
	sub := k.Cut(fmt.Sprintf("adapters.%s.%s", port, binding.Type))
	return sub, binding.Type, nil
}

func (c *Config) applyDefaults() {
	if c.Server.Host == "" {
		c.Server.Host = "0.0.0.0"
	}
	if c.Server.Port == 0 {
		c.Server.Port = 8080
	}
	if c.Server.ReadTimeout == 0 {
		c.Server.ReadTimeout = 15 * time.Second
	}
	if c.Server.WriteTimeout == 0 {
		c.Server.WriteTimeout = 30 * time.Second
	}
	if c.Server.ShutdownTimeout == 0 {
		c.Server.ShutdownTimeout = 10 * time.Second
	}
	if c.Telemetry.Exporter == "" {
		c.Telemetry.Exporter = "stdout"
	}
	if c.Telemetry.SampleRatio == 0 {
		c.Telemetry.SampleRatio = 1.0
	}
}

func normalizeEnvKey(s string) string {
	// REPORT_SERVER__PORT -> server.port ; single underscores are literal.
	out := make([]rune, 0, len(s))
	s = s[len("REPORT_"):]
	i := 0
	runes := []rune(s)
	for i < len(runes) {
		if runes[i] == '_' && i+1 < len(runes) && runes[i+1] == '_' {
			out = append(out, '.')
			i += 2
			continue
		}
		out = append(out, toLower(runes[i]))
		i++
	}
	return string(out)
}

func toLower(r rune) rune {
	if r >= 'A' && r <= 'Z' {
		return r + ('a' - 'A')
	}
	return r
}
