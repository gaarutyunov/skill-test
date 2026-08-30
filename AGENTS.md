# Agent & Contributor Guide

This file defines the mandatory working agreement for any agent or human
contributing to this repository. `CLAUDE.md` is a symlink to this file.

## Git workflow (mandatory)

- **NEVER push to `main`.** `main` is protected; direct commits/pushes are forbidden.
- **Always create a dedicated branch for every unit of work.**
- **Always create a git worktree for the branch** and do the work there, so the
  main checkout stays clean.
- **Open a Pull Request from the very start** (draft PR is fine) and keep pushing
  incremental commits to it. Never accumulate work locally without a PR.

## Golang work (mandatory skills)

For **any** Go development in this repository you MUST use the installed skills
under `.claude/skills/`:

- **`golang-pro`** — idiomatic Go, project structure, error handling, concurrency.
- **`testcontainers-go`** — integration testing with real dependencies in containers.
- **`tdd`** — the test-driven development loop (see below).
- **`otel-instrumentation`** — OpenTelemetry semantic instrumentation.

Consult and follow these skills; do not hand-roll patterns they already cover.

## Test-Driven Development (mandatory)

Development MUST follow TDD, in this exact order:

1. **Write integration tests first** against the real, running dependencies.
2. **Scaffold the minimum code** to make the project build. Prefer **code
   generation from specs** (OpenAPI → `oapi-codegen`, Weaver → semconv, Wire → DI)
   over hand-rolling scaffolding.
3. The project **must build and run inside the tests**, and the tests **must fail
   first** (red) — proving they exercise real behaviour, not stubs.
4. **Implement** the behaviour and run the tests until they pass (green).
5. Refactor with the tests staying green.

### Mocks (mandatory)

- **Integration tests** use **no mocks** — real dependencies via testcontainers.
- **Unit tests** MUST use **Uber's `go.uber.org/mock` (gomock + `mockgen`)** for
  any test double. **NEVER** hand-roll mocks/stubs/fakes. Generate mocks from the
  port interfaces with `mockgen` (see `make generate`).

## Telemetry (mandatory)

- **Always prefer official OpenTelemetry semantic conventions before inventing
  attributes.** Before adding any telemetry attribute, check the official
  conventions (the `go.opentelemetry.io/otel/semconv/<version>` package and the
  [semantic-conventions registries](https://github.com/open-telemetry/semantic-conventions/tree/main/model)).
  Use the official key/constant when one exists (e.g. `service.*`, `vcs.*`,
  `http.*`, `url.*`, `server.*`, `error.type`, `code.*`).
- Only define **project-specific** attributes (in the Weaver registry under
  `go-service/semconv/registry`, generated into `pkg/semconv`) for concepts that
  have **no** official convention (e.g. `report.*`, `student.id`, `adapter.*`).

## Go style rules

- **NEVER** use explicit interface-satisfaction assertions such as
  `var _ Interface = (*Impl)(nil)`. This is un-idiomatic, Java-flavoured noise.
  Rely on the compiler at the call/wiring site to enforce interface satisfaction.
- Prefer small, composable interfaces defined at the consumer (port) side.
- Wrap errors with `%w` and context; never swallow errors.

## Project layout

The Go microservice lives in `go-service/` (see its `README.md`). It follows a
hexagonal architecture: `internal/domain`, `internal/port`, `internal/usecase`,
`internal/adapter`, with generated code in `pkg/api`, `internal/server`, and
`pkg/semconv`.
