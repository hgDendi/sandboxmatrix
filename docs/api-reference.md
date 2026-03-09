# REST API Reference

Base URL: `http://localhost:8080` (configurable via `--addr` flag)

All request and response bodies use `Content-Type: application/json`.

When RBAC is enabled, all endpoints except health and version require a `Authorization: Bearer <token>` header.

---

## Health / Version / Metrics

### GET /api/v1/health

Health check endpoint. Always returns 200 when the server is running.

**Response (200):**
```json
{"status": "ok"}
```

### GET /api/v1/version

Returns server version information.

**Response (200):**
```json
{
  "version": "0.1.0",
  "commit": "26228ed",
  "buildDate": "2025-01-15T10:00:00Z",
  "goVersion": "go1.25.0",
  "os": "linux",
  "arch": "amd64"
}
```

### GET /metrics

Prometheus metrics endpoint. Returns metrics in Prometheus text exposition format.

**Response (200):** Prometheus text format (not JSON).

---

## Sandboxes

### POST /api/v1/sandboxes

Create a new sandbox from a blueprint. The sandbox is created and started immediately.

**Request:**
```json
{
  "name": "my-sandbox",
  "blueprint": "blueprints/python-dev.yaml",
  "workspace": "/path/to/project"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Unique sandbox name |
| `blueprint` | string | Yes | Path to blueprint YAML file |
| `workspace` | string | No | Host directory to mount as workspace |

**Response (201):**
```json
{
  "apiVersion": "smx/v1alpha1",
  "kind": "Sandbox",
  "metadata": {
    "name": "my-sandbox",
    "createdAt": "2025-01-15T10:00:00Z",
    "updatedAt": "2025-01-15T10:00:01Z",
    "labels": {"blueprint": "python-dev"}
  },
  "spec": {
    "blueprintRef": "python-dev",
    "resources": {"cpu": "2", "memory": "2Gi"}
  },
  "status": {
    "state": "Running",
    "runtimeID": "abc123def456",
    "ip": "172.17.0.2",
    "startedAt": "2025-01-15T10:00:01Z"
  }
}
```

**Errors:**
- `400` -- Missing name or blueprint
- `500` -- Blueprint validation failed, duplicate name, or runtime error

### GET /api/v1/sandboxes

List all sandboxes.

**Response (200):**
```json
[
  {
    "apiVersion": "smx/v1alpha1",
    "kind": "Sandbox",
    "metadata": {"name": "my-sandbox", "createdAt": "..."},
    "spec": {"blueprintRef": "python-dev"},
    "status": {"state": "Running", "runtimeID": "abc123", "ip": "172.17.0.2"}
  }
]
```

Returns an empty array `[]` if no sandboxes exist.

### GET /api/v1/sandboxes/{name}

Get a specific sandbox by name.

**Response (200):** Full Sandbox object (same schema as create response).

**Errors:**
- `400` -- Missing name
- `404` -- Sandbox not found

### POST /api/v1/sandboxes/{name}/start

Start a stopped sandbox.

**Response (200):**
```json
{"status": "started", "name": "my-sandbox"}
```

**Errors:**
- `400` -- Missing name
- `500` -- Sandbox not in stopped state, or runtime error

### POST /api/v1/sandboxes/{name}/stop

Stop a running sandbox.

**Response (200):**
```json
{"status": "stopped", "name": "my-sandbox"}
```

**Errors:**
- `400` -- Missing name
- `500` -- Sandbox not running, or runtime error

### DELETE /api/v1/sandboxes/{name}

Destroy a sandbox and clean up its resources.

**Response (200):**
```json
{"status": "destroyed", "name": "my-sandbox"}
```

**Errors:**
- `400` -- Missing name
- `500` -- Sandbox not found, or runtime error

### POST /api/v1/sandboxes/{name}/exec

Execute a command inside a running sandbox.

**Request:**
```json
{
  "command": ["python", "-c", "print('hello')"]
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `command` | string[] | Yes | Command and arguments to execute |

**Response (200):**
```json
{
  "exitCode": 0,
  "stdout": "hello\n",
  "stderr": ""
}
```

**Errors:**
- `400` -- Missing name or command
- `500` -- Sandbox not running, or exec failed

**Limits:** Exec output is buffered up to 10 MB per stream (stdout/stderr). Request body max 1 MB.

### GET /api/v1/sandboxes/{name}/stats

Get CPU and memory usage statistics for a running sandbox.

**Response (200):**
```json
{
  "cpuUsage": 25.3,
  "memoryUsage": 134217728,
  "memoryLimit": 2147483648,
  "diskUsage": 0
}
```

| Field | Type | Description |
|-------|------|-------------|
| `cpuUsage` | float64 | CPU usage percentage |
| `memoryUsage` | uint64 | Memory usage in bytes |
| `memoryLimit` | uint64 | Memory limit in bytes |
| `diskUsage` | uint64 | Disk usage in bytes |

**Errors:**
- `400` -- Missing name
- `500` -- Sandbox not running

### POST /api/v1/sandboxes/{name}/snapshots

Create a point-in-time snapshot of a sandbox.

**Request (optional body):**
```json
{
  "tag": "v1"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `tag` | string | No | Tag for the snapshot |

**Response (201):**
```json
{
  "snapshotId": "sha256:abc123...",
  "tag": "v1"
}
```

### GET /api/v1/sandboxes/{name}/snapshots

List all snapshots for a sandbox.

**Response (200):**
```json
[
  {
    "id": "sha256:abc123...",
    "tag": "v1",
    "sandboxId": "container-id",
    "createdAt": "2025-01-15T10:05:00Z",
    "size": 52428800
  }
]
```

### GET /api/v1/sandboxes/{name}/exec/stream (WebSocket)

Real-time exec streaming via WebSocket.

**Protocol:**

1. Client connects via WebSocket upgrade
2. Client sends command:
   ```json
   {"command": ["sh", "-c", "your-command"]}
   ```
3. Server streams output events:
   ```json
   {"type": "stdout", "data": "output line\n"}
   {"type": "stderr", "data": "error line\n"}
   ```
4. Client can send stdin:
   ```json
   {"type": "stdin", "data": "input text"}
   ```
5. On completion:
   ```json
   {"type": "exit", "exitCode": 0}
   ```
6. On error:
   ```json
   {"type": "error", "data": "error message"}
   ```

---

## Matrices

### POST /api/v1/matrices

Create a new matrix (group of coordinated sandboxes). Each member sandbox is created on an isolated Docker network.

**Request:**
```json
{
  "name": "fullstack",
  "members": [
    {"name": "frontend", "blueprint": "blueprints/node-dev.yaml"},
    {"name": "backend", "blueprint": "blueprints/python-dev.yaml"}
  ]
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Unique matrix name |
| `members` | array | Yes | List of member definitions |
| `members[].name` | string | Yes | Member name (sandbox will be `{matrix}-{member}`) |
| `members[].blueprint` | string | Yes | Path to blueprint YAML |

**Response (201):**
```json
{
  "apiVersion": "smx/v1alpha1",
  "kind": "Matrix",
  "metadata": {"name": "fullstack", "createdAt": "..."},
  "members": [
    {"name": "frontend", "blueprint": "blueprints/node-dev.yaml"},
    {"name": "backend", "blueprint": "blueprints/python-dev.yaml"}
  ],
  "state": "Active"
}
```

### GET /api/v1/matrices

List all matrices.

**Response (200):** Array of Matrix objects. Returns `[]` if none exist.

### GET /api/v1/matrices/{name}

Get a specific matrix by name.

**Response (200):** Full Matrix object.

**Errors:** `404` -- Matrix not found.

### POST /api/v1/matrices/{name}/start

Start all sandboxes in a stopped matrix.

**Response (200):**
```json
{"status": "started", "name": "fullstack"}
```

### POST /api/v1/matrices/{name}/stop

Stop all sandboxes in a matrix.

**Response (200):**
```json
{"status": "stopped", "name": "fullstack"}
```

### DELETE /api/v1/matrices/{name}

Destroy a matrix, all its member sandboxes, and the isolated network.

**Response (200):**
```json
{"status": "destroyed", "name": "fullstack"}
```

### POST /api/v1/matrices/{name}/shard

Distribute tasks across matrix members using a sharding strategy.

**Request:**
```json
{
  "tasks": [
    {"id": "t1", "payload": "process file1.txt"},
    {"id": "t2", "payload": "process file2.txt", "key": "group-a"}
  ],
  "strategy": "round-robin"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `tasks` | array | Yes | Tasks to distribute |
| `tasks[].id` | string | Yes | Unique task ID |
| `tasks[].payload` | string | Yes | Task payload |
| `tasks[].key` | string | No | Key for hash-based sharding |
| `strategy` | string | No | `round-robin` (default), `hash`, or `balanced` |

**Response (200):**
```json
{
  "totalTasks": 2,
  "assignments": [
    {"memberName": "frontend", "task": {"id": "t1", "payload": "process file1.txt"}},
    {"memberName": "backend", "task": {"id": "t2", "payload": "process file2.txt", "key": "group-a"}}
  ],
  "strategy": "round-robin"
}
```

Tasks are also delivered to member sandboxes via A2A messages (type: `task-assignment`) if the gateway is configured.

### POST /api/v1/matrices/{name}/collect

Collect results from distributed tasks.

**Request:**
```json
{
  "taskID": "t1",
  "strategy": "collect-all",
  "timeoutSec": 60
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `taskID` | string | Yes | Task ID to collect results for |
| `strategy` | string | No | `collect-all` (default), `first-success`, `majority` |
| `timeoutSec` | int | No | Timeout in seconds (default: 60) |

**Response (200):**
```json
{
  "taskID": "t1",
  "strategy": "collect-all",
  "results": [
    {"memberName": "frontend", "taskID": "t1", "status": "success", "output": "done"}
  ],
  "total": 1,
  "succeeded": 1,
  "failed": 0
}
```

---

## Sessions

### POST /api/v1/sessions

Start a new session on a running sandbox.

**Request:**
```json
{
  "sandbox": "my-sandbox"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `sandbox` | string | Yes | Name of the sandbox |

**Response (201):**
```json
{
  "apiVersion": "smx/v1alpha1",
  "kind": "Session",
  "metadata": {"name": "my-sandbox-1705312800000000000", "createdAt": "..."},
  "sandbox": "my-sandbox",
  "state": "Active",
  "startedAt": "2025-01-15T10:00:00Z",
  "execCount": 0
}
```

### GET /api/v1/sessions

List all sessions. Optionally filter by sandbox.

**Query parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `sandbox` | string | Filter sessions by sandbox name |

**Response (200):** Array of Session objects.

### POST /api/v1/sessions/{id}/end

End an active session.

**Response (200):**
```json
{"status": "ended", "id": "my-sandbox-1705312800000000000"}
```

### POST /api/v1/sessions/{id}/exec

Execute a command within a session. Increments the session's exec counter.

**Request:**
```json
{
  "command": ["python", "-c", "print('hello')"]
}
```

**Response (200):**
```json
{
  "exitCode": 0,
  "stdout": "hello\n",
  "stderr": ""
}
```

---

## Error Format

All error responses use the following format:

```json
{
  "error": "description of what went wrong"
}
```

Common HTTP status codes:
- `400` -- Bad request (missing required fields, invalid JSON)
- `401` -- Unauthorized (missing or invalid token, when RBAC is enabled)
- `403` -- Forbidden (valid token but insufficient permissions)
- `404` -- Resource not found
- `500` -- Internal server error
- `503` -- Service unavailable (e.g., A2A gateway not configured)

## Authentication

When RBAC is enabled (via `smx auth add-user`), include the token in every request:

```
Authorization: Bearer <token>
```

Endpoints exempt from authentication: `GET /api/v1/health`, `GET /api/v1/version`, and CORS preflight (`OPTIONS`).

## Rate Limits and Constraints

- Maximum request body size: 1 MB
- Maximum buffered exec output: 10 MB per stream
- Server timeouts: read 30s, write 60s, idle 120s, header read 10s
- Maximum header size: 1 MB
