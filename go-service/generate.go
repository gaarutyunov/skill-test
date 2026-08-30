package main

// Code generation entry points. Run `make generate` (or `go generate ./...`).

// OpenAPI: upstream (Node) client, report client, report server.
//go:generate go tool oapi-codegen -config pkg/api/upstream/cfg.yaml api/openapi.yaml
//go:generate go tool oapi-codegen -config pkg/api/report/cfg.yaml api/openapi.yaml
//go:generate go tool oapi-codegen -config internal/server/cfg.yaml api/openapi.yaml

// Wire dependency-injection graph.
//go:generate go tool wire gen ./internal/app

// gomock mocks for the hexagonal ports (unit tests only).
//go:generate go tool mockgen -destination internal/port/mock/mock_port.go -package mock github.com/gaarutyunov/skill-test/go-service/internal/port StudentRepository,ReportGenerator
