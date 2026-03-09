# SDK Usage Guide

sandboxMatrix provides Python and TypeScript SDKs, each offering two client modes:

- **CLI Client** -- Wraps the `smx` binary (requires smx installed locally)
- **HTTP Client** -- Talks directly to the REST API server (requires `smx server start`)

## Python SDK

### Installation

```bash
cd sdk/python
pip install -e .
```

Requires Python 3.10+.

### CLI Client

The CLI client wraps the `smx` binary. It requires the binary to be in PATH or specified explicitly.

```python
from sandboxmatrix import SandboxMatrixClient

# Auto-detect smx in PATH
client = SandboxMatrixClient()

# Or specify the binary path
client = SandboxMatrixClient(binary="/path/to/smx")
```

#### Create and Use a Sandbox

```python
# Create a sandbox from a blueprint
sandbox = client.create_sandbox(
    name="my-sandbox",
    blueprint="blueprints/python-dev.yaml",
    workspace="/path/to/project"
)
print(f"Created: {sandbox.name} (state: {sandbox.state})")

# Execute a command
result = client.exec("my-sandbox", "python -c 'print(1 + 1)'")
print(f"Exit code: {result.exit_code}")
print(f"Output: {result.stdout}")

# List sandboxes
sandboxes = client.list_sandboxes()
for sb in sandboxes:
    print(f"  {sb.name}: {sb.state}")

# Stop and start
client.stop_sandbox("my-sandbox")
client.start_sandbox("my-sandbox")

# Destroy
client.destroy_sandbox("my-sandbox")
```

#### Snapshots

```python
# Create a snapshot
snapshot_id = client.snapshot("my-sandbox", tag="v1")
print(f"Snapshot: {snapshot_id}")

# Restore from snapshot
restored = client.restore("my-sandbox", snapshot_id, new_name="restored-sandbox")
print(f"Restored: {restored.name}")
```

#### Matrix Orchestration

```python
# Create a matrix with multiple sandboxes
client.create_matrix("fullstack", {
    "frontend": "blueprints/node-dev.yaml",
    "backend": "blueprints/python-dev.yaml",
})

# List matrices
matrices = client.list_matrices()
for mx in matrices:
    print(f"  {mx.name}: {mx.state}")

# Destroy matrix (destroys all member sandboxes)
client.destroy_matrix("fullstack")
```

#### Session Management

```python
# Start a session on a sandbox
session_id = client.start_session("my-sandbox")
print(f"Session: {session_id}")

# End the session
client.end_session(session_id)
```

### HTTP Client

The HTTP client connects directly to the REST API server. Start the server first:

```bash
smx server start --addr :8080
```

```python
from sandboxmatrix.http_client import HTTPClient

# Connect to local server
client = HTTPClient(base_url="http://localhost:8080")

# With RBAC token
client = HTTPClient(
    base_url="http://localhost:8080",
    token="your-bearer-token"
)
```

#### Create and Use a Sandbox

```python
# Create
sandbox = client.create_sandbox(
    name="my-sandbox",
    blueprint="blueprints/python-dev.yaml",
    workspace="/path/to/project"
)

# Execute
result = client.exec("my-sandbox", "echo hello")
print(result.stdout)  # "hello\n"

# Get sandbox details
sb = client.get_sandbox("my-sandbox")
print(f"{sb.name}: {sb.state} ({sb.ip})")

# List all
sandboxes = client.list_sandboxes()

# Stats
stats = client.stats("my-sandbox")
print(f"CPU: {stats['cpuUsage']}%")

# Lifecycle
client.stop_sandbox("my-sandbox")
client.start_sandbox("my-sandbox")
client.destroy_sandbox("my-sandbox")
```

#### Snapshots (HTTP)

```python
# Create snapshot
snap = client.create_snapshot("my-sandbox", tag="v1")
print(f"ID: {snap['snapshotId']}")

# List snapshots
snapshots = client.list_snapshots("my-sandbox")
```

#### Matrix Orchestration (HTTP)

```python
# Create matrix
mx = client.create_matrix("fullstack", members=[
    {"name": "frontend", "blueprint": "blueprints/node-dev.yaml"},
    {"name": "backend", "blueprint": "blueprints/python-dev.yaml"},
])

# Inspect
matrix = client.get_matrix("fullstack")

# Lifecycle
client.stop_matrix("fullstack")
client.start_matrix("fullstack")
client.destroy_matrix("fullstack")
```

#### Sessions (HTTP)

```python
# Start session
session = client.start_session("my-sandbox")
session_id = session["metadata"]["name"]

# List sessions (optionally filter by sandbox)
sessions = client.list_sessions(sandbox="my-sandbox")

# End session
client.end_session(session_id)
```

### Error Handling (Python)

```python
from sandboxmatrix.exceptions import (
    CLIError,
    SandboxNotFoundError,
    SandboxMatrixError,
)

try:
    client.get_sandbox("nonexistent")
except SandboxNotFoundError:
    print("Sandbox does not exist")
except CLIError as e:
    print(f"CLI error (exit {e.exit_code}): {e}")
except SandboxMatrixError as e:
    print(f"API error: {e}")
```

---

## TypeScript SDK

### Installation

```bash
cd sdk/typescript
npm install
npm run build
```

Requires Node.js 18+.

### CLI Client

```typescript
import { SandboxMatrixClient } from "@sandboxmatrix/sdk";

// Auto-detect smx in PATH
const client = new SandboxMatrixClient();

// Or specify the binary path
const client = new SandboxMatrixClient({ binary: "/path/to/smx" });
```

#### Create and Use a Sandbox

```typescript
// Create
const sandbox = client.createSandbox(
  "my-sandbox",
  "blueprints/python-dev.yaml",
  "/path/to/project" // optional workspace
);
console.log(`Created: ${sandbox.name} (${sandbox.state})`);

// Execute
const result = client.exec("my-sandbox", "echo hello");
console.log(`Exit: ${result.exitCode}, Output: ${result.stdout}`);

// Execute with array command
const result2 = client.exec("my-sandbox", ["python", "-c", "print(42)"]);

// List
const sandboxes = client.listSandboxes();
sandboxes.forEach(sb => console.log(`  ${sb.name}: ${sb.state}`));

// Lifecycle
client.stopSandbox("my-sandbox");
client.startSandbox("my-sandbox");
client.destroySandbox("my-sandbox");
```

#### Snapshots

```typescript
const snapshotId = client.snapshot("my-sandbox", "v1");
const restored = client.restore("my-sandbox", snapshotId, "restored-sandbox");
```

#### Matrix Orchestration

```typescript
client.createMatrix("fullstack", {
  frontend: "blueprints/node-dev.yaml",
  backend: "blueprints/python-dev.yaml",
});

const matrices = client.listMatrices();
client.destroyMatrix("fullstack");
```

#### Sessions

```typescript
const sessionId = client.startSession("my-sandbox");
client.endSession(sessionId);
```

### HTTP Client

```typescript
import { HTTPClient } from "@sandboxmatrix/sdk";

// Connect to local server
const client = new HTTPClient({
  baseURL: "http://localhost:8080",
  token: "your-bearer-token", // optional
});
```

All HTTP client methods are async:

```typescript
// Create
const sandbox = await client.createSandbox(
  "my-sandbox",
  "blueprints/python-dev.yaml"
);

// Execute
const result = await client.exec("my-sandbox", "echo hello");
console.log(result.stdout);

// Stats
const stats = await client.stats("my-sandbox");

// Snapshots
const snap = await client.createSnapshot("my-sandbox", "v1");
const snapshots = await client.listSnapshots("my-sandbox");

// Matrix
const matrix = await client.createMatrix("fullstack", [
  { name: "frontend", blueprint: "blueprints/node-dev.yaml" },
  { name: "backend", blueprint: "blueprints/python-dev.yaml" },
]);
await client.startMatrix("fullstack");
await client.stopMatrix("fullstack");
await client.destroyMatrix("fullstack");

// Sessions
const session = await client.startSession("my-sandbox");
const sessions = await client.listSessions("my-sandbox");
await client.endSession(session.metadata.name);

// Health
const health = await client.health();
const version = await client.version();
```

### Error Handling (TypeScript)

```typescript
import { CLIError, SandboxNotFoundError, SandboxMatrixError } from "@sandboxmatrix/sdk";

try {
  await client.getSandbox("nonexistent");
} catch (e) {
  if (e instanceof SandboxNotFoundError) {
    console.log("Not found");
  } else if (e instanceof SandboxMatrixError) {
    console.log(`API error: ${e.message}`);
  }
}
```

---

## Choosing Between CLI and HTTP Client

| Feature | CLI Client | HTTP Client |
|---------|-----------|-------------|
| Requires smx binary | Yes | No |
| Requires running server | No | Yes (`smx server start`) |
| RBAC support | No | Yes (Bearer token) |
| Network access | No (local only) | Yes (can connect remotely) |
| Performance | Process spawn per call | HTTP request per call |
| Best for | Local scripts, CI | Remote access, web apps |

## Complete Workflow Example

```python
from sandboxmatrix.http_client import HTTPClient

client = HTTPClient(base_url="http://localhost:8080")

# 1. Create a sandbox
sb = client.create_sandbox("worker", "blueprints/python-dev.yaml")

# 2. Install dependencies
client.exec("worker", "pip install requests pandas")

# 3. Run a script
result = client.exec("worker", "python /workspace/analysis.py")
print(result.stdout)

# 4. Snapshot the state
snap = client.create_snapshot("worker", tag="after-setup")

# 5. Clean up
client.destroy_sandbox("worker")
```
