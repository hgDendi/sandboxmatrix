# Architecture Overview

## System Overview

sandboxMatrix is an open-source, local-first AI sandbox orchestrator. It provides Kubernetes-inspired abstractions for managing isolated development environments that AI agents can use to safely execute code, install packages, and manage projects.

**Design philosophy:**

- **K8s-inspired** -- Familiar concepts (Pod=Sandbox, Namespace=Matrix, Job=Session, CRI=Runtime Interface, Helm Chart=Blueprint) adapted for AI agent workflows.
- **Local-first** -- Runs as a single binary on a developer's machine with no cloud dependency. Scales up to distributed deployments with etcd and K8s operator when needed.
- **Pluggable everything** -- Runtime backends (Docker, gVisor, Firecracker), state stores (file, BoltDB, etcd), and network policies are all swappable.
- **AI-native** -- Built-in MCP server for AI agent integration, agent-to-agent messaging, task sharding, and result aggregation.

## Architecture Diagram

```
+----------------------------------------------------------------------+
|                         Interface Layer                               |
|  CLI (smx)  |  REST API  |  SDKs (Python/TS)  |  Web Dashboard      |
+----------------------------------------------------------------------+
|                         Agent Plane                                   |
|  MCP Server (13 tools)  |  A2A Gateway  |  Session Manager           |
+----------------------------------------------------------------------+
|                         Control Plane                                 |
|  Controller  |  Scheduler  |  Pool Manager  |  RBAC + Audit + Metrics|
+----------------------------------------------------------------------+
|                     Orchestration Plane                               |
|  Readiness Probes  |  Task Sharding  |  Result Aggregation           |
+----------------------------------------------------------------------+
|                         Runtime Plane (pluggable)                     |
|  Docker  |  gVisor  |  Firecracker (stub)  |  Kata  |  WASM         |
+----------------------------------------------------------------------+
|                         Storage Plane                                 |
|  Workspaces  |  Snapshots  |  State (JSON / BoltDB / etcd)          |
+----------------------------------------------------------------------+
|                         Deployment                                    |
|  Single binary  |  K8s Operator + CRDs  |  Helm Chart               |
+----------------------------------------------------------------------+
```

## Core Components

### Controller (`internal/controller/`)

The Controller is the central orchestrator. It coordinates sandbox lifecycle operations (create, start, stop, destroy, exec, snapshot, restore) by composing the Runtime interface and the State store.

Key responsibilities:
- **Sandbox lifecycle** (`controller.go`) -- Validates blueprints, builds runtime configs, creates containers, runs readiness probes, and records state transitions. Uses a mutex to serialize create/destroy operations and prevent duplicate names.
- **Matrix orchestration** (`matrix.go`) -- Creates groups of sandboxes on an isolated Docker network. Member sandbox names are prefixed with the matrix name (e.g., `fullstack-frontend`).
- **Session management** (`session.go`) -- Creates bounded execution contexts tied to a sandbox. Each session tracks an exec count and transitions through Active, Completed, or Failed states.
- **Reconciliation** (`reconciler.go`) -- On startup, scans the runtime for existing containers with `sandboxmatrix/*` labels and re-imports them into the state store. This ensures sandboxes survive CLI restarts.
- **Metrics recording** -- Every operation records Prometheus counters and histograms via the observability package.

### Runtime Interface (`internal/runtime/`)

The `Runtime` interface defines 14 methods that all isolation backends must implement:

```go
type Runtime interface {
    Name() string
    Create(ctx, cfg) (id, error)
    Start(ctx, id) error
    Stop(ctx, id) error
    Destroy(ctx, id) error
    Exec(ctx, id, cfg) (ExecResult, error)
    Info(ctx, id) (Info, error)
    Stats(ctx, id) (Stats, error)
    List(ctx) ([]Info, error)
    Snapshot(ctx, id, tag) (snapshotID, error)
    Restore(ctx, snapshotID, cfg) (id, error)
    ListSnapshots(ctx, id) ([]SnapshotInfo, error)
    DeleteSnapshot(ctx, snapshotID) error
    CreateNetwork(ctx, name, internal) error
    DeleteNetwork(ctx, name) error
}
```

**Implementations:**
- `internal/runtime/docker/` -- Production backend using the Docker Engine API. Supports GPU passthrough (NVIDIA), device passthrough, security hardening (no-new-privileges, read-only rootfs, dropped capabilities), and network isolation.
- gVisor and Firecracker backends are defined but Firecracker remains a stub.

### State Store (`internal/state/`)

Pluggable persistence for sandbox, session, and matrix records. Three interfaces:

- `Store` -- Sandbox CRUD (Get, Save, Delete, List)
- `SessionStore` -- Session CRUD (GetSession, SaveSession, ListSessions, ListSessionsBySandbox)
- `MatrixStore` -- Matrix CRUD (Get, Save, Delete, List)

**Backends:**
| Backend | Package | Use case |
|---------|---------|----------|
| File (JSON) | `internal/state/` | Default. Stores JSON files in `~/.sandboxmatrix/state/` |
| BoltDB | `internal/state/` | Embedded key-value store. Single-file, no server needed |
| etcd | `internal/state/distributed.go` | Distributed deployments. Full Store + SessionStore + MatrixStore |

Backend selection is configured via `StoreConfig.Backend` ("file", "bolt", or "etcd").

### MCP Server (`internal/agent/mcp/`)

Implements a Model Context Protocol server using `github.com/mark3labs/mcp-go`. Exposes 13 tools over stdio for AI agent integration:

| Tool | Description |
|------|-------------|
| `sandbox_create` | Create sandbox from blueprint |
| `sandbox_list` | List all sandboxes |
| `sandbox_exec` | Execute command (via `sh -c`) |
| `sandbox_start` | Start stopped sandbox |
| `sandbox_stop` | Stop running sandbox |
| `sandbox_destroy` | Destroy and clean up |
| `sandbox_stats` | CPU/memory statistics |
| `sandbox_ready_wait` | Poll until readiness probe passes |
| `a2a_send` | Send agent-to-agent message |
| `a2a_receive` | Receive pending messages |
| `a2a_broadcast` | Broadcast to multiple sandboxes |
| `matrix_shard_task` | Distribute tasks across matrix |
| `matrix_collect_results` | Aggregate results from matrix |

The MCP server delegates to the same Controller used by the CLI and REST API.

### A2A Gateway (`internal/agent/a2a/`)

In-memory message-passing system for agent-to-agent communication. Each sandbox has an inbox (a slice of `Message` structs). Features:

- **Send** -- Deliver a message to a sandbox's inbox. Assigns UUID and timestamp.
- **Receive** -- Retrieve and clear all pending messages (destructive read).
- **Peek** -- Read messages without clearing.
- **ReceiveByType** -- Selectively drain messages of a specific type, preserving others.
- **Subscribe** -- Register callback handlers for real-time notification.
- **Broadcast** -- Send to multiple targets in one call.

Messages have `From`, `To`, `Type`, and `Payload` (JSON string) fields.

### REST API Server (`internal/server/`)

HTTP server built on Go 1.22+ `http.ServeMux` with path parameters. Provides 24 endpoints across sandboxes, matrices, sessions, and system health.

**Middleware stack** (applied in order):
1. `loggingMiddleware` -- Structured request logging + Prometheus HTTP metrics
2. `corsMiddleware` -- CORS headers for cross-origin dashboard access
3. `jsonContentTypeMiddleware` -- Sets `Content-Type: application/json`
4. `AuthMiddleware` -- RBAC token validation + audit logging (no-op if RBAC not configured)

**WebSocket support** (`ws_handler.go`):
The `/api/v1/sandboxes/{name}/exec/stream` endpoint upgrades to WebSocket for real-time stdin/stdout/stderr streaming. Uses `io.Pipe` for stdin and custom `wsWriter` for streaming output.

### Web Dashboard (`internal/web/`)

Embedded web UI served from `internal/web/static/` using Go's `embed` package. Provides:
- Real-time sandbox/matrix/session listing with auto-refresh
- Start/stop/destroy controls
- In-browser terminal (WebSocket exec)
- Dark theme

Runs on a separate port (default `:9090`) from the REST API.

### Observability (`internal/observability/`)

Prometheus metrics registered via `promauto`:

| Metric | Type | Labels |
|--------|------|--------|
| `smx_sandboxes_active` | Gauge | -- |
| `smx_sandbox_operations_total` | Counter | operation, result |
| `smx_sandbox_operation_duration_seconds` | Histogram | operation |
| `smx_exec_total` | Counter | sandbox, result |
| `smx_exec_duration_seconds` | Histogram | sandbox |
| `smx_sessions_active` | Gauge | -- |
| `smx_matrices_active` | Gauge | -- |
| `smx_pool_size` | Gauge | blueprint |
| `smx_http_requests_total` | Counter | method, path, status |
| `smx_http_request_duration_seconds` | Histogram | method, path |
| `smx_websocket_connections` | Gauge | -- |

All components use Go's `log/slog` for structured JSON logging.

### Pool Manager (`internal/pool/`)

Maintains pre-warmed, started containers for instant sandbox creation. Per-blueprint configuration:

- `MinReady` -- Minimum warm instances to maintain
- `MaxSize` -- Maximum pool size

A background goroutine per blueprint monitors the pool and creates replacement containers when instances are claimed. Tracks statistics: ready count, in-use count, total created, average claim time.

### Probe Runner (`internal/probe/`)

Implements Kubernetes-style readiness probes with three probe types:

- **exec** -- Runs a command inside the container; success = exit code 0
- **http** -- Sends GET request to sandbox IP:port; success = 2xx status
- **tcp** -- Opens TCP connection to sandbox IP:port; success = connection established

Configurable parameters: `initialDelaySec`, `periodSec`, `timeoutSec`, `successThreshold`, `failureThreshold`.

### Sharding (`internal/sharding/`)

Distributes tasks across matrix members using pluggable strategies:

- **round-robin** (default) -- Sequential assignment with atomic counter
- **hash** -- FNV-32a hash of task key determines member
- **balanced** -- Assigns to member with fewest current tasks

### Aggregation (`internal/aggregation/`)

Collects results from distributed matrix tasks by polling the coordinator's A2A inbox for `task-result` messages. Strategies:

- **collect-all** (default) -- Wait for all members to report
- **first-success** -- Return as soon as one member succeeds
- **majority** -- Return when >50% of members succeed

### Blueprint System (`pkg/blueprint/`)

Parses and validates YAML blueprint files (`smx/v1alpha1` API version). A blueprint defines:
- Base image, runtime backend, resource limits (CPU, memory, disk, GPU)
- Setup commands, toolchain sidecars
- Workspace mount configuration
- Network policy (none, host, bridge, isolate) and exposed ports
- Device passthrough mappings
- Readiness probe configuration

### Config (`internal/config/`)

Manages `~/.sandboxmatrix/config.yaml` with defaults:

```yaml
defaultRuntime: docker
logLevel: info
server:
  addr: ":8080"
dashboard:
  addr: ":9090"
pool:
  minReady: 2
  maxSize: 5
```

### RBAC (`internal/auth/`)

Role-based access control with three built-in roles:

| Role | Permissions |
|------|-------------|
| `admin` | Full access to all resources |
| `operator` | CRUD + exec on sandboxes, matrices, sessions, pools; read blueprints |
| `viewer` | Read-only access to all resources |

Authentication uses Bearer tokens in the `Authorization` header. Tokens are 32-byte hex strings generated with `crypto/rand`. Audit logging records every action with timestamp, user, resource, and result.

## Data Flow

### Sandbox Creation

```
Client (CLI/SDK/API)
  |
  v
Controller.Create()
  |-- Validate blueprint (pkg/blueprint)
  |-- Build runtime.CreateConfig (image, CPU, memory, GPU, devices, network, mounts)
  |-- Save initial state (SandboxStateCreating) to Store
  |-- runtime.Create() -> container ID
  |-- runtime.Start() -> running container
  |-- runtime.Info() -> get IP address
  |-- If readiness probe configured:
  |     probe.Runner.WaitForReady() -> poll until ready
  |     Update state to SandboxStateReady
  |-- Save final state to Store
  |-- Increment smx_sandboxes_active metric
  |-- Return Sandbox object
```

### Request Flow Through REST API

```
HTTP Request
  |
  v
loggingMiddleware (log + metrics)
  -> corsMiddleware (CORS headers)
    -> jsonContentTypeMiddleware
      -> AuthMiddleware (token validation + audit)
        -> Route handler
          -> Controller method
            -> Runtime operation
              -> Docker Engine API
```

## Security Model

- **RBAC** -- Three roles (admin, operator, viewer) with resource-level permissions. Token-based authentication with constant-time comparison.
- **Container hardening** -- The Docker runtime applies: no-new-privileges, read-only root filesystem, dropped capabilities (ALL dropped, only NET_BIND_SERVICE, CHOWN, SETUID, SETGID, DAC_OVERRIDE added back), `seccomp=unconfined` only when needed.
- **Network isolation** -- Four policies: `none` (no network), `host` (full host access, logged as warning), `bridge` (default Docker bridge), `isolate` (sandbox-specific network with no external access). Matrix members share an internal Docker network.
- **Audit logging** -- Every API request is recorded with user, action, resource, and result (success/denied/error).
- **Input validation** -- Request body size limited to 1 MB, exec output buffered to 10 MB, blueprint validation before container creation.
