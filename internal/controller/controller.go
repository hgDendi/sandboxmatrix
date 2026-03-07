// Package controller manages sandbox lifecycle operations.
package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/hg-dendi/sandboxmatrix/internal/runtime"
	"github.com/hg-dendi/sandboxmatrix/internal/state"
	v1alpha1 "github.com/hg-dendi/sandboxmatrix/pkg/api/v1alpha1"
	"github.com/hg-dendi/sandboxmatrix/pkg/blueprint"
)

// Controller orchestrates sandbox lifecycle through runtime and state.
type Controller struct {
	runtime runtime.Runtime
	store   state.Store
}

// New creates a new Controller.
func New(rt runtime.Runtime, store state.Store) *Controller {
	return &Controller{runtime: rt, store: store}
}

// CreateOptions holds options for creating a sandbox.
type CreateOptions struct {
	Name          string
	BlueprintPath string
	WorkspaceDir  string
}

// Create creates a new sandbox from a blueprint.
func (c *Controller) Create(ctx context.Context, opts CreateOptions) (*v1alpha1.Sandbox, error) {
	// Check for duplicate name.
	if _, err := c.store.Get(opts.Name); err == nil {
		return nil, fmt.Errorf("sandbox %q already exists", opts.Name)
	}

	// Parse and validate blueprint.
	bp, errs := blueprint.ValidateFile(opts.BlueprintPath)
	if len(errs) > 0 {
		return nil, fmt.Errorf("invalid blueprint: %v", errs[0])
	}

	// Build runtime config from blueprint.
	cfg := runtime.CreateConfig{
		Name:   "smx-" + opts.Name,
		Image:  bp.Spec.Base,
		CPU:    bp.Spec.Resources.CPU,
		Memory: bp.Spec.Resources.Memory,
		Labels: map[string]string{
			"sandboxmatrix/sandbox": opts.Name,
			"sandboxmatrix/blueprint": bp.Metadata.Name,
		},
	}

	// Port mappings.
	for _, port := range bp.Spec.Network.Expose {
		cfg.Ports = append(cfg.Ports, runtime.PortMapping{
			ContainerPort: port,
			Protocol:      "tcp",
		})
	}

	// Workspace mount.
	if opts.WorkspaceDir != "" {
		mountPath := bp.Spec.Workspace.MountPath
		if mountPath == "" {
			mountPath = "/workspace"
		}
		cfg.Mounts = append(cfg.Mounts, runtime.Mount{
			Source: opts.WorkspaceDir,
			Target: mountPath,
		})
	}

	// Create sandbox state record.
	now := time.Now()
	sb := &v1alpha1.Sandbox{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Sandbox"},
		Metadata: v1alpha1.ObjectMeta{
			Name:      opts.Name,
			CreatedAt: now,
			UpdatedAt: now,
			Labels: map[string]string{
				"blueprint": bp.Metadata.Name,
			},
		},
		Spec: v1alpha1.SandboxSpec{
			BlueprintRef: bp.Metadata.Name,
			Resources:    bp.Spec.Resources,
		},
		Status: v1alpha1.SandboxStatus{
			State: v1alpha1.SandboxStateCreating,
		},
	}

	if opts.WorkspaceDir != "" {
		sb.Spec.Workspace = v1alpha1.WorkspaceSpec{
			MountPath: bp.Spec.Workspace.MountPath,
			Source:    opts.WorkspaceDir,
		}
	}

	if err := c.store.Save(sb); err != nil {
		return nil, fmt.Errorf("save state: %w", err)
	}

	// Create container via runtime.
	runtimeID, err := c.runtime.Create(ctx, cfg)
	if err != nil {
		sb.Status.State = v1alpha1.SandboxStateError
		sb.Status.Message = err.Error()
		_ = c.store.Save(sb)
		return nil, fmt.Errorf("create runtime: %w", err)
	}

	// Start the container.
	if err := c.runtime.Start(ctx, runtimeID); err != nil {
		sb.Status.State = v1alpha1.SandboxStateError
		sb.Status.Message = err.Error()
		_ = c.store.Save(sb)
		return nil, fmt.Errorf("start runtime: %w", err)
	}

	startedAt := time.Now()
	sb.Status.State = v1alpha1.SandboxStateRunning
	sb.Status.RuntimeID = runtimeID
	sb.Status.StartedAt = &startedAt
	sb.Metadata.UpdatedAt = startedAt

	// Get IP address.
	info, err := c.runtime.Info(ctx, runtimeID)
	if err == nil {
		sb.Status.IP = info.IP
	}

	if err := c.store.Save(sb); err != nil {
		return nil, fmt.Errorf("save state: %w", err)
	}

	return sb, nil
}

// Stop stops a running sandbox.
func (c *Controller) Stop(ctx context.Context, name string) error {
	sb, err := c.store.Get(name)
	if err != nil {
		return err
	}
	if sb.Status.State != v1alpha1.SandboxStateRunning {
		return fmt.Errorf("sandbox %q is not running (state: %s)", name, sb.Status.State)
	}

	if err := c.runtime.Stop(ctx, sb.Status.RuntimeID); err != nil {
		return fmt.Errorf("stop runtime: %w", err)
	}

	now := time.Now()
	sb.Status.State = v1alpha1.SandboxStateStopped
	sb.Status.StoppedAt = &now
	sb.Metadata.UpdatedAt = now
	return c.store.Save(sb)
}

// Start starts a stopped sandbox.
func (c *Controller) Start(ctx context.Context, name string) error {
	sb, err := c.store.Get(name)
	if err != nil {
		return err
	}
	if sb.Status.State != v1alpha1.SandboxStateStopped {
		return fmt.Errorf("sandbox %q is not stopped (state: %s)", name, sb.Status.State)
	}

	if err := c.runtime.Start(ctx, sb.Status.RuntimeID); err != nil {
		return fmt.Errorf("start runtime: %w", err)
	}

	now := time.Now()
	sb.Status.State = v1alpha1.SandboxStateRunning
	sb.Status.StartedAt = &now
	sb.Status.StoppedAt = nil
	sb.Metadata.UpdatedAt = now
	return c.store.Save(sb)
}

// Destroy removes a sandbox and cleans up resources.
func (c *Controller) Destroy(ctx context.Context, name string) error {
	sb, err := c.store.Get(name)
	if err != nil {
		return err
	}

	if sb.Status.RuntimeID != "" {
		if err := c.runtime.Destroy(ctx, sb.Status.RuntimeID); err != nil {
			return fmt.Errorf("destroy runtime: %w", err)
		}
	}

	return c.store.Delete(name)
}

// Exec executes a command in a running sandbox.
func (c *Controller) Exec(ctx context.Context, name string, cfg runtime.ExecConfig) (runtime.ExecResult, error) {
	sb, err := c.store.Get(name)
	if err != nil {
		return runtime.ExecResult{ExitCode: -1}, err
	}
	if sb.Status.State != v1alpha1.SandboxStateRunning {
		return runtime.ExecResult{ExitCode: -1}, fmt.Errorf("sandbox %q is not running (state: %s)", name, sb.Status.State)
	}

	return c.runtime.Exec(ctx, sb.Status.RuntimeID, cfg)
}

// Get returns a sandbox by name.
func (c *Controller) Get(name string) (*v1alpha1.Sandbox, error) {
	return c.store.Get(name)
}

// List returns all sandboxes.
func (c *Controller) List() ([]*v1alpha1.Sandbox, error) {
	return c.store.List()
}
