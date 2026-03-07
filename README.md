# sandboxMatrix

[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![CI](https://github.com/hgDendi/sandboxmatrix/actions/workflows/ci.yml/badge.svg)](https://github.com/hgDendi/sandboxmatrix/actions/workflows/ci.yml)

> **sandboxMatrix is to AI Agents what Kubernetes is to microservices.**

Open-source, local-first AI sandbox orchestrator with pluggable isolation and MCP integration.

## Why sandboxMatrix

AI coding agents need isolated, reproducible environments to safely execute code, install packages, and manage projects. Current solutions are either cloud-only, require Kubernetes, or lack AI-native features.

| | E2B | Modal | Daytona | DevPod | **sandboxMatrix** |
|---|---|---|---|---|---|
| Open source | Partial | No | Yes | Yes | **Yes** |
| Local-first | No | No | No | Yes | **Yes** |
| MCP integration | No | No | Partial | No | **10 tools** |
| Multi-sandbox orchestration | No | No | No | No | **Matrix** |
| Agent-to-agent messaging | No | No | No | No | **A2A Gateway** |
| Snapshot/restore | Yes | No | No | No | **Yes** |
| GPU passthrough | No | Yes | No | No | **Yes** |
| Pluggable runtimes | No | No | No | No | **Docker/gVisor/Firecracker** |
| REST API + Web Dashboard | Yes | Yes | Yes | No | **Yes** |
| Pre-warmed pools | Yes | Yes | No | No | **Yes** |

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
```

## Features

- **Docker sandbox lifecycle** -- create, start, stop, destroy, exec, and inspect containers
- **Snapshot and restore** -- point-in-time snapshots via Docker commit with tag-based management
- **Matrix orchestration** -- coordinate multiple sandboxes as a single unit with isolated networking
- **MCP server** -- 10 built-in tools for AI agent integration over the Model Context Protocol (stdio)
- **REST API server** -- 20+ JSON endpoints for programmatic access
- **Web dashboard** -- real-time dark-theme management UI with auto-refresh
- **Session management** -- bounded execution contexts for agent workflows with exec tracking
- **A2A messaging** -- agent-to-agent send, receive, and broadcast gateway
- **Pre-warmed pools** -- instant sandbox creation from pre-warmed container pools
- **GPU passthrough** -- NVIDIA GPU support for AI workloads (PyTorch, CUDA)
- **Resource monitoring** -- live CPU and memory statistics per sandbox
- **Blueprint system** -- declarative YAML environment definitions with validation
- **Pluggable runtime architecture** -- Docker, gVisor, Firecracker backends
- **Network policies** -- configurable per-blueprint (none, host, bridge, isolate)
- **Persistent state** -- file-based JSON or BoltDB storage survives restarts
- **SDKs** -- Python and TypeScript clients for programmatic access

## Architecture

```
+--------------------------------------------------------------+
|                      Interface Layer                          |
|  CLI (smx)  |  REST API  |  SDKs (Go/Python/TS)  |  Web UI  |
+--------------------------------------------------------------+
|                      Agent Plane                              |
|  MCP Server  |  MCP Client  |  A2A Gateway                   |
+--------------------------------------------------------------+
|                      Control Plane                            |
|  API Server  |  Scheduler  |  State Manager  |  Pool Manager  |
+--------------------------------------------------------------+
|                      Runtime Plane (pluggable)                |
|  Docker  |  Firecracker  |  gVisor  |  Kata  |  WASM         |
+--------------------------------------------------------------+
|                      Storage Plane                            |
|  Workspaces  |  Snapshots  |  Templates  |  State (JSON/Bolt) |
+--------------------------------------------------------------+
```

## Quick Start

### Build from source

```bash
git clone https://github.com/hgDendi/sandboxmatrix.git
cd sandboxmatrix && make build
```

### Verify installation

```bash
./bin/smx version
```

### Create and use a sandbox

```bash
# Create a sandbox from a blueprint
./bin/smx sandbox create -b blueprints/python-dev.yaml -n my-sandbox
./bin/smx sandbox exec my-sandbox -- python -c "print('hello')"

# Snapshot and restore
./bin/smx sandbox snapshot my-sandbox --tag v1
./bin/smx sandbox snapshots my-sandbox
./bin/smx sandbox restore my-sandbox --snapshot "smx-snapshot/smx-my-sandbox:v1" --name restored

# Matrix orchestration
./bin/smx matrix create fullstack \
  --member api:blueprints/python-dev.yaml \
  --member worker:blueprints/python-dev.yaml
./bin/smx matrix list
./bin/smx matrix inspect fullstack
./bin/smx matrix destroy fullstack

# MCP server for AI agents
./bin/smx mcp serve

# REST API server
./bin/smx server start --addr :8080

# Web dashboard
./bin/smx dashboard --addr :9090

# Pre-warm sandbox pools
./bin/smx pool warm --blueprint blueprints/python-dev.yaml --min 3
./bin/smx pool stats

# Cleanup
./bin/smx sandbox destroy my-sandbox
./bin/smx sandbox destroy restored
```

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
| **Other** | | |
| `smx blueprint validate <file>` | `bp validate` | Validate a blueprint YAML |
| `smx blueprint inspect <file>` | `bp inspect` | Display blueprint details |
| `smx a2a send` | | Send a message between sandboxes |
| `smx a2a receive <sandbox>` | | Receive pending messages |
| `smx a2a broadcast` | | Broadcast to multiple sandboxes |
| `smx version` | | Print version information |

## MCP Tools

The MCP server (`smx mcp serve`) exposes 10 tools over stdio for AI agent integration:

| Tool | Description |
|---|---|
| `sandbox_create` | Create a new sandbox from a blueprint |
| `sandbox_list` | List all sandboxes |
| `sandbox_exec` | Execute a command in a running sandbox (via `sh -c`) |
| `sandbox_start` | Start a stopped sandbox |
| `sandbox_stop` | Stop a running sandbox |
| `sandbox_destroy` | Destroy a sandbox and clean up resources |
| `sandbox_stats` | Get CPU/memory statistics for a running sandbox |
| `a2a_send` | Send a message from one sandbox to another |
| `a2a_receive` | Receive pending messages for a sandbox (clears inbox) |
| `a2a_broadcast` | Broadcast a message to multiple sandboxes |

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

## Roadmap

- [x] **Phase 1** -- CLI scaffolding, Docker sandbox lifecycle, blueprint system
- [x] **Phase 2** -- Snapshot/restore, MCP server, session management, Matrix orchestration
- [x] **Phase 3** -- Network policies, gVisor/Firecracker runtimes, A2A gateway
- [x] **Phase 4** -- REST API server, web dashboard, pre-warmed pools, GPU passthrough, SDKs
- [ ] **Phase 5** -- Multi-node scheduling, K8s operator, distributed state, RBAC

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup and guidelines.

## License

Apache License 2.0 -- see [LICENSE](LICENSE) for details.
