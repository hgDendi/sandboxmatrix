# sandboxMatrix Python SDK

Python SDK for [sandboxMatrix](https://github.com/hgDendi/sandboxmatrix) -- an AI sandbox orchestrator.

Wraps the `smx` CLI binary so that Python programs and AI agents can manage sandboxes programmatically.

## Installation

```bash
pip install sandboxmatrix
```

Or install from source:

```bash
cd sdk/python
pip install -e .
```

## Prerequisites

The `smx` CLI binary must be installed and available on your `PATH`, or you can
pass its location explicitly when creating the client.

## Quick Start

```python
from sandboxmatrix import SandboxMatrixClient

# Auto-detect smx binary on PATH
client = SandboxMatrixClient()

# Or point to a specific binary
client = SandboxMatrixClient(binary="/usr/local/bin/smx")

# Create a sandbox
sandbox = client.create_sandbox(name="dev", blueprint="ubuntu:22.04")
print(sandbox.state)  # "Running"

# Execute a command
result = client.exec("dev", "echo hello world")
print(result.stdout)   # "hello world\n"
print(result.exit_code) # 0

# List sandboxes
for sb in client.list_sandboxes():
    print(f"{sb.name}: {sb.state}")

# Snapshot and restore
snap_id = client.snapshot("dev", tag="checkpoint-1")
restored = client.restore("dev", snap_id, new_name="dev-copy")

# Matrix (multi-sandbox group)
client.create_matrix("my-cluster", members={
    "web": "nginx:latest",
    "api": "python:3.11",
    "db": "postgres:15",
})

# Sessions
session_id = client.start_session("dev")
client.end_session(session_id)

# Cleanup
client.stop_sandbox("dev")
client.destroy_sandbox("dev")
```

## HTTP Client (REST API)

For direct API access without the CLI binary:

```python
from sandboxmatrix import HTTPClient

client = HTTPClient(base_url="http://localhost:8080", token="your-token")

# List sandboxes
sandboxes = client.list_sandboxes()

# Execute command
result = client.exec("my-sandbox", "python -c 'print(42)'")
print(result.stdout)
```

## Error Handling

```python
from sandboxmatrix import SandboxMatrixClient, CLIError, SandboxMatrixError

client = SandboxMatrixClient()

try:
    client.get_sandbox("nonexistent")
except CLIError as e:
    print(f"CLI failed (exit {e.exit_code}): {e}")
    print(f"stderr: {e.stderr}")
```

## API Reference

### `SandboxMatrixClient`

| Method | Description |
|--------|-------------|
| `version()` | Get smx version info as a dict |
| `create_sandbox(name, blueprint, workspace=None)` | Create a new sandbox |
| `get_sandbox(name)` | Inspect a sandbox |
| `list_sandboxes()` | List all sandboxes |
| `exec(name, command)` | Run a command in a sandbox |
| `start_sandbox(name)` | Start a stopped sandbox |
| `stop_sandbox(name)` | Stop a running sandbox |
| `destroy_sandbox(name)` | Permanently destroy a sandbox |
| `snapshot(name, tag=None)` | Snapshot a sandbox |
| `restore(name, snapshot_id, new_name)` | Restore from snapshot |
| `create_matrix(name, members)` | Create a sandbox group |
| `list_matrices()` | List all matrices |
| `destroy_matrix(name)` | Destroy a matrix |
| `start_session(sandbox_name)` | Start a session |
| `end_session(session_id)` | End a session |

### Data Models

- **`Sandbox`** -- `name`, `state`, `blueprint`, `runtime_id`, `ip`, `created_at`
- **`ExecResult`** -- `exit_code`, `stdout`, `stderr`
- **`Snapshot`** -- `id`, `tag`, `created_at`, `size`
- **`Matrix`** -- `name`, `state`, `members`
- **`Session`** -- `id`, `sandbox`, `state`, `exec_count`

### Exceptions

- **`SandboxMatrixError`** -- Base exception
- **`SandboxNotFoundError`** -- Sandbox does not exist
- **`SandboxNotRunningError`** -- Sandbox is not running
- **`BlueprintError`** -- Invalid blueprint
- **`CLIError`** -- CLI execution failure (includes `exit_code` and `stderr`)

## License

Apache-2.0
