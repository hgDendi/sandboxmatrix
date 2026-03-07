# @sandboxmatrix/sdk

TypeScript SDK for sandboxMatrix - an AI sandbox orchestrator.

This SDK wraps the `smx` CLI binary and provides a typed interface for managing sandboxes, matrices, snapshots, and sessions.

## Prerequisites

- Node.js >= 18
- The `smx` CLI binary must be installed and available in your `PATH` (or specify the path explicitly)

## Installation

```bash
npm install @sandboxmatrix/sdk
```

## Quick Start

```typescript
import { SandboxMatrixClient } from "@sandboxmatrix/sdk";

const client = new SandboxMatrixClient();

// Create a sandbox
const sandbox = client.createSandbox("my-sandbox", "ubuntu:22.04");
console.log(`Sandbox state: ${sandbox.state}`);

// Execute a command
const result = client.exec("my-sandbox", "echo hello world");
console.log(result.stdout); // "hello world\n"

// Clean up
client.destroySandbox("my-sandbox");
```

## Configuration

You can specify a custom path to the `smx` binary:

```typescript
const client = new SandboxMatrixClient({
  binary: "/opt/bin/smx",
});
```

## API Reference

### Sandbox Operations

| Method | Description |
|--------|-------------|
| `createSandbox(name, blueprint, workspace?)` | Create a new sandbox from a blueprint |
| `getSandbox(name)` | Get details about a sandbox |
| `listSandboxes()` | List all sandboxes |
| `exec(name, command)` | Execute a command in a sandbox |
| `startSandbox(name)` | Start a stopped sandbox |
| `stopSandbox(name)` | Stop a running sandbox |
| `destroySandbox(name)` | Destroy a sandbox |

### Snapshot Operations

| Method | Description |
|--------|-------------|
| `snapshot(name, tag?)` | Create a snapshot of a sandbox |
| `restore(name, snapshotId, newName)` | Restore a sandbox from a snapshot |

### Matrix Operations

| Method | Description |
|--------|-------------|
| `createMatrix(name, members)` | Create a matrix with named members |
| `listMatrices()` | List all matrices |
| `destroyMatrix(name)` | Destroy a matrix |

### Session Operations

| Method | Description |
|--------|-------------|
| `startSession(sandboxName)` | Start a session on a sandbox |
| `endSession(sessionId)` | End a session |

### Other

| Method | Description |
|--------|-------------|
| `version()` | Get version information |

## Error Handling

The SDK provides typed errors for common failure cases:

```typescript
import { SandboxMatrixClient, CLIError, SandboxNotFoundError } from "@sandboxmatrix/sdk";

const client = new SandboxMatrixClient();

try {
  const sandbox = client.getSandbox("nonexistent");
} catch (error) {
  if (error instanceof CLIError) {
    console.error(`CLI failed with exit code ${error.exitCode}: ${error.stderr}`);
  }
}
```

## License

Apache-2.0
