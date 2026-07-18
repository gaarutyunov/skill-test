package cmd

import (
	"github.com/spf13/cobra"

	"github.com/gaarutyunov/skill-test/go-service/internal/app"
)

// newServeCommand builds the `serve` subcommand that starts the HTTP server.
func newServeCommand(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start the HTTP server",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			application, cleanup, err := app.InitializeApp(ctx, *configPath)
			if err != nil {
				return err
			}
			defer cleanup()
			return application.Run(ctx)
		},
	}
}
