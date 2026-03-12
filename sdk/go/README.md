# sandboxmatrix Go SDK

Go client for the sandboxMatrix REST API. Zero external dependencies -- uses only the Go standard library.

## Install

```bash
go get github.com/hg-dendi/sandboxmatrix/sdk/go
```

## Quick Start

```go
package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	smx "github.com/hg-dendi/sandboxmatrix/sdk/go"
)

func main() {
	ctx := context.Background()
	client := smx.NewClient("http://localhost:8080", smx.WithToken("my-token"))

	// Create a sandbox
	sb, err := client.CreateSandbox(ctx, "dev", "blueprints/python.yaml", "")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Created sandbox: %s (state: %s)\n", sb.Name, sb.State)

	// Execute a command
	result, err := client.Exec(ctx, "dev", "echo hello world")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Output: %s\n", result.Stdout)

	// Write a file
	err = client.WriteFile(ctx, "dev", "/tmp/hello.txt", strings.NewReader("Hello!"))
	if err != nil {
		log.Fatal(err)
	}

	// Read it back
	data, err := client.ReadFile(ctx, "dev", "/tmp/hello.txt")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("File content: %s\n", string(data))

	// List files
	files, err := client.ListFiles(ctx, "dev", "/tmp")
	if err != nil {
		log.Fatal(err)
	}
	for _, f := range files {
		fmt.Printf("  %s (%d bytes)\n", f.Name, f.Size)
	}

	// Clean up
	_ = client.DestroySandbox(ctx, "dev")
}
```

## Error Handling

```go
result, err := client.GetSandbox(ctx, "nonexistent")
if err != nil {
	var notFound *smx.NotFoundError
	if errors.As(err, &notFound) {
		fmt.Println("Sandbox not found")
	}
}
```

## Matrix Operations

```go
members := []smx.MatrixMember{
	{Name: "worker-1", Blueprint: "blueprints/python.yaml"},
	{Name: "worker-2", Blueprint: "blueprints/python.yaml"},
}
mx, err := client.CreateMatrix(ctx, "my-cluster", members)
```

## API Reference

### Client Methods

| Method | Description |
|--------|-------------|
| `Health(ctx)` | Check server health |
| `Version(ctx)` | Get server version info |
| `CreateSandbox(ctx, name, blueprint, workspace)` | Create a sandbox |
| `GetSandbox(ctx, name)` | Get sandbox details |
| `ListSandboxes(ctx)` | List all sandboxes |
| `StartSandbox(ctx, name)` | Start a stopped sandbox |
| `StopSandbox(ctx, name)` | Stop a running sandbox |
| `DestroySandbox(ctx, name)` | Destroy a sandbox |
| `Exec(ctx, name, command)` | Execute a shell command |
| `ExecRaw(ctx, name, argv)` | Execute with explicit argv |
| `Stats(ctx, name)` | Get resource usage stats |
| `CreateSnapshot(ctx, name, tag)` | Create a snapshot |
| `ListSnapshots(ctx, name)` | List snapshots |
| `CreateMatrix(ctx, name, members)` | Create a matrix |
| `GetMatrix(ctx, name)` | Get matrix details |
| `ListMatrices(ctx)` | List all matrices |
| `StartMatrix(ctx, name)` | Start a matrix |
| `StopMatrix(ctx, name)` | Stop a matrix |
| `DestroyMatrix(ctx, name)` | Destroy a matrix |
| `StartSession(ctx, sandbox)` | Start a session |
| `ListSessions(ctx, sandbox)` | List sessions |
| `EndSession(ctx, id)` | End a session |
| `WriteFile(ctx, sandbox, path, reader)` | Write a file to sandbox |
| `ReadFile(ctx, sandbox, path)` | Read a file from sandbox |
| `ListFiles(ctx, sandbox, path)` | List files in sandbox directory |
