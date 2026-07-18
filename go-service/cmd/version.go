package cmd

import (
	"encoding/json"
	"os"

	"github.com/spf13/cobra"

	"github.com/gaarutyunov/skill-test/go-service/pkg/semconv"
)

// newVersionCommand prints the compiled-in build information.
func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print build information",
		RunE: func(_ *cobra.Command, _ []string) error {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(semconv.Build())
		},
	}
}
