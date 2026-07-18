//go:build integration

package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-openapi/spec"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/validate"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// loadErrorSchema extracts the components.schemas.Error definition from the
// OpenAPI document and converts it into a go-openapi spec.Schema so responses
// can be validated against the actual spec.
func loadErrorSchema(t *testing.T) *spec.Schema {
	t.Helper()
	return loadComponentSchema(t, "Error")
}

func loadComponentSchema(t *testing.T, name string) *spec.Schema {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join("..", "..", "api", "openapi.yaml"))
	require.NoError(t, err, "read openapi.yaml")

	var doc map[string]any
	require.NoError(t, yaml.Unmarshal(raw, &doc), "parse openapi.yaml")

	components, ok := doc["components"].(map[string]any)
	require.True(t, ok, "openapi.yaml must define components")
	schemas, ok := components["schemas"].(map[string]any)
	require.True(t, ok, "openapi.yaml must define components.schemas")
	def, ok := schemas[name]
	require.Truef(t, ok, "openapi.yaml must define components.schemas.%s", name)

	// Round-trip the fragment through JSON into a go-openapi spec.Schema.
	j, err := json.Marshal(def)
	require.NoError(t, err)
	var schema spec.Schema
	require.NoError(t, json.Unmarshal(j, &schema), "decode %s schema", name)
	return &schema
}

// validateAgainstSchema validates data against the given schema using
// go-openapi/validate.
func validateAgainstSchema(schema *spec.Schema, data any) error {
	return validate.AgainstSchema(schema, data, strfmt.Default)
}
