// Package config loads the service configuration through goga/config, which
// merges defaults, the YAML file and the environment in one fixed order and
// decodes the result into [Config].
package config

import (
	"context"
	"fmt"
	"time"

	gogaconfig "github.com/gaarutyunov/goga/config"
	"github.com/go-viper/mapstructure/v2"
	"github.com/knadh/koanf/v2"
)

// EnvPrefix is the environment prefix goga/config reads. Following the goga
// convention, "__" separates key-path segments and "_" is a literal underscore
// inside one, so REPORT__ADAPTERS__STUDENT_REPOSITORY__HTTP__BASE_URL sets
// adapters.student_repository.http.base_url.
const EnvPrefix = "REPORT"

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

// Loaded is a loaded service configuration: the decoded [Config] plus the
// merged koanf handle it was decoded from.
//
// The handle is what makes the adapter registry possible: an adapter is
// configured from the subtree [AdapterSettings] cuts, so it never learns the
// application's configuration struct and the application never grows a field
// per adapter.
type Loaded = gogaconfig.Config[Config]

// Load reads the configuration from defaults, the YAML file at path (absence is
// not an error) and REPORT-prefixed environment variables, in that fixed order.
//
// This is the one-line instantiation of the generic goga/config loader. It lives
// here, next to the type it is instantiated with, because wire cannot provide a
// generic function: wire's generator works from concrete types and there is no
// way to name config.Load[Config] in a provider set. Everything downstream of
// this call — see internal/app — is wired normally.
func Load(ctx context.Context, path string) (*Loaded, error) {
	return gogaconfig.Load[Config](ctx,
		gogaconfig.WithDefaults(Defaults()),
		gogaconfig.WithFile(path),
		gogaconfig.WithEnv(EnvPrefix),
	)
}

// Defaults is the lowest-precedence source: the values the service runs on when
// neither the file nor the environment sets them.
//
// They are a source rather than a post-decode fixup, so that an operator who
// explicitly writes `telemetry.sample_ratio: 0` gets zero sampling instead of
// having it silently promoted back to 1.0.
func Defaults() map[string]any {
	return map[string]any{
		"server.host":             "0.0.0.0",
		"server.port":             8080,
		"server.read_timeout":     15 * time.Second,
		"server.write_timeout":    30 * time.Second,
		"server.shutdown_timeout": 10 * time.Second,
		"telemetry.exporter":      "stdout",
		"telemetry.sample_ratio":  1.0,
	}
}

// Unmarshal decodes the koanf subtree at path into out.
//
// It exists because [gogaconfig.Config.Cut] hands back a bare *koanf.Koanf and
// goga/config does not export the decoder it used for the top-level value. A
// module configured from a subtree — an adapter here — therefore has to
// reproduce goga's decoder itself, and the two must agree: same "koanf" tag,
// same WeaklyTypedInput (every value arriving from the environment is a string),
// and the same duration/slice hooks. Adapter settings like `session_ttl: 10m`
// depend on all three.
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
// bound to the given port (adapters.<port>.<type>), and the adapter's name.
func AdapterSettings(cfg *Loaded, port string) (*koanf.Koanf, string, error) {
	binding, ok := cfg.Value.Adapters[port]
	if !ok {
		return nil, "", fmt.Errorf("no adapter configured for port %q", port)
	}
	if binding.Type == "" {
		return nil, "", fmt.Errorf("adapter for port %q has no type", port)
	}
	// Cut on the MERGED handle, so an adapter setting supplied by the
	// environment reaches the adapter exactly like one written in the file.
	sub := cfg.Cut(fmt.Sprintf("adapters.%s.%s", port, binding.Type))
	return sub, binding.Type, nil
}
