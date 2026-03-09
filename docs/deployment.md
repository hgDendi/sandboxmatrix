# Deployment Guide

## Local Development

### Prerequisites

- Go 1.25+
- Docker Engine (for the default runtime)
- Make (optional, for build commands)
- golangci-lint (optional, for linting)

### Build from Source

```bash
git clone https://github.com/hgDendi/sandboxmatrix.git
cd sandboxmatrix
make build
```

The binary is written to `bin/smx`. Verify:

```bash
./bin/smx version
```

To install to `$GOPATH/bin`:

```bash
make install
```

### Run Locally

```bash
# Create a sandbox
smx sandbox create -b blueprints/python-dev.yaml -n dev

# Execute a command
smx sandbox exec dev -- python -c "print('hello')"

# Start the REST API server
smx server start --addr :8080

# Start the web dashboard
smx dashboard --addr :9090

# Start the MCP server (for AI agents)
smx mcp serve
```

## Docker

You can run the sandboxMatrix operator itself in a Docker container. The container needs access to the Docker socket to manage sandbox containers:

```bash
docker run -d \
  --name sandboxmatrix \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -p 8080:8080 \
  -p 9090:9090 \
  ghcr.io/hgdendi/sandboxmatrix:latest \
  smx server start --addr :8080
```

## Kubernetes

sandboxMatrix can run as a Kubernetes operator that manages Sandbox, Matrix, and Blueprint custom resources.

### Install CRDs

```bash
kubectl apply -f deploy/crds/
```

This installs three CRDs:
- `sandboxes.smx.sandboxmatrix.dev`
- `matrices.smx.sandboxmatrix.dev`
- `blueprints.smx.sandboxmatrix.dev`

### Deploy with Kustomize

```bash
kubectl apply -k deploy/
```

This creates:
- `sandboxmatrix` namespace
- ServiceAccount with RBAC permissions
- Operator deployment

### Deploy with Helm

```bash
helm install sandboxmatrix deploy/helm/sandboxmatrix/
```

Helm values (`deploy/helm/sandboxmatrix/values.yaml`):

```yaml
replicaCount: 1

image:
  repository: ghcr.io/hgdendi/sandboxmatrix
  tag: latest
  pullPolicy: IfNotPresent

namespace: sandboxmatrix

serviceAccount:
  create: true
  name: sandboxmatrix-operator

operator:
  args: []
  watchNamespace: "sandboxmatrix"

resources:
  requests:
    cpu: 100m
    memory: 128Mi
  limits:
    cpu: 500m
    memory: 256Mi

installCRDs: true
```

Override values:

```bash
helm install sandboxmatrix deploy/helm/sandboxmatrix/ \
  --set image.tag=v0.2.0 \
  --set operator.watchNamespace=""  # watch all namespaces
```

### Declarative Sandbox Management

Once the operator is running, create sandboxes via kubectl:

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

```bash
kubectl apply -f sandbox.yaml
kubectl get sandboxes
```

## Configuration

sandboxMatrix reads configuration from `~/.sandboxmatrix/config.yaml`. If the file does not exist, defaults are used.

### Full Configuration Reference

```yaml
# Default runtime backend: docker, gvisor
defaultRuntime: docker

# Directory for blueprint files
blueprintDir: ""

# Directory for state files (JSON state backend)
stateDir: ""

# Log level: debug, info, warn, error
logLevel: info

# REST API server settings
server:
  addr: ":8080"

# Web dashboard settings
dashboard:
  addr: ":9090"

# Pre-warmed pool settings
pool:
  minReady: 2
  maxSize: 5
```

The configuration directory is created automatically at `~/.sandboxmatrix/`.

## State Backends

sandboxMatrix supports three state backends for persisting sandbox, session, and matrix records.

### File (Default)

JSON files stored in `~/.sandboxmatrix/state/`. No additional setup required.

```bash
# Uses file backend by default
smx sandbox create -b blueprints/python-dev.yaml -n dev
```

### BoltDB

Embedded key-value store. Single database file, no server needed. Better performance than file backend for large numbers of sandboxes.

BoltDB supports sandbox and session storage but falls back to file-based storage for matrices (due to method name conflicts in the Go interface).

### etcd

For distributed deployments where multiple sandboxMatrix instances share state.

```bash
# Start etcd (if not already running)
etcd --listen-client-urls http://0.0.0.0:2379 \
     --advertise-client-urls http://localhost:2379

# Configure sandboxMatrix to use etcd
# Set via StoreConfig in code or environment variables
```

The etcd backend implements all three store interfaces (Store, SessionStore, MatrixStore) through a single `EtcdStore` type. Default endpoint: `localhost:2379`.

## Monitoring

### Prometheus Metrics

The REST API server exposes Prometheus metrics at `GET /metrics`. Scrape configuration for Prometheus:

```yaml
scrape_configs:
  - job_name: 'sandboxmatrix'
    static_configs:
      - targets: ['localhost:8080']
    metrics_path: '/metrics'
```

### Available Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `smx_sandboxes_active` | Gauge | Number of active sandboxes |
| `smx_sandbox_operations_total` | Counter | Operations by type and result |
| `smx_sandbox_operation_duration_seconds` | Histogram | Operation duration |
| `smx_exec_total` | Counter | Exec commands by sandbox and result |
| `smx_exec_duration_seconds` | Histogram | Exec command duration |
| `smx_sessions_active` | Gauge | Active session count |
| `smx_matrices_active` | Gauge | Active matrix count |
| `smx_pool_size` | Gauge | Pre-warmed instances per blueprint |
| `smx_http_requests_total` | Counter | HTTP requests by method, path, status |
| `smx_http_request_duration_seconds` | Histogram | HTTP request duration |
| `smx_websocket_connections` | Gauge | Active WebSocket connections |

### Grafana Dashboard Suggestions

Create dashboards for:

1. **Sandbox Overview** -- `smx_sandboxes_active`, `smx_sandbox_operations_total` by operation, operation duration percentiles
2. **API Performance** -- `smx_http_requests_total` by status, request duration percentiles, error rate
3. **Exec Monitoring** -- `smx_exec_total` by sandbox and result, exec duration percentiles
4. **Pool Health** -- `smx_pool_size` by blueprint, pool claim rates
5. **WebSocket** -- `smx_websocket_connections` over time

### Structured Logging

All components use Go's `log/slog` with JSON output. Log fields include:
- `msg` -- Log message
- `name` -- Sandbox/matrix/session name
- `duration` -- Operation duration
- `method`, `path`, `status` -- HTTP request details

## Security

### RBAC Setup

Enable RBAC by creating users with roles:

```bash
# Add an admin user (generates a token)
smx auth add-user alice --role admin
# Output: Token: a1b2c3d4e5f6...

# Add an operator
smx auth add-user bob --role operator

# Add a read-only viewer
smx auth add-user carol --role viewer

# List users
smx auth list-users

# Remove a user
smx auth remove-user bob
```

### Roles and Permissions

| Role | Sandboxes | Matrices | Sessions | Pools | Blueprints |
|------|-----------|----------|----------|-------|------------|
| `admin` | Full | Full | Full | Full | Full |
| `operator` | CRUD + exec | CRUD | CRUD + exec | CRUD | Read |
| `viewer` | Read | Read | Read | Read | Read |

### Using Tokens

Pass the token in API requests:

```bash
curl -H "Authorization: Bearer a1b2c3d4e5f6..." \
  http://localhost:8080/api/v1/sandboxes
```

In Python SDK:

```python
from sandboxmatrix.http_client import HTTPClient
client = HTTPClient(token="a1b2c3d4e5f6...")
```

In TypeScript SDK:

```typescript
import { HTTPClient } from "@sandboxmatrix/sdk";
const client = new HTTPClient({ token: "a1b2c3d4e5f6..." });
```

### Audit Logging

Every authenticated API action is recorded in the audit log:

```bash
smx auth audit
```

Each entry includes:
- Timestamp
- User name
- Action (e.g., `sandbox.create`, `matrix.delete`)
- Resource (e.g., `sandbox/my-sandbox`)
- Result (`success`, `denied`, `error`)
- Detail (error message, if applicable)

### Container Security Hardening

The Docker runtime applies security defaults to all sandbox containers:
- `no-new-privileges` flag enabled
- Capabilities dropped (ALL), then selectively added: `NET_BIND_SERVICE`, `CHOWN`, `SETUID`, `SETGID`, `DAC_OVERRIDE`
- Network policies enforced per blueprint configuration
- Workspace mounts can be configured as read-only
