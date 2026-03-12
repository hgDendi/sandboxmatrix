// Package firecracker provides a stub implementation of the Runtime interface
// for the Firecracker microVM backend.
//
// Firecracker requires Linux with KVM support (/dev/kvm). It is not available
// on macOS or in environments without hardware virtualization. This package
// serves as a placeholder that demonstrates the pluggable runtime architecture.
// All methods except Name() return "not implemented" errors.
package firecracker

import (
	"context"
	"fmt"
	"io"

	"github.com/hg-dendi/sandboxmatrix/internal/runtime"
)

// ErrNotImplemented is returned by all operational methods of the Firecracker stub.
var ErrNotImplemented = fmt.Errorf("firecracker runtime is not implemented: requires Linux with KVM support")

// Runtime is a stub implementation of runtime.Runtime for Firecracker microVMs.
type Runtime struct{}

// New creates a new Firecracker runtime stub.
func New() *Runtime {
	return &Runtime{}
}

// Name returns the runtime backend name.
func (r *Runtime) Name() string { return "firecracker" }

// Create is not implemented.
func (r *Runtime) Create(_ context.Context, _ *runtime.CreateConfig) (string, error) {
	return "", ErrNotImplemented
}

// Start is not implemented.
func (r *Runtime) Start(_ context.Context, _ string) error {
	return ErrNotImplemented
}

// Stop is not implemented.
func (r *Runtime) Stop(_ context.Context, _ string) error {
	return ErrNotImplemented
}

// Destroy is not implemented.
func (r *Runtime) Destroy(_ context.Context, _ string) error {
	return ErrNotImplemented
}

// Exec is not implemented.
func (r *Runtime) Exec(_ context.Context, _ string, _ *runtime.ExecConfig) (runtime.ExecResult, error) {
	return runtime.ExecResult{ExitCode: -1}, ErrNotImplemented
}

// Info is not implemented.
func (r *Runtime) Info(_ context.Context, _ string) (runtime.Info, error) {
	return runtime.Info{}, ErrNotImplemented
}

// Stats is not implemented.
func (r *Runtime) Stats(_ context.Context, _ string) (runtime.Stats, error) {
	return runtime.Stats{}, ErrNotImplemented
}

// List is not implemented.
func (r *Runtime) List(_ context.Context) ([]runtime.Info, error) {
	return nil, ErrNotImplemented
}

// Snapshot is not implemented.
func (r *Runtime) Snapshot(_ context.Context, _, _ string) (string, error) {
	return "", ErrNotImplemented
}

// Restore is not implemented.
func (r *Runtime) Restore(_ context.Context, _ string, _ *runtime.CreateConfig) (string, error) {
	return "", ErrNotImplemented
}

// ListSnapshots is not implemented.
func (r *Runtime) ListSnapshots(_ context.Context, _ string) ([]runtime.SnapshotInfo, error) {
	return nil, ErrNotImplemented
}

// DeleteSnapshot is not implemented.
func (r *Runtime) DeleteSnapshot(_ context.Context, _ string) error {
	return ErrNotImplemented
}

// CreateNetwork is not implemented.
func (r *Runtime) CreateNetwork(_ context.Context, _ string, _ bool) error {
	return ErrNotImplemented
}

// DeleteNetwork is not implemented.
func (r *Runtime) DeleteNetwork(_ context.Context, _ string) error {
	return ErrNotImplemented
}

// CopyToContainer is not implemented.
func (r *Runtime) CopyToContainer(_ context.Context, _ string, _ string, _ io.Reader) error {
	return ErrNotImplemented
}

// CopyFromContainer is not implemented.
func (r *Runtime) CopyFromContainer(_ context.Context, _ string, _ string) (io.ReadCloser, error) {
	return nil, ErrNotImplemented
}
