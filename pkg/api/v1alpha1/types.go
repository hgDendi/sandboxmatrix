// Package v1alpha1 defines the core API types for sandboxMatrix.
package v1alpha1

import "time"

// SandboxState represents the lifecycle state of a sandbox.
type SandboxState string

const (
	SandboxStatePending    SandboxState = "Pending"
	SandboxStateCreating   SandboxState = "Creating"
	SandboxStateRunning    SandboxState = "Running"
	SandboxStateStopped    SandboxState = "Stopped"
	SandboxStateError      SandboxState = "Error"
	SandboxStateDestroying SandboxState = "Destroying"
	SandboxStateDestroyed  SandboxState = "Destroyed"
)

// ObjectMeta contains metadata common to all API objects.
type ObjectMeta struct {
	Name      string            `json:"name" yaml:"name"`
	Version   string            `json:"version,omitempty" yaml:"version,omitempty"`
	Labels    map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
	CreatedAt time.Time         `json:"createdAt,omitempty" yaml:"createdAt,omitempty"`
	UpdatedAt time.Time         `json:"updatedAt,omitempty" yaml:"updatedAt,omitempty"`
}

// TypeMeta describes the API version and kind of an object.
type TypeMeta struct {
	APIVersion string `json:"apiVersion" yaml:"apiVersion"`
	Kind       string `json:"kind" yaml:"kind"`
}

// Resources defines compute resource limits.
type Resources struct {
	CPU    string `json:"cpu,omitempty" yaml:"cpu,omitempty"`
	Memory string `json:"memory,omitempty" yaml:"memory,omitempty"`
	Disk   string `json:"disk,omitempty" yaml:"disk,omitempty"`
}

// SetupStep defines a single setup command.
type SetupStep struct {
	Run string `json:"run" yaml:"run"`
}

// Toolchain defines a development toolchain sidecar.
type Toolchain struct {
	Name  string `json:"name" yaml:"name"`
	Image string `json:"image" yaml:"image"`
}

// WorkspaceSpec defines workspace mounting configuration.
type WorkspaceSpec struct {
	MountPath string `json:"mountPath" yaml:"mountPath"`
	Source    string `json:"source,omitempty" yaml:"source,omitempty"`
	ReadOnly  bool   `json:"readOnly,omitempty" yaml:"readOnly,omitempty"`
}

// NetworkSpec defines network configuration.
type NetworkSpec struct {
	Expose []int `json:"expose,omitempty" yaml:"expose,omitempty"`
}

// BlueprintSpec defines the desired state of a sandbox environment.
type BlueprintSpec struct {
	Base       string        `json:"base" yaml:"base"`
	Runtime    string        `json:"runtime" yaml:"runtime"`
	Resources  Resources     `json:"resources,omitempty" yaml:"resources,omitempty"`
	Setup      []SetupStep   `json:"setup,omitempty" yaml:"setup,omitempty"`
	Toolchains []Toolchain   `json:"toolchains,omitempty" yaml:"toolchains,omitempty"`
	Workspace  WorkspaceSpec `json:"workspace,omitempty" yaml:"workspace,omitempty"`
	Network    NetworkSpec   `json:"network,omitempty" yaml:"network,omitempty"`
}

// Blueprint defines a reusable sandbox environment template.
type Blueprint struct {
	TypeMeta `json:",inline" yaml:",inline"`
	Metadata ObjectMeta    `json:"metadata" yaml:"metadata"`
	Spec     BlueprintSpec `json:"spec" yaml:"spec"`
}

// SandboxSpec defines the desired state of a sandbox.
type SandboxSpec struct {
	BlueprintRef string        `json:"blueprintRef" yaml:"blueprintRef"`
	Resources    Resources     `json:"resources,omitempty" yaml:"resources,omitempty"`
	Workspace    WorkspaceSpec `json:"workspace,omitempty" yaml:"workspace,omitempty"`
}

// SandboxStatus holds the observed state of a sandbox.
type SandboxStatus struct {
	State     SandboxState `json:"state" yaml:"state"`
	RuntimeID string       `json:"runtimeID,omitempty" yaml:"runtimeID,omitempty"`
	IP        string       `json:"ip,omitempty" yaml:"ip,omitempty"`
	StartedAt *time.Time   `json:"startedAt,omitempty" yaml:"startedAt,omitempty"`
	StoppedAt *time.Time   `json:"stoppedAt,omitempty" yaml:"stoppedAt,omitempty"`
	Message   string       `json:"message,omitempty" yaml:"message,omitempty"`
}

// Sandbox represents an isolated development environment.
type Sandbox struct {
	TypeMeta `json:",inline" yaml:",inline"`
	Metadata ObjectMeta    `json:"metadata" yaml:"metadata"`
	Spec     SandboxSpec   `json:"spec" yaml:"spec"`
	Status   SandboxStatus `json:"status" yaml:"status"`
}

// SessionState represents the lifecycle state of a session.
type SessionState string

const (
	SessionStateActive    SessionState = "Active"
	SessionStateCompleted SessionState = "Completed"
	SessionStateFailed    SessionState = "Failed"
)

// Session represents a bounded AI agent execution context.
type Session struct {
	TypeMeta  `json:",inline" yaml:",inline"`
	Metadata  ObjectMeta   `json:"metadata" yaml:"metadata"`
	Sandbox   string       `json:"sandbox" yaml:"sandbox"`
	State     SessionState `json:"state" yaml:"state"`
	StartedAt *time.Time   `json:"startedAt,omitempty" yaml:"startedAt,omitempty"`
	EndedAt   *time.Time   `json:"endedAt,omitempty" yaml:"endedAt,omitempty"`
	ExecCount int          `json:"execCount" yaml:"execCount"`
}

// Matrix represents a group of coordinated sandboxes.
type Matrix struct {
	TypeMeta `json:",inline" yaml:",inline"`
	Metadata ObjectMeta `json:"metadata" yaml:"metadata"`
	Members  []string   `json:"members" yaml:"members"`
}
