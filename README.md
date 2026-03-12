# sandboxMatrix

[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![CI](https://github.com/hgDendi/sandboxmatrix/actions/workflows/ci.yml/badge.svg)](https://github.com/hgDendi/sandboxmatrix/actions/workflows/ci.yml)

> **sandboxMatrix is to AI Agents what Kubernetes is to microservices.**

Open-source, local-first AI sandbox orchestrator with pluggable isolation, MCP integration, and AI framework adapters.

## Why sandboxMatrix

AI coding agents need isolated, reproducible environments to safely execute code, install packages, and manage projects. Current solutions are either cloud-only, require Kubernetes, or lack AI-native features.

| | E2B | Modal | Daytona | DevPod | **sandboxMatrix** |
|---|:---:|:---:|:---:|:---:|:---:|
| Open source | Partial | ❌ | ✅ | ✅ | ✅ |
| Local-first | ❌ | ❌ | ❌ | ✅ | ✅ |
| MCP integration | ✅ | ❌ | Partial | ❌ | ✅ 19 tools |
| Code interpreter | ✅ | ❌ | ❌ | ❌ | ✅ 5 languages |
| File upload/download API | ✅ | ❌ | ❌ | ❌ | ✅ |
| LangChain / CrewAI adapters | ❌ | ❌ | ❌ | ❌ | ✅ |
| Multi-sandbox orchestration | ❌ | ❌ | ❌ | ❌ | ✅ Matrix |
| Agent-to-agent messaging | ❌ | ❌ | ❌ | ❌ | ✅ A2A Gateway |
| Task sharding + aggregation | ❌ | ❌ | ❌ | ❌ | ✅ |
| Blueprint inheritance | ❌ | ❌ | ❌ | ❌ | ✅ extends |
| Env/Secrets injection | ✅ | ✅ | ❌ | ❌ | ✅ env/file/literal |
| Image pre-building | ✅ | ✅ | ❌ | ❌ | ✅ |
| Readiness probes | ❌ | ❌ | ❌ | ❌ | ✅ exec/HTTP/TCP |
| Device passthrough | ❌ | ❌ | ❌ | ❌ | ✅ /dev/kvm, /dev/dri |
| Snapshot/restore | ✅ | ❌ | ❌ | ❌ | ✅ |
| GPU passthrough | ❌ | ✅ | ❌ | ❌ | ✅ |
| Pluggable runtimes | ❌ | ❌ | ❌ | ❌ | ✅ Docker/gVisor/Firecracker |
| REST API + Web Dashboard | ✅ | ✅ | ✅ | ❌ | ✅ 40+ endpoints |
| SDKs | Python/TS | Python | ❌ | ❌ | ✅ Go/Python/TS |
| Pre-warmed pools | ✅ | ✅ | ❌ | ❌ | ✅ |
| RBAC + Audit logging | ✅ | ✅ | ❌ | ❌ | ✅ |
| WebSocket exec streaming | ❌ | ❌ | ❌ | ❌ | ✅ |
| Prometheus metrics | ❌ | ✅ | ❌ | ❌ | ✅ |
| SSO/OIDC authentication | ❌ | ❌ | ❌ | ❌ | ✅ Google/GitHub/OIDC |
| Team namespaces | ❌ | ❌ | ❌ | ❌ | ✅ |
| Resource quotas | ❌ | ❌ | ❌ | ❌ | ✅ CPU/mem/GPU/count |
| Distributed tracing | ❌ | ❌ | ❌ | ❌ | ✅ OpenTelemetry |
| Grafana dashboards | ❌ | ✅ | ❌ | ❌ | ✅ 3 built-in |
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
Helm Chart         ->     Blueprint (with extends inheritance)
ConfigMap/Secret   ->     Blueprint env/secrets
RBAC               ->     RBAC (admin/operator/viewer)
OIDC/SSO           ->     SSO (Google/GitHub/OIDC)
Namespace Quotas   ->     Team Quotas (CPU/mem/GPU/count)
etcd               ->     etcd (distributed state)
CRD + Operator     ->     CRD + Operator
OpenTelemetry      ->     Distributed Tracing (OTLP)
Grafana            ->     Grafana Dashboards (3 built-in)
Readiness Probe    ->     Readiness Probe (exec/HTTP/TCP)
Device Plugin      ->     Device Passthrough (/dev/kvm, /dev/dri)
Job Parallelism    ->     Task Sharding (round-robin/hash/balanced)
```

## Features

### Core Sandbox
- **Docker sandbox lifecycle** -- create, start, stop, destroy, exec, and inspect containers
- **Snapshot and restore** -- point-in-time snapshots via Docker commit with tag-based management
- **File upload/download** -- read, write, and list files inside running sandboxes via Docker cp API
- **Code interpreter** -- execute Python, JavaScript, Bash, Go, and Rust code with structured output
- **Env/Secrets injection** -- blueprint-level env vars and secrets from host env, files, or literals
- **Resource monitoring** -- live CPU and memory statistics per sandbox
- **GPU passthrough** -- NVIDIA GPU support for AI workloads (PyTorch, CUDA)
- **Device passthrough** -- pass host devices (`/dev/kvm`, `/dev/dri`, etc.) into sandboxes

### Multi-Sandbox Orchestration
- **Matrix orchestration** -- coordinate multiple sandboxes as a single unit with isolated networking
- **A2A messaging** -- agent-to-agent send, receive, and broadcast gateway
- **Task sharding** -- distribute tasks across matrix members (round-robin, hash, balanced)
- **Result aggregation** -- collect and merge results from distributed matrix tasks
- **Service discovery** -- `<member>.<matrix>.local` hostname resolution between matrix members
- **Port forwarding** -- inspect and manage port mappings for sandbox services

### Blueprint System
- **Declarative YAML** -- define environments with base image, resources, setup, networking
- **Blueprint inheritance** -- `extends` field for DRY configuration with multi-level support (max 5 depth)
- **Image pre-building** -- bake blueprints into Docker images for instant sandbox creation
- **Readiness probes** -- exec/HTTP/TCP probes to wait for sandbox readiness (K8s-style)
- **Network policies** -- configurable per-blueprint (none, host, bridge, isolate)
- **11 example blueprints** -- Python, Go, Node, Rust, GPU, Android, fullstack, data-science, web-api

### AI Integration
- **MCP server** -- 19 built-in tools for AI agent integration over Model Context Protocol (stdio)
- **LangChain adapter** -- 4 tools (exec, write_file, read_file, code_interpreter) via `pip install sandboxmatrix[langchain]`
- **CrewAI adapter** -- 4 tools with same capabilities via `pip install sandboxmatrix[crewai]`
- **Session management** -- bounded execution contexts for agent workflows with exec tracking

### Platform
- **REST API server** -- 40+ JSON endpoints for programmatic access
- **Web dashboard** -- real-time dark-theme management UI with terminal
- **SDKs** -- Go, Python, and TypeScript clients with full API coverage
- **WebSocket exec streaming** -- real-time stdout/stderr streaming with stdin support
- **Pre-warmed pools** -- instant sandbox creation from pre-warmed container pools
- **RBAC** -- role-based access control (admin/operator/viewer) with token auth
- **SSO/OIDC authentication** -- Google, GitHub, and generic OIDC IdP support with JWT tokens and auto user creation
- **Team namespaces + resource quotas** -- teams with members, per-team limits on CPU, memory, GPU, sandbox/matrix counts
- **Audit logging** -- every action recorded with user, resource, and result
- **Observability** -- structured logging (slog/JSON), Prometheus metrics, OpenTelemetry distributed tracing
- **Grafana dashboards** -- 3 ready-to-import dashboards (Overview, Performance, Resources) with provisioning config
- **Persistent state** -- file-based JSON, BoltDB, or etcd for distributed deployments
- **Pluggable runtimes** -- Docker, gVisor, Firecracker backends
- **Kubernetes operator** -- CRDs for Sandbox/Matrix/Blueprint with Helm chart

## Architecture

```
+----------------------------------------------------------------------+
|                        Interface Layer                                |
|  CLI (smx)  |  REST API  |  SDKs (Go/Python/TS)  |  Web Dashboard   |
+----------------------------------------------------------------------+
|                        Agent Plane                                    |
|  MCP Server (19 tools)  |  A2A Gateway  |  Session Manager           |
|  LangChain Adapter      |  CrewAI Adapter                            |
+----------------------------------------------------------------------+
|                        Control Plane                                  |
|  API Server  |  Scheduler  |  Pool Manager  |  RBAC + SSO/OIDC       |
|  Team Quotas |  Audit Log  |  Metrics       |  OpenTelemetry Tracing |
+----------------------------------------------------------------------+
|                        Orchestration Plane                            |
|  Readiness Probes  |  Task Sharding  |  Result Aggregation           |
|  Service Discovery |  Port Forwarding |  Image Builder               |
+----------------------------------------------------------------------+
|                        Runtime Plane (pluggable)                      |
|  Docker  |  Firecracker  |  gVisor  |  Kata  |  WASM                |
+----------------------------------------------------------------------+
|                        Storage Plane                                  |
|  Workspaces  |  Snapshots  |  State (JSON / BoltDB / etcd)          |
+----------------------------------------------------------------------+
|                        Deployment                                     |
|  Single binary  |  K8s Operator + CRDs  |  Helm Chart               |
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

# Create with environment variables
./bin/smx sandbox create -b blueprints/env-example.yaml -n my-app -e API_KEY=sk-xxx -e DEBUG=true

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

### Image pre-building

```bash
# Build a blueprint into a cached Docker image (skip setup on future creates)
./bin/smx image build -b blueprints/python-dev.yaml
./bin/smx image list
./bin/smx image clean
```

### Matrix orchestration

```bash
./bin/smx matrix create fullstack \
  --member api:blueprints/python-dev.yaml \
  --member worker:blueprints/python-dev.yaml
./bin/smx matrix inspect fullstack
./bin/smx matrix destroy fullstack
```

### Code interpreter (via REST API)

```bash
curl -X POST http://localhost:8080/api/v1/sandboxes/my-sandbox/interpret \
  -H "Content-Type: application/json" \
  -d '{"language":"python","code":"import math; print(math.pi)"}'
```

### File operations (via REST API)

```bash
# Upload a file
curl -X PUT "http://localhost:8080/api/v1/sandboxes/my-sandbox/files?path=/workspace/main.py" \
  -H "Content-Type: application/octet-stream" \
  --data-binary @main.py

# Download a file
curl "http://localhost:8080/api/v1/sandboxes/my-sandbox/files?path=/workspace/main.py"

# List files
curl "http://localhost:8080/api/v1/sandboxes/my-sandbox/files/list?path=/workspace"
```

### Authentication & Team API routes

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/v1/auth/oidc/login` | OIDC login redirect |
| `GET` | `/api/v1/auth/oidc/callback` | OIDC callback |
| `POST` | `/api/v1/auth/token/refresh` | Refresh JWT token |
| `GET` | `/api/v1/auth/userinfo` | Current user info |
| `POST` | `/api/v1/teams` | Create team |
| `GET` | `/api/v1/teams` | List teams |
| `GET` | `/api/v1/teams/{name}` | Get team |
| `PUT` | `/api/v1/teams/{name}` | Update team |
| `DELETE` | `/api/v1/teams/{name}` | Delete team |
| `GET` | `/api/v1/teams/{name}/usage` | Team resource usage |
| `GET` | `/api/v1/teams/{name}/members` | List members |
| `POST` | `/api/v1/teams/{name}/members` | Add member |
| `DELETE` | `/api/v1/teams/{name}/members/{user}` | Remove member |

> **More examples:** [API Reference](docs/api-reference.md) | [SDK Guide](docs/sdk-guide.md) | [Deployment Guide](docs/deployment.md)

## SDK Usage

### Go SDK

```go
import smx "github.com/hg-dendi/sandboxmatrix/sdk/go"

client := smx.NewClient("http://localhost:8080")

sb, _ := client.CreateSandbox(ctx, "my-sandbox", "blueprints/python-dev.yaml", "")
result, _ := client.Exec(ctx, "my-sandbox", "python -c 'print(42)'")
fmt.Println(result.Stdout)

_ = client.WriteFile(ctx, "my-sandbox", "/workspace/hello.py", strings.NewReader("print('hi')"))
data, _ := client.ReadFile(ctx, "my-sandbox", "/workspace/hello.py")

_ = client.DestroySandbox(ctx, "my-sandbox")
```

### Python SDK

```python
from sandboxmatrix import HTTPClient

client = HTTPClient("http://localhost:8080")

sb = client.create_sandbox("my-sandbox", "blueprints/python-dev.yaml")
result = client.exec("my-sandbox", "python -c 'print(42)'")
print(result.stdout)

# Code interpreter
interp = client.interpret("my-sandbox", "python", "import math; print(math.pi)")
print(interp.stdout)

# File operations
client.write_file("my-sandbox", "/workspace/hello.py", "print('hi')")
content = client.read_file("my-sandbox", "/workspace/hello.py")
files = client.list_files("my-sandbox", "/workspace")

client.destroy_sandbox("my-sandbox")
```

### TypeScript SDK

```typescript
import { HTTPClient } from "@sandboxmatrix/sdk";

const client = new HTTPClient({ baseURL: "http://localhost:8080" });

const sb = await client.createSandbox("my-sandbox", "blueprints/python-dev.yaml");
const result = await client.exec("my-sandbox", "python -c 'print(42)'");
console.log(result.stdout);

// Code interpreter
const interp = await client.interpret("my-sandbox", "python", "import math; print(math.pi)");

// File operations
await client.writeFile("my-sandbox", "/workspace/hello.py", "print('hi')");
const content = await client.readFile("my-sandbox", "/workspace/hello.py");
const files = await client.listFiles("my-sandbox", "/workspace");

await client.destroySandbox("my-sandbox");
```

### LangChain Integration

```python
# pip install sandboxmatrix[langchain]
from sandboxmatrix.langchain_tools import create_sandbox_tools

tools = create_sandbox_tools(sandbox_name="my-sandbox")
# Returns: [SandboxExecTool, SandboxWriteFileTool, SandboxReadFileTool, CodeInterpreterTool]

# Use with any LangChain agent
from langchain.agents import create_react_agent
agent = create_react_agent(llm, tools)
```

### CrewAI Integration

```python
# pip install sandboxmatrix[crewai]
from sandboxmatrix.crewai_tools import SandboxExecTool, CodeInterpreterTool

exec_tool = SandboxExecTool(sandbox_name="my-sandbox")
code_tool = CodeInterpreterTool(sandbox_name="my-sandbox")

agent = Agent(tools=[exec_tool, code_tool], ...)
```

## CLI Reference

| Command | Alias | Description |
|---|---|---|
| **Sandbox** | | |
| `smx sandbox create -b <bp> -n <name>` | `sb create` | Create and start a sandbox from a blueprint |
| `smx sandbox create ... -e KEY=VAL` | | Create with environment variable injection |
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
| **Image** | | |
| `smx image build -b <blueprint>` | | Build a cached Docker image from a blueprint |
| `smx image list` | `image ls` | List all pre-built images |
| `smx image clean` | | Remove all pre-built images |
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
| **Team** | | |
| `smx team create <name>` | | Create a team with optional quota flags |
| `smx team list` | `team ls` | List all teams |
| `smx team inspect <name>` | | Show team info and current resource usage |
| `smx team add-member <team> <user>` | | Add a member with `--role` |
| `smx team remove-member <team> <user>` | | Remove a member from the team |
| `smx team set-quota <name>` | | Update team resource quotas |
| **Auth** | | |
| `smx auth add-user <name>` | | Add user with role and generate token |
| `smx auth list-users` | | List all users |
| `smx auth remove-user <name>` | | Remove a user |
| `smx auth audit` | | View audit log |
| `smx auth oidc-config` | | Show or configure OIDC settings |
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

The MCP server (`smx mcp serve`) exposes 19 tools over stdio for AI agent integration:

| Tool | Description |
|---|---|
| `sandbox_create` | Create a new sandbox from a blueprint (supports env vars) |
| `sandbox_list` | List all sandboxes |
| `sandbox_exec` | Execute a command in a running sandbox (via `sh -c`) |
| `sandbox_start` | Start a stopped sandbox |
| `sandbox_stop` | Stop a running sandbox |
| `sandbox_destroy` | Destroy a sandbox and clean up resources |
| `sandbox_stats` | Get CPU/memory statistics for a running sandbox |
| `sandbox_ready_wait` | Wait for a sandbox to pass its readiness probe |
| `sandbox_write_file` | Write content to a file inside a sandbox |
| `sandbox_read_file` | Read content from a file inside a sandbox |
| `sandbox_ports` | List exposed ports and host mappings for a sandbox |
| `code_interpret` | Execute code (Python/JS/Bash/Go/Rust) with structured output |
| `blueprint_build` | Build a Docker image from a blueprint for faster creation |
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

## Blueprint System

### Basic blueprint

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
  setup:
    - run: pip install poetry ruff mypy
  workspace:
    mountPath: /workspace
  network:
    expose: [8000]
```

### Blueprint with env/secrets

```yaml
apiVersion: smx/v1alpha1
kind: Blueprint
metadata:
  name: web-service
  version: "1.0.0"
spec:
  base: python:3.12-slim
  runtime: docker
  env:
    APP_ENV: production
    LOG_LEVEL: info
  secrets:
    - name: DATABASE_URL
      source: env:DATABASE_URL        # from host env var
    - name: API_KEY
      source: file:/run/secrets/key   # from file
    - name: DEBUG
      source: "false"                 # literal value
  setup:
    - run: pip install flask
  workspace:
    mountPath: /workspace
  network:
    policy: bridge
    expose: [5000]
```

### Blueprint inheritance

```yaml
# base-python.yaml -- shared base
apiVersion: smx/v1alpha1
kind: Blueprint
metadata:
  name: base-python
  version: "1.0"
spec:
  base: python:3.12-slim
  runtime: docker
  resources:
    cpu: "1"
    memory: 512Mi
  env:
    PYTHONDONTWRITEBYTECODE: "1"
  setup:
    - run: pip install --upgrade pip
  workspace:
    mountPath: /workspace
  network:
    policy: bridge
```

```yaml
# data-science.yaml -- inherits from base, adds packages and memory
apiVersion: smx/v1alpha1
kind: Blueprint
metadata:
  name: data-science
  version: "1.0"
spec:
  extends: base-python.yaml
  resources:
    memory: 2Gi
  setup:
    - run: pip install pandas numpy matplotlib scikit-learn jupyter
  env:
    JUPYTER_PORT: "8888"
  network:
    policy: bridge
    expose: [8888]
```

Inheritance rules:
- **Scalars** (base, runtime, workspace, readinessProbe): child wins if set
- **Resources**: child wins per-field (CPU, Memory, Disk, GPU independently)
- **Lists** (setup, toolchains, devices, secrets): child's entries appended to parent's
- **Maps** (env): merged, child wins on key conflicts
- **Max depth**: 5 levels to prevent circular references

### GPU-enabled blueprint

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

### Device passthrough with readiness probe

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

## SSO/OIDC Authentication

sandboxMatrix supports SSO via Google, GitHub, and any generic OIDC identity provider. Users are auto-created on first login with configurable role mapping from IdP groups.

```yaml
# ~/.sandboxmatrix/config.yaml
oidc:
  enabled: true
  provider: "google"  # or "github", "oidc"
  issuer: "https://accounts.google.com"
  clientId: "your-client-id"
  clientSecret: "your-secret"
  redirectUrl: "http://localhost:8080/api/v1/auth/oidc/callback"
  scopes: ["openid", "email", "profile"]
  roleMapping:
    "engineering": "operator"
    "platform-team": "admin"

jwt:
  signingKey: "your-secret-key"
  issuer: "sandboxmatrix"
  accessTokenTtl: "1h"
  refreshTokenTtl: "168h"
```

## Team Namespaces + Resource Quotas

Organize users into teams with per-team resource limits. Quota enforcement is applied on sandbox/matrix creation.

### CLI

```bash
smx team create ml-team --quota-sandboxes=20 --quota-cpu=32 --quota-memory=64G
smx team add-member ml-team alice --role=operator
smx team inspect ml-team  # shows team info + current usage
```

### API

```bash
# Create team
curl -X POST http://localhost:8080/api/v1/teams \
  -d '{"name":"ml-team","quota":{"maxSandboxes":20,"maxCpu":"32","maxMemory":"64G"}}'

# Create sandbox with team
curl -X POST http://localhost:8080/api/v1/sandboxes \
  -d '{"name":"my-sandbox","blueprint":"base-python.yaml","team":"ml-team"}'
```

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

### OpenTelemetry Distributed Tracing

sandboxMatrix supports full distributed tracing via OpenTelemetry with OTLP export (HTTP and gRPC). Spans are emitted for controller operations, Docker runtime calls, and HTTP handlers.

```yaml
# ~/.sandboxmatrix/config.yaml
tracing:
  enabled: true
  endpoint: "localhost:4318"
  protocol: "http"  # or "grpc"
  serviceName: "sandboxmatrix"
  sampleRate: 1.0
```

### Grafana Dashboards

Three ready-to-import dashboards are included in `deploy/grafana/dashboards/`:

| Dashboard | Description |
|---|---|
| **Overview** | Sandbox counts, create/destroy rates, active sessions, error rates |
| **Performance** | Operation latencies (p50/p95/p99), exec durations, API response times |
| **Resources** | CPU/memory usage per sandbox, pool utilization, quota consumption |

```bash
# Copy dashboards to Grafana provisioning path
cp -r deploy/grafana/dashboards/ /var/lib/grafana/dashboards/sandboxmatrix/
cp deploy/grafana/provisioning/dashboards.yaml /etc/grafana/provisioning/dashboards/sandboxmatrix.yaml
```

Together, Prometheus metrics + OpenTelemetry traces + Grafana dashboards provide a complete observability stack.

### Structured Logging

All components use Go's `log/slog` with JSON output for structured logging.

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
| [API Reference](docs/api-reference.md) | All 40+ REST API endpoints with examples |
| [Deployment Guide](docs/deployment.md) | Local, Docker, K8s deployment + monitoring |
| [SDK Guide](docs/sdk-guide.md) | Go, Python & TypeScript SDK usage |

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup and guidelines.

## License

Apache License 2.0 -- see [LICENSE](LICENSE) for details.
