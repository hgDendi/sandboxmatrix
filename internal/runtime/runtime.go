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

// GPUConfig holds GPU passthrough configuration for a sandbox.
type GPUConfig struct {
	Count  int
	Driver string // "nvidia"
}

// DeviceMapping represents a host device to pass through to a container.
type DeviceMapping struct {
	HostPath      string // e.g. /dev/kvm
	ContainerPath string // defaults to HostPath
	Permissions   string // "rwm", "rw", "r"
}

// CreateConfig holds configuration for creating a new sandbox runtime.
type CreateConfig struct {
	Name    string
	Image   string
	CPU     string
	Memory  string
	Disk    string
	GPU     *GPUConfig
	Devices []DeviceMapping
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

	// CopyToContainer copies data from a reader into a file inside the container.
	CopyToContainer(ctx context.Context, id string, destPath string, content io.Reader) error

	// CopyFromContainer reads a file from inside the container.
	CopyFromContainer(ctx context.Context, id string, srcPath string) (io.ReadCloser, error)

	// Pause suspends a running container (freezes all processes).
	Pause(ctx context.Context, id string) error

	// Unpause resumes a paused container.
	Unpause(ctx context.Context, id string) error

	// UpdateResources dynamically adjusts resource limits on a running container.
	UpdateResources(ctx context.Context, id string, cfg ResourceUpdate) error

	// HostInfo returns resource information about the host machine.
	HostInfo(ctx context.Context) (HostResources, error)
}

// ResourceUpdate specifies new resource limits for a running container.
type ResourceUpdate struct {
	CPUQuota int64 // CPU quota in microseconds per 100ms period (100000 = 1 core)
	Memory   int64 // Memory limit in bytes (0 = unchanged)
}

// HostResources describes the host machine's total and available resources.
type HostResources struct {
	TotalCPUs   int     `json:"totalCpus"`
	TotalMemory int64   `json:"totalMemory"`
	UsedMemory  int64   `json:"usedMemory"`
	AvailMemory int64   `json:"availMemory"`
	CPUPercent  float64 `json:"cpuPercent"`
	LoadAvg1    float64 `json:"loadAvg1"`
}

// SandboxPriority defines the eviction priority of a sandbox.
type SandboxPriority int

const (
	PriorityLow      SandboxPriority = 0 // First to be paused/shrunk
	PriorityNormal   SandboxPriority = 1 // Default
	PriorityHigh     SandboxPriority = 2 // Last to be affected
	PriorityCritical SandboxPriority = 3 // Never paused, only shrunk
)
