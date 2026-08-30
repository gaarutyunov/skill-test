package main

import (
	"log/slog"
	"os"

	"github.com/gaarutyunov/skill-test/go-service/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}
