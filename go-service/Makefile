# Student PDF Report Service — developer Makefile.
.DEFAULT_GOAL := help

BINARY      := report-service
PKG_SEMCONV := github.com/gaarutyunov/skill-test/go-service/pkg/semconv

# Git-derived build metadata, injected via -ldflags -X.
VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
REVISION   ?= $(shell git rev-parse HEAD 2>/dev/null || echo unknown)
TAG        ?= $(shell git describe --tags --exact-match 2>/dev/null || echo "")
REPOSITORY ?= $(shell git config --get remote.origin.url 2>/dev/null || echo "")
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -X $(PKG_SEMCONV).Version=$(VERSION) \
           -X $(PKG_SEMCONV).Revision=$(REVISION) \
           -X $(PKG_SEMCONV).Tag=$(TAG) \
           -X $(PKG_SEMCONV).Repository=$(REPOSITORY) \
           -X $(PKG_SEMCONV).Date=$(BUILD_DATE)

WEAVER_IMAGE ?= otel/weaver:latest

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: ## Build the service binary with build-info LDFLAGS
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) .

.PHONY: run
run: ## Run the service locally (serve)
	go run -ldflags "$(LDFLAGS)" . serve

.PHONY: generate
generate: ## Run all code generation (oapi-codegen, wire, mockgen)
	go generate ./...

.PHONY: semconv
semconv: ## Regenerate pkg/semconv from the Weaver registry (requires docker)
	docker run --rm -v "$(CURDIR)":/work -w /work $(WEAVER_IMAGE) \
		registry generate -r semconv/registry -t semconv/templates go pkg/semconv
	gofmt -w pkg/semconv/attributes.go

.PHONY: semconv-check
semconv-check: ## Validate the Weaver semantic-convention registry
	docker run --rm -v "$(CURDIR)/semconv":/work -w /work $(WEAVER_IMAGE) registry check -r registry

.PHONY: fmt
fmt: ## Format the code
	gofmt -w $(shell find . -name '*.go' -not -name '*.gen.go' -not -name 'wire_gen.go')

.PHONY: lint
lint: ## Run golangci-lint
	golangci-lint run ./...

.PHONY: test-unit
test-unit: ## Run unit tests with coverage
	go test -race -covermode=atomic -coverprofile=coverage.out ./...

.PHONY: check
check: fmt lint test-unit ## Format, lint and unit-test (CI gate)

.PHONY: test-integration
test-integration: ## Run integration tests (testcontainers; requires docker)
	go test -tags=integration -v -timeout 20m ./tests/integration/...

.PHONY: docker-build
docker-build: ## Build the service docker image
	docker build -t $(BINARY):$(VERSION) --build-arg VERSION=$(VERSION) --build-arg REVISION=$(REVISION) --build-arg BUILD_DATE=$(BUILD_DATE) .

.PHONY: compose-up
compose-up: ## Launch the full stack locally (postgres + node + service)
	docker compose up --build

.PHONY: compose-down
compose-down: ## Tear down the local stack
	docker compose down -v

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf bin coverage.out testdata/artifacts
