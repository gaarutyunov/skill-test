// Package cmd holds the cobra command tree for the report service.
package cmd

import (
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

// Execute runs the root command.
func Execute() error {
	return NewRootCommand().Execute()
}
