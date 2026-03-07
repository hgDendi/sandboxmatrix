// Package runtime defines the pluggable runtime interface for sandbox isolation backends.
package runtime

import (
	"context"
	"io"
	"time"
)

// NetworkConfig holds network-related configuration for a sandbox.
type NetworkConfig struct {
	Mode string   // "none", "host", "bridge", or a custom network name
	DNS  []string // custom DNS servers
}

// CreateConfig holds configuration for creating a new sandbox runtime.
type CreateConfig struct {
	Name    string
	Image   string
	CPU     string
	Memory  string
	Disk    string
	Mounts  []Mount
	Ports   []PortMapping
	Env     map[string]string
	Cmd     []string
	Labels  map[string]string
	Network NetworkConfig
}

// Mount represents a filesystem mount.
type Mount struct {
	Source   string
	Target   string
	ReadOnly bool
}

// PortMapping represents a port forwarding rule.
type PortMapping struct {
	HostPort      int
	ContainerPort int
	Protocol      string
}

// ExecConfig holds configuration for executing a command in a sandbox.
type ExecConfig struct {
	Cmd    []string
	Env    map[string]string
	Dir    string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	TTY    bool
}

// ExecResult holds the result of a command execution.
type ExecResult struct {
	ExitCode int
}

// Stats holds resource usage statistics for a sandbox.
type Stats struct {
	CPUUsage    float64
	MemoryUsage uint64
	MemoryLimit uint64
	DiskUsage   uint64
}

// SnapshotInfo holds metadata about a point-in-time snapshot.
type SnapshotInfo struct {
	ID        string
	Tag       string
	SandboxID string
	CreatedAt time.Time
	Size      int64
}

// Info holds runtime metadata about a sandbox instance.
type Info struct {
	ID     string
	Name   string
	Image  string
	State  string
	IP     string
	Ports  []PortMapping
	Labels map[string]string
}

// Runtime defines the interface that all sandbox isolation backends must implement.
type Runtime interface {
	// Name returns the runtime backend name (e.g., "docker", "firecracker").
	Name() string

	// Create creates a new sandbox instance without starting it.
	Create(ctx context.Context, cfg *CreateConfig) (id string, err error)

	// Start starts a previously created sandbox.
	Start(ctx context.Context, id string) error

	// Stop stops a running sandbox.
	Stop(ctx context.Context, id string) error

	// Destroy removes a sandbox and its resources.
	Destroy(ctx context.Context, id string) error

	// Exec executes a command inside a running sandbox.
	Exec(ctx context.Context, id string, cfg *ExecConfig) (ExecResult, error)

	// Info returns metadata about a sandbox instance.
	Info(ctx context.Context, id string) (Info, error)

	// Stats returns resource usage statistics.
	Stats(ctx context.Context, id string) (Stats, error)

	// List returns all sandbox instances managed by this runtime.
	List(ctx context.Context) ([]Info, error)

	// Snapshot creates a point-in-time snapshot of a sandbox, returning a snapshot ID.
	Snapshot(ctx context.Context, id, tag string) (snapshotID string, err error)

	// Restore creates a new sandbox from a snapshot.
	Restore(ctx context.Context, snapshotID string, cfg *CreateConfig) (id string, err error)

	// ListSnapshots returns all snapshots for a sandbox.
	ListSnapshots(ctx context.Context, id string) ([]SnapshotInfo, error)

	// DeleteSnapshot removes a snapshot.
	DeleteSnapshot(ctx context.Context, snapshotID string) error

	// CreateNetwork creates a Docker network with the given name. If internal
	// is true the network has no external connectivity.
	CreateNetwork(ctx context.Context, name string, internal bool) error

	// DeleteNetwork removes a Docker network by name.
	DeleteNetwork(ctx context.Context, name string) error
}
