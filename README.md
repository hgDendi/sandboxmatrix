# sandboxMatrix

**Open-source, local-first AI sandbox orchestrator.**

sandboxMatrix brings Kubernetes-inspired concepts to AI development workflows with pluggable isolation backends and first-class MCP integration.

## Features

- **Local-first** — no cloud dependency required
- **Pluggable isolation** — Docker, Firecracker, gVisor, WASM backends
- **AI-native** — built-in MCP server for AI agent integration
- **Multi-sandbox orchestration** — Matrix concept for coordinated workflows
- **Blueprint system** — reusable, declarative environment definitions (YAML)

## Architecture

```
┌──────────────────────────────────────────────────────────┐
│                    Interface Layer                         │
│  CLI (smx)  │  REST/gRPC API  │  SDKs  │  Web UI         │
├──────────────────────────────────────────────────────────┤
│                    Agent Plane                            │
│  MCP Server  │  MCP Client  │  A2A Gateway               │
├──────────────────────────────────────────────────────────┤
│                    Control Plane                          │
│  API Server  │  Scheduler  │  State Manager               │
├──────────────────────────────────────────────────────────┤
│                    Runtime Plane (pluggable)              │
│  Docker  │  Firecracker  │  gVisor  │  Kata  │  WASM     │
├──────────────────────────────────────────────────────────┤
│                    Storage Plane                          │
│  Workspaces  │  Snapshots  │  Templates  │  State         │
└──────────────────────────────────────────────────────────┘
```

## Quick Start

### Install from source

```bash
git clone https://github.com/hg-dendi/sandboxmatrix.git
cd sandboxmatrix
make build
```

### Verify installation

```bash
./bin/smx version
```

### Create a sandbox from a blueprint

```bash
# Validate a blueprint
smx blueprint validate blueprints/python-dev.yaml

# Create and start a sandbox (requires Docker)
smx sandbox create --blueprint python-dev --name my-sandbox
smx sandbox exec my-sandbox -- python -c "print('hello from sandbox')"
smx sandbox list
smx sandbox stop my-sandbox
smx sandbox destroy my-sandbox
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

## Development

```bash
make build      # Build the smx binary
make test       # Run all tests
make lint       # Run linters
make install    # Install to $GOPATH/bin
```

## Roadmap

- **v0.1.0** — CLI + Docker sandbox management + Blueprints
- **v0.3.0** — MCP server, Matrix orchestration, snapshot/restore, API server
- **v0.5.0** — Firecracker/gVisor backends, network policies, SDKs
- **v1.0.0** — Multi-node, GPU passthrough, K8s operator, web dashboard

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup and guidelines.

## License

Apache License 2.0 — see [LICENSE](LICENSE) for details.
