# sandboxMatrix

[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![CI](https://github.com/hgDendi/sandboxmatrix/actions/workflows/ci.yml/badge.svg)](https://github.com/hgDendi/sandboxmatrix/actions/workflows/ci.yml)

> **sandboxMatrix is to AI Agents what Kubernetes is to microservices.**

Open-source, local-first AI sandbox orchestrator with pluggable isolation and MCP integration.

## Why sandboxMatrix

AI coding agents need isolated, reproducible environments to safely execute code, install packages, and manage projects. Current solutions are either cloud-only, require Kubernetes, or lack AI-native features.

| | E2B | Modal | Daytona | DevPod | **sandboxMatrix** |
|---|:---:|:---:|:---:|:---:|:---:|
| Open source | Partial | ❌ | ✅ | ✅ | ✅ |
| Local-first | ❌ | ❌ | ❌ | ✅ | ✅ |
| MCP integration | ✅ | ❌ | Partial | ❌ | ✅ 13 tools |
| Multi-sandbox orchestration | ❌ | ❌ | ❌ | ❌ | ✅ Matrix |
| Agent-to-agent messaging | ❌ | ❌ | ❌ | ❌ | ✅ A2A Gateway |
| Task sharding + aggregation | ❌ | ❌ | ❌ | ❌ | ✅ |
| Readiness probes | ❌ | ❌ | ❌ | ❌ | ✅ exec/HTTP/TCP |
| Device passthrough | ❌ | ❌ | ❌ | ❌ | ✅ /dev/kvm, /dev/dri |
| Snapshot/restore | ✅ | ❌ | ❌ | ❌ | ✅ |
| GPU passthrough | ❌ | ✅ | ❌ | ❌ | ✅ |
| Pluggable runtimes | ❌ | ❌ | ❌ | ❌ | ✅ Docker/gVisor/Firecracker |
| REST API + Web Dashboard | ✅ | ✅ | ✅ | ❌ | ✅ |
| Pre-warmed pools | ✅ | ✅ | ❌ | ❌ | ✅ |
| RBAC + Audit logging | ✅ | ✅ | ❌ | ❌ | ✅ |
| WebSocket exec streaming | ❌ | ❌ | ❌ | ❌ | ✅ |
| Prometheus metrics | ❌ | ✅ | ❌ | ❌ | ✅ |
| K8s Operator + Helm | ❌ | ❌ | ❌ | ❌ | ✅ |
| Distributed state (etcd) | ➖ | ➖ | ❌ | ❌ | ✅ |

## Cloud Native Mapping

sandboxMatrix brings Kubernetes-inspired concepts to AI agent workflows:

```
Cloud Native              AI Agent Development
──────────────            ──────────────────────
Kubernetes         ->     sandboxMatrix
Pod                ->     Sandbox
Namespace          ->     Matrix
PersistentVolume   ->     Workspace
Job                ->     Session
CRI (containerd)   ->     Runtime Interface (Docker/gVisor/Firecracker)
Helm Chart         ->     Blueprint
RBAC               ->     RBAC (admin/operator/viewer)
etcd               ->     etcd (distributed state)
CRD + Operator     ->     CRD + Operator
Readiness Probe    ->     Readiness Probe (exec/HTTP/TCP)
Device Plugin      ->     Device Passthrough (/dev/kvm, /dev/dri)
Job Parallelism    ->     Task Sharding (round-robin/hash/balanced)
```

## Features

- **Docker sandbox lifecycle** -- create, start, stop, destroy, exec, and inspect containers
- **Snapshot and restore** -- point-in-time snapshots via Docker commit with tag-based management
- **Matrix orchestration** -- coordinate multiple sandboxes as a single unit with isolated networking
- **MCP server** -- 13 built-in tools for AI agent integration over the Model Context Protocol (stdio)
- **REST API server** -- 20+ JSON endpoints for programmatic access
- **Web dashboard** -- real-time dark-theme management UI with auto-refresh
- **Session management** -- bounded execution contexts for agent workflows with exec tracking
- **A2A messaging** -- agent-to-agent send, receive, and broadcast gateway
- **Pre-warmed pools** -- instant sandbox creation from pre-warmed container pools
- **GPU passthrough** -- NVIDIA GPU support for AI workloads (PyTorch, CUDA)
- **Device passthrough** -- pass host devices (`/dev/kvm`, `/dev/dri`, etc.) into sandboxes with optional mode
- **Readiness probes** -- exec/HTTP/TCP probes to wait for sandbox readiness (K8s-style)
- **Task sharding** -- distribute tasks across matrix members with round-robin, hash, or balanced strategies
- **Result aggregation** -- collect and merge results from distributed matrix tasks
- **Resource monitoring** -- live CPU and memory statistics per sandbox
- **WebSocket exec streaming** -- real-time stdout/stderr streaming with stdin support for long-running commands
- **Blueprint system** -- declarative YAML environment definitions with validation
- **Pluggable runtime architecture** -- Docker, gVisor, Firecracker backends
- **Network policies** -- configurable per-blueprint (none, host, bridge, isolate)
- **RBAC** -- role-based access control (admin/operator/viewer) with token auth
- **Audit logging** -- every action recorded with user, resource, and result
- **Observability** -- structured logging (slog/JSON), Prometheus metrics (`/metrics` endpoint)
- **Persistent state** -- file-based JSON, BoltDB, or etcd for distributed deployments
- **Kubernetes operator** -- CRDs for Sandbox/Matrix/Blueprint with Helm chart
- **SDKs** -- Python and TypeScript clients for programmatic access

## Architecture

```
+----------------------------------------------------------------------+
|                        Interface Layer                                 |
|  CLI (smx)  |  REST API  |  SDKs (Go/Python/TS)  |  Web Dashboard   |
+----------------------------------------------------------------------+
|                        Agent Plane                                     |
|  MCP Server (13 tools)  |  A2A Gateway  |  Session Manager            |
+----------------------------------------------------------------------+
|                        Control Plane                                   |
|  API Server  |  Scheduler  |  Pool Manager  |  RBAC + Audit + Metrics|
+----------------------------------------------------------------------+
|                        Orchestration Plane                               |
|  Readiness Probes  |  Task Sharding  |  Result Aggregation            |
+----------------------------------------------------------------------+
|                        Runtime Plane (pluggable)                       |
|  Docker  |  Firecracker  |  gVisor  |  Kata  |  WASM                 |
+----------------------------------------------------------------------+
|                        Storage Plane                                   |
|  Workspaces  |  Snapshots  |  State (JSON / BoltDB / etcd)           |
+----------------------------------------------------------------------+
|                        Deployment                                      |
|  Single binary  |  K8s Operator + CRDs  |  Helm Chart                |
+----------------------------------------------------------------------+
```

## Quick Start

```bash
# Build
git clone https://github.com/hgDendi/sandboxmatrix.git
cd sandboxmatrix && make build

# Create and use a sandbox
./bin/smx sandbox create -b blueprints/python-dev.yaml -n my-sandbox
./bin/smx sandbox exec my-sandbox -- python -c "print('hello from sandbox')"

# Snapshot and restore
./bin/smx sandbox snapshot my-sandbox --tag v1
./bin/smx sandbox restore my-sandbox --snapshot "smx-snapshot/smx-my-sandbox:v1" --name restored

# Cleanup
./bin/smx sandbox destroy my-sandbox
./bin/smx sandbox destroy restored
```

### Start services

```bash
./bin/smx mcp serve                  # MCP server for AI agents (stdio)
./bin/smx server start --addr :8080  # REST API server
./bin/smx dashboard --addr :9090     # Web dashboard with terminal
```

### Matrix orchestration

```bash
./bin/smx matrix create fullstack \
  --member api:blueprints/python-dev.yaml \
  --member worker:blueprints/python-dev.yaml
./bin/smx matrix inspect fullstack
./bin/smx matrix destroy fullstack
```

> **More examples:** [API Reference](docs/api-reference.md) | [SDK Guide](docs/sdk-guide.md) | [Deployment Guide](docs/deployment.md)

## CLI Reference

| Command | Alias | Description |
|---|---|---|
| **Sandbox** | | |
| `smx sandbox create` | `sb create` | Create and start a sandbox from a blueprint |
| `smx sandbox list` | `sb ls` | List all sandboxes |
| `smx sandbox start <name>` | | Start a stopped sandbox |
| `smx sandbox stop <name>` | | Stop a running sandbox |
| `smx sandbox destroy <name>` | `sb rm` | Destroy a sandbox |
| `smx sandbox exec <name> -- <cmd>` | | Execute a command in a sandbox |
| `smx sandbox inspect <name>` | | Show detailed sandbox information |
| `smx sandbox stats <name>` | | Show CPU/memory usage |
| `smx sandbox snapshot <name>` | | Create a point-in-time snapshot |
| `smx sandbox snapshots <name>` | | List snapshots |
| `smx sandbox restore <name>` | | Restore from a snapshot |
| `smx sandbox gpu-check <name>` | | Check GPU availability (nvidia-smi) |
| **Matrix** | | |
| `smx matrix create <name>` | `mx create` | Create a multi-sandbox matrix |
| `smx matrix list` | `mx ls` | List all matrices |
| `smx matrix inspect <name>` | | Show matrix details |
| `smx matrix start <name>` | | Start all sandboxes in a matrix |
| `smx matrix stop <name>` | | Stop all sandboxes in a matrix |
| `smx matrix destroy <name>` | `mx rm` | Destroy matrix and all sandboxes |
| **Session** | | |
| `smx session start <sandbox>` | | Start a new session |
| `smx session end <id>` | | End a session |
| `smx session list` | `session ls` | List sessions |
| `smx session exec <id> -- <cmd>` | | Execute within a session |
| **Pool** | | |
| `smx pool warm` | | Pre-warm sandbox instances |
| `smx pool stats` | | Show pool statistics |
| `smx pool drain` | | Destroy all warm instances |
| **Server** | | |
| `smx server start` | | Start the REST API server |
| `smx dashboard` | | Start the web dashboard |
| `smx mcp serve` | | Start the MCP server (stdio) |
| **Auth** | | |
| `smx auth add-user <name>` | | Add user with role and generate token |
| `smx auth list-users` | | List all users |
| `smx auth remove-user <name>` | | Remove a user |
| `smx auth audit` | | View audit log |
| **Config** | | |
| `smx config show` | | Display current configuration |
| `smx config set <key> <value>` | | Set a configuration value |
| `smx config init` | | Create default config file |
| `smx config path` | | Show config file path |
| **Kubernetes** | | |
| `smx operator start` | | Start the K8s operator (scaffold) |
| **Other** | | |
| `smx blueprint validate <file>` | `bp validate` | Validate a blueprint YAML |
| `smx blueprint inspect <file>` | `bp inspect` | Display blueprint details |
| `smx a2a send` | | Send a message between sandboxes |
| `smx a2a receive <sandbox>` | | Receive pending messages |
| `smx a2a broadcast` | | Broadcast to multiple sandboxes |
| `smx version` | | Print version information |

## MCP Tools

The MCP server (`smx mcp serve`) exposes 13 tools over stdio for AI agent integration:

| Tool | Description |
|---|---|
| `sandbox_create` | Create a new sandbox from a blueprint |
| `sandbox_list` | List all sandboxes |
| `sandbox_exec` | Execute a command in a running sandbox (via `sh -c`) |
| `sandbox_start` | Start a stopped sandbox |
| `sandbox_stop` | Stop a running sandbox |
| `sandbox_destroy` | Destroy a sandbox and clean up resources |
| `sandbox_stats` | Get CPU/memory statistics for a running sandbox |
| `sandbox_ready_wait` | Wait for a sandbox to pass its readiness probe |
| `a2a_send` | Send a message from one sandbox to another |
| `a2a_receive` | Receive pending messages for a sandbox (clears inbox) |
| `a2a_broadcast` | Broadcast a message to multiple sandboxes |
| `matrix_shard_task` | Distribute tasks across matrix members (round-robin/hash/balanced) |
| `matrix_collect_results` | Collect and aggregate task results from matrix members |

### MCP configuration

Add to your AI agent's MCP config (e.g., Claude Code, Claude Desktop):

```json
{
  "mcpServers": {
    "sandboxmatrix": {
      "command": "/path/to/smx",
      "args": ["mcp", "serve"]
    }
  }
}
```

## WebSocket Exec Streaming

For real-time command output, connect via WebSocket:

```
GET /api/v1/sandboxes/{name}/exec/stream
```

Protocol:
1. Client connects via WebSocket
2. Client sends: `{"command": ["sh", "-c", "your-command"]}`
3. Server streams: `{"type":"stdout","data":"..."}` and `{"type":"stderr","data":"..."}`
4. Client can send stdin: `{"type":"stdin","data":"..."}`
5. On completion: `{"type":"exit","exitCode":0}`

## Observability

### Prometheus Metrics

The REST API server exposes Prometheus metrics at `GET /metrics`:

| Metric | Type | Description |
|---|---|---|
| `smx_sandboxes_active` | Gauge | Number of active (running/ready) sandboxes |
| `smx_sandbox_operations_total` | Counter | Total sandbox operations by operation and result |
| `smx_sandbox_operation_duration_seconds` | Histogram | Duration of sandbox operations |
| `smx_exec_total` | Counter | Total exec commands by sandbox and result |
| `smx_exec_duration_seconds` | Histogram | Duration of exec commands |
| `smx_sessions_active` | Gauge | Number of active sessions |
| `smx_http_requests_total` | Counter | HTTP API requests by method, path, status |
| `smx_http_request_duration_seconds` | Histogram | HTTP request duration |
| `smx_websocket_connections` | Gauge | Active WebSocket connections |

### Structured Logging

All components use Go's `log/slog` with JSON output for structured logging.

## Blueprint Examples

Sandbox blueprint:

```yaml
apiVersion: smx/v1alpha1
kind: Blueprint
metadata:
  name: python-dev
  version: "1.0.0"
spec:
  base: python:3.12-slim
  runtime: docker
  resources:
    cpu: "2"
    memory: 2Gi
    disk: 10Gi
  setup:
    - run: pip install poetry ruff mypy
  toolchains:
    - name: python-lsp
      image: smx-toolchains/pylsp:latest
  workspace:
    mountPath: /workspace
  network:
    expose: [8000]
```

GPU-enabled blueprint:

```yaml
apiVersion: smx/v1alpha1
kind: Blueprint
metadata:
  name: gpu-python-dev
  version: "1.0.0"
spec:
  base: nvidia/cuda:12.4.1-runtime-ubuntu22.04
  runtime: docker
  resources:
    cpu: "4"
    memory: 8Gi
    gpu:
      count: 1
      driver: nvidia
  setup:
    - run: pip3 install torch torchvision
  workspace:
    mountPath: /workspace
```

Blueprint with device passthrough and readiness probe:

```yaml
apiVersion: smx/v1alpha1
kind: Blueprint
metadata:
  name: android-emulator
  version: "1.0.0"
spec:
  base: budtmo/docker-android:emulator_14.0
  runtime: docker
  resources:
    cpu: "4"
    memory: 8Gi
  devices:
    - hostPath: /dev/kvm
      permissions: "rw"
    - hostPath: /dev/dri
      permissions: "rw"
      optional: true
  readinessProbe:
    type: exec
    command: ["adb", "shell", "getprop", "sys.boot_completed"]
    initialDelaySec: 10
    periodSec: 3
    timeoutSec: 5
    successThreshold: 1
    failureThreshold: 40
  network:
    expose: [5554, 5555]
```

Matrix blueprint:

```yaml
apiVersion: smx/v1alpha1
kind: Matrix
metadata:
  name: fullstack
members:
  - name: frontend
    blueprint: blueprints/node-dev.yaml
  - name: backend
    blueprint: blueprints/python-dev.yaml
```

## Development

```bash
make build        # Build the smx binary to bin/
make install      # Install smx to $GOPATH/bin
make test         # Run all tests
make test-race    # Run tests with race detector
make test-cover   # Run tests with coverage report
make lint         # Run golangci-lint
make fmt          # Format code (go fmt + goimports)
make vet          # Run go vet
make e2e          # Run end-to-end tests (requires Docker)
make clean        # Remove build artifacts
```

## Kubernetes Deployment

Deploy sandboxMatrix as a K8s operator:

```bash
# Install CRDs
kubectl apply -f deploy/crds/

# Deploy with kustomize
kubectl apply -k deploy/

# Or deploy with Helm
helm install sandboxmatrix deploy/helm/sandboxmatrix/
```

Then manage sandboxes declaratively:

```yaml
apiVersion: smx.sandboxmatrix.dev/v1alpha1
kind: Sandbox
metadata:
  name: my-sandbox
spec:
  blueprintRef: python-dev
  resources:
    cpu: "2"
    memory: 2Gi
```

## Documentation

| Document | Description |
|---|---|
| [Architecture](docs/architecture.md) | System design, core components, data flow |
| [API Reference](docs/api-reference.md) | All 24 REST API endpoints with examples |
| [Deployment Guide](docs/deployment.md) | Local, Docker, K8s deployment + monitoring |
| [SDK Guide](docs/sdk-guide.md) | Python & TypeScript SDK usage |

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup and guidelines.

## License

Apache License 2.0 -- see [LICENSE](LICENSE) for details.
