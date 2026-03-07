# sandboxMatrix

[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![CI](https://github.com/hgDendi/sandboxmatrix/actions/workflows/ci.yml/badge.svg)](https://github.com/hgDendi/sandboxmatrix/actions/workflows/ci.yml)

Open-source, local-first AI sandbox orchestrator with pluggable isolation and MCP integration.

## Features

- **Docker sandbox lifecycle** -- create, start, stop, destroy, exec, and inspect containers
- **Snapshot and restore** -- point-in-time snapshots via Docker commit with tag-based management
- **Matrix orchestration** -- coordinate multiple sandboxes as a single unit with shared lifecycle
- **MCP server** -- 10 built-in tools for AI agent integration over the Model Context Protocol (stdio)
- **Session management** -- bounded execution contexts for agent workflows with exec tracking
- **A2A messaging** -- agent-to-agent send, receive, and broadcast gateway
- **Resource monitoring** -- live CPU and memory statistics per sandbox
- **Blueprint system** -- declarative YAML environment definitions with validation and inspection
- **Pluggable runtime architecture** -- designed for Docker, gVisor, Firecracker, and WASM backends
- **File-based persistent state** -- sandbox, session, and matrix state survives restarts
- **Network policies** -- configurable per-blueprint (expose ports, bridge, isolate)

## Architecture

```
┌──────────────────────────────────────────────────────────┐
│                    Interface Layer                        │
│  CLI (smx)  │  REST/gRPC API  │  SDKs  │  Web UI        │
├──────────────────────────────────────────────────────────┤
│                    Agent Plane                           │
│  MCP Server  │  MCP Client  │  A2A Gateway              │
├──────────────────────────────────────────────────────────┤
│                    Control Plane                         │
│  API Server  │  Scheduler  │  State Manager              │
├──────────────────────────────────────────────────────────┤
│                    Runtime Plane (pluggable)             │
│  Docker  │  Firecracker  │  gVisor  │  Kata  │  WASM    │
├──────────────────────────────────────────────────────────┤
│                    Storage Plane                         │
│  Workspaces  │  Snapshots  │  Templates  │  State        │
└──────────────────────────────────────────────────────────┘
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

# Cleanup
./bin/smx sandbox destroy my-sandbox
./bin/smx sandbox destroy restored
```

## CLI Reference

| Command | Alias | Description |
|---|---|---|
| `smx sandbox create` | `sb create` | Create and start a sandbox from a blueprint |
| `smx sandbox list` | `sb ls` | List all sandboxes |
| `smx sandbox start <name>` | | Start a stopped sandbox |
| `smx sandbox stop <name>` | | Stop a running sandbox |
| `smx sandbox destroy <name>` | `sb rm` | Destroy a sandbox |
| `smx sandbox exec <name> -- <cmd>` | | Execute a command in a sandbox |
| `smx sandbox inspect <name>` | | Show detailed sandbox information |
| `smx sandbox stats <name>` | | Show CPU/memory usage for a sandbox |
| `smx sandbox snapshot <name>` | | Create a point-in-time snapshot |
| `smx sandbox snapshots <name>` | | List snapshots of a sandbox |
| `smx sandbox restore <name>` | | Restore a sandbox from a snapshot |
| `smx matrix create <name>` | `mx create` | Create a multi-sandbox matrix |
| `smx matrix list` | `mx ls` | List all matrices |
| `smx matrix inspect <name>` | | Show detailed matrix information |
| `smx matrix start <name>` | | Start all sandboxes in a matrix |
| `smx matrix stop <name>` | | Stop all sandboxes in a matrix |
| `smx matrix destroy <name>` | `mx rm` | Destroy a matrix and all its sandboxes |
| `smx session start <sandbox>` | | Start a new session for a sandbox |
| `smx session end <id>` | | End a session |
| `smx session list` | `session ls` | List sessions (optionally filter by sandbox) |
| `smx session exec <id> -- <cmd>` | | Execute a command within a session |
| `smx blueprint validate <file>` | `bp validate` | Validate a blueprint YAML file |
| `smx blueprint inspect <file>` | `bp inspect` | Display details of a blueprint |
| `smx mcp serve` | | Start the MCP server on stdio |
| `smx a2a send` | | Send a message between sandboxes |
| `smx a2a receive <sandbox>` | | Receive pending messages for a sandbox |
| `smx a2a broadcast` | | Broadcast a message to multiple sandboxes |
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

### MCP configuration example

Add to your AI agent's MCP config (e.g., Claude Desktop):

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

## Core Concepts

| sandboxMatrix | Kubernetes Analog | Purpose |
|---|---|---|
| **Sandbox** | Pod | Isolated dev environment unit |
| **Blueprint** | PodTemplate | Reusable environment definition |
| **Workspace** | PersistentVolume | Project files and state |
| **Toolchain** | Sidecar | Dev tools (LSP, git, compilers) |
| **Session** | Job | Bounded AI agent execution context |
| **Matrix** | Namespace | Group of coordinated sandboxes |

## Blueprint Example

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

Matrix blueprints can define multiple coordinated members:

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
make e2e          # Run end-to-end tests
make clean        # Remove build artifacts
```

## Roadmap

- [x] **Phase 1** -- CLI scaffolding, Docker sandbox lifecycle, blueprint system
- [x] **Phase 2** -- Snapshot/restore, resource stats, sandbox inspect
- [x] **Phase 3** -- MCP server, Matrix orchestration, session management, A2A gateway
- [ ] **Phase 4** -- Firecracker/gVisor backends, network policies, REST API, SDKs
- [ ] **Phase 5** -- Multi-node scheduling, GPU passthrough, K8s operator, web dashboard

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup and guidelines.

## License

Apache License 2.0 -- see [LICENSE](LICENSE) for details.
