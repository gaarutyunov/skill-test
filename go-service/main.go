package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/gaarutyunov/skill-test/go-service/cmd"
)

func main() {
	root := cmd.NewRootCommand()
	if err := root.ExecuteContext(context.Background()); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}
