# Contributing to sandboxMatrix

Thank you for your interest in contributing to sandboxMatrix. This guide covers development setup, project structure, coding conventions, and the PR process.

## Development Setup

### Prerequisites

- **Go 1.25+** -- [Install Go](https://go.dev/doc/install)
- **Docker Engine** -- Required for the default runtime and e2e tests
- **golangci-lint** -- [Install](https://golangci-lint.run/welcome/install-local/) for linting
- **goimports** -- `go install golang.org/x/tools/cmd/goimports@latest`
- **Make** -- For build commands

### Clone and Build

```bash
git clone https://github.com/hgDendi/sandboxmatrix.git
cd sandboxmatrix
make build
```

### Verify

```bash
./bin/smx version
make test
make lint
```

### Available Make Targets

| Target | Description |
|--------|-------------|
| `make build` | Build the `smx` binary to `bin/` |
| `make install` | Install `smx` to `$GOPATH/bin` |
| `make test` | Run all tests |
| `make test-race` | Run tests with Go race detector |
| `make test-cover` | Run tests with coverage report (`coverage.html`) |
| `make lint` | Run golangci-lint |
| `make fmt` | Format code with `go fmt` and `goimports` |
| `make vet` | Run `go vet` |
| `make e2e` | Run end-to-end tests (requires Docker) |
| `make clean` | Remove build artifacts |

## Project Structure

```
sandboxmatrix/
  cmd/smx/              # CLI entrypoint
  internal/
    agent/
      a2a/              # Agent-to-agent messaging gateway
      mcp/              # MCP server (13 tools for AI agents)
    aggregation/        # Result collection from distributed tasks
    auth/               # RBAC and audit logging
    cli/                # CLI command definitions (cobra)
    config/             # Configuration file management
    controller/         # Core orchestrator (sandbox, matrix, session lifecycle)
    observability/      # Prometheus metrics
    pool/               # Pre-warmed sandbox pool manager
    probe/              # Readiness probes (exec, HTTP, TCP)
    runtime/
      docker/           # Docker runtime implementation
      runtime.go        # Runtime interface definition
    server/             # REST API server, handlers, middleware, WebSocket
    sharding/           # Task distribution strategies
    state/              # State store backends (file, BoltDB, etcd)
    web/                # Embedded web dashboard
      static/           # HTML/CSS/JS assets
  pkg/
    api/v1alpha1/       # API types (Sandbox, Matrix, Session, Blueprint, etc.)
    blueprint/          # Blueprint YAML parser and validator
  sdk/
    python/             # Python SDK (CLI + HTTP client)
    typescript/         # TypeScript SDK (CLI + HTTP client)
  deploy/
    crds/               # Kubernetes CRDs
    helm/               # Helm chart
  blueprints/           # Example blueprint YAML files
  docs/                 # Documentation
  test/                 # E2E test scripts
```

## Code Style

### Go Conventions

- Follow standard Go conventions ([Effective Go](https://go.dev/doc/effective_go), [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)).
- Use `goimports` for import ordering (stdlib, then external, then internal).
- All exported types, functions, and methods must have doc comments.
- Use `log/slog` for structured logging -- never `fmt.Println` in library code.
- Errors should provide context: `fmt.Errorf("create sandbox: %w", err)`.

### Linting

The project uses golangci-lint. Run before submitting:

```bash
make lint
```

Key linters enabled:
- `gofmt` -- Code formatting
- `govet` -- Go vet checks
- `errcheck` -- Unchecked error returns
- `misspell` -- Spelling errors in comments and strings
- `gocritic` -- Opinionated Go style checks

### Formatting

```bash
make fmt
```

This runs both `go fmt` and `goimports` across the entire project.

## Testing Guidelines

### Unit Tests

Write unit tests alongside the code in `*_test.go` files. Tests should:

- Be deterministic (no flaky tests)
- Not require Docker or external services
- Use `testing.T` and standard Go test patterns
- Use table-driven tests where appropriate

```bash
# Run all tests
make test

# Run tests with race detector
make test-race

# Run tests with coverage
make test-cover
```

### End-to-End Tests

E2E tests require Docker and exercise the full CLI flow:

```bash
make e2e
```

E2E test scripts live in `test/`.

### Testing New Features

1. Add unit tests for new packages and functions
2. If the feature involves the REST API, add handler tests (see `internal/server/server_test.go`)
3. If the feature involves WebSocket, add WebSocket tests (see `internal/server/ws_handler_test.go`)
4. Consider adding e2e test coverage for user-facing workflows

## Pull Request Process

1. **Fork and branch** -- Create a feature branch from `main`:
   ```bash
   git checkout -b feat/my-feature
   ```

2. **Make changes** -- Follow the code style guidelines above.

3. **Test** -- Ensure all tests pass:
   ```bash
   make lint && make test
   ```

4. **Commit** -- Write clear commit messages following [Conventional Commits](https://www.conventionalcommits.org/):
   - `feat: add TCP probe support`
   - `fix: prevent race condition in pool warm loop`
   - `docs: update API reference for snapshot endpoints`
   - `refactor: extract device config builder`
   - `test: add matrix create handler tests`

5. **Push and open PR** -- Push your branch and open a pull request against `main`.

6. **CI checks** -- The PR must pass:
   - `make lint` (golangci-lint)
   - `make test` (unit tests)
   - `make vet` (go vet)

7. **Review** -- Address review feedback with additional commits (do not force-push).

## Adding New Runtime Backends

To add a new isolation backend (e.g., Podman, WASM):

1. **Create the package** -- `internal/runtime/podman/podman.go`

2. **Implement the interface** -- Your type must satisfy `runtime.Runtime` (14 methods):
   ```go
   type PodmanRuntime struct { ... }

   func (r *PodmanRuntime) Name() string { return "podman" }
   func (r *PodmanRuntime) Create(ctx context.Context, cfg *runtime.CreateConfig) (string, error) { ... }
   func (r *PodmanRuntime) Start(ctx context.Context, id string) error { ... }
   // ... implement all 14 methods from runtime.Runtime
   ```

3. **Register the runtime** -- Add it to the runtime selection logic in the CLI (usually in `internal/cli/`) so that `--runtime podman` or `defaultRuntime: podman` in config selects your backend.

4. **Add tests** -- Unit tests for the runtime implementation. E2E tests if the runtime is available in CI.

5. **Document** -- Update the README and architecture docs.

## Adding New MCP Tools

To expose a new operation to AI agents via the MCP server:

1. **Define the tool** in `internal/agent/mcp/server.go` in the `registerTools()` method:
   ```go
   s.mcpServer.AddTool(
       mcp.NewTool("my_new_tool",
           mcp.WithDescription("Description of what this tool does"),
           mcp.WithString("param1",
               mcp.Required(),
               mcp.Description("Parameter description"),
           ),
       ),
       s.handleMyNewTool,
   )
   ```

2. **Implement the handler**:
   ```go
   func (s *Server) handleMyNewTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
       args := request.GetArguments()
       // Validate parameters, call controller methods, return result
       return mcp.NewToolResultText("result"), nil
   }
   ```

3. **Update documentation** -- Add the tool to the MCP tools table in the README.

4. **Add tests** -- Test the handler logic.

## Adding New REST API Endpoints

1. **Add the route** in `internal/server/server.go` in `registerRoutes()`:
   ```go
   s.router.HandleFunc("POST /api/v1/myresource", handleMyResource(s.ctrl))
   ```

2. **Add the handler** in `internal/server/handlers.go`:
   ```go
   func handleMyResource(ctrl *controller.Controller) http.HandlerFunc {
       return func(w http.ResponseWriter, r *http.Request) {
           // Validate input, call controller, write response
       }
   }
   ```

3. **Update the API reference** in `docs/api-reference.md`.

4. **Add handler tests** in `internal/server/server_test.go`.

## Reporting Issues

Use GitHub Issues for bug reports and feature requests:
- **Bug reports**: Include reproduction steps, expected vs actual behavior, Go version, and OS
- **Feature requests**: Describe the use case and proposed solution

## Code of Conduct

This project follows the [Contributor Covenant](CODE_OF_CONDUCT.md). Please be respectful and constructive.
