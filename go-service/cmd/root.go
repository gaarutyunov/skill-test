// Package cmd holds the cobra command tree for the report service.
package cmd

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/gaarutyunov/skill-test/go-service/pkg/semconv"
)

// NewRootCommand builds the root cobra command.
func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "report-service",
		Short:         "Student PDF report generation microservice",
		Long:          "A microservice that generates PDF student reports by consuming the Node.js School Management backend API.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       semconv.Version,
	}

	var configPath string
	root.PersistentFlags().StringVarP(&configPath, "config", "c", "configs/config.yaml", "path to the YAML config file")

	root.AddCommand(newServeCommand(&configPath))
	root.AddCommand(newVersionCommand())
	return root
}

// Execute runs the root command with a context that is cancelled on SIGINT or
// SIGTERM, giving every command a signal-aware context for graceful shutdown.
func Execute() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return NewRootCommand().ExecuteContext(ctx)
}
