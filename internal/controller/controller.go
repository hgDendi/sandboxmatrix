// Package controller manages sandbox lifecycle operations.
package controller

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/hg-dendi/sandboxmatrix/internal/observability"
	"github.com/hg-dendi/sandboxmatrix/internal/probe"
	"github.com/hg-dendi/sandboxmatrix/internal/runtime"
	"github.com/hg-dendi/sandboxmatrix/internal/state"
	v1alpha1 "github.com/hg-dendi/sandboxmatrix/pkg/api/v1alpha1"
	"github.com/hg-dendi/sandboxmatrix/pkg/blueprint"
)

// Controller orchestrates sandbox lifecycle through runtime and state.
type Controller struct {
	mu       sync.Mutex
	runtime  runtime.Runtime
	store    state.Store
	sessions state.SessionStore
	matrices state.MatrixStore
}

// New creates a new Controller. The sessions and matrices parameters are
// optional; if nil, the corresponding methods will return an error.
func New(rt runtime.Runtime, store state.Store, sessions state.SessionStore, matrices state.MatrixStore) *Controller {
	return &Controller{runtime: rt, store: store, sessions: sessions, matrices: matrices}
}

// CreateOptions holds options for creating a sandbox.
type CreateOptions struct {
	Name          string
	BlueprintPath string
	WorkspaceDir  string
	NetworkName   string // optional: override network (e.g. for matrix isolation)
}

// buildDeviceConfig converts blueprint device specs into runtime device mappings.
func buildDeviceConfig(devices []v1alpha1.DeviceMapping) []runtime.DeviceMapping {
	var result []runtime.DeviceMapping
	for _, d := range devices {
		// Skip optional devices that don't exist on the host.
		if d.Optional {
			if _, err := os.Stat(d.HostPath); os.IsNotExist(err) {
				continue
			}
		}
		cPath := d.ContainerPath
		if cPath == "" {
			cPath = d.HostPath
		}
		perms := d.Permissions
		if perms == "" {
			perms = "rwm"
		}
		result = append(result, runtime.DeviceMapping{
			HostPath:      d.HostPath,
			ContainerPath: cPath,
			Permissions:   perms,
		})
	}
	return result
}

// recordOp records a sandbox operation metric.
func recordOp(op, result string, start time.Time) {
	observability.Metrics.SandboxOpsTotal.WithLabelValues(op, result).Inc()
	observability.Metrics.SandboxOpDuration.WithLabelValues(op).Observe(time.Since(start).Seconds())
}

// startAndTrack starts a container and updates the sandbox record. If Start
// fails, the orphaned container is destroyed to prevent leaks.
func (c *Controller) startAndTrack(ctx context.Context, runtimeID string, sb *v1alpha1.Sandbox) error {
	if err := c.runtime.Start(ctx, runtimeID); err != nil {
		// Clean up orphaned container to prevent leaks.
		_ = c.runtime.Destroy(ctx, runtimeID)
		sb.Status.State = v1alpha1.SandboxStateError
		sb.Status.Message = err.Error()
		_ = c.store.Save(sb)
		return fmt.Errorf("start runtime: %w", err)
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
	return nil
}

// Create creates a new sandbox from a blueprint.
func (c *Controller) Create(ctx context.Context, opts CreateOptions) (*v1alpha1.Sandbox, error) {
	start := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()

	slog.Info("creating sandbox", "name", opts.Name, "blueprint", opts.BlueprintPath)

	// Check for duplicate name.
	if _, err := c.store.Get(opts.Name); err == nil {
		recordOp("create", "error", start)
		return nil, fmt.Errorf("sandbox %q already exists", opts.Name)
	}

	// Parse and validate blueprint.
	bp, errs := blueprint.ValidateFile(opts.BlueprintPath)
	if len(errs) > 0 {
		recordOp("create", "error", start)
		return nil, fmt.Errorf("invalid blueprint: %v", errs[0])
	}

	// Warn about host network mode.
	if bp.Spec.Network.Policy == v1alpha1.NetworkPolicyHost {
		slog.Warn("sandbox using host network mode", "name", opts.Name)
	}

	// Build runtime config from blueprint.
	cfg := &runtime.CreateConfig{
		Name:   "smx-" + opts.Name,
		Image:  bp.Spec.Base,
		CPU:    bp.Spec.Resources.CPU,
		Memory: bp.Spec.Resources.Memory,
		Labels: map[string]string{
			"sandboxmatrix/sandbox":   opts.Name,
			"sandboxmatrix/blueprint": bp.Metadata.Name,
		},
	}

	// GPU passthrough.
	if bp.Spec.Resources.GPU != nil {
		cfg.GPU = &runtime.GPUConfig{
			Count:  bp.Spec.Resources.GPU.Count,
			Driver: bp.Spec.Resources.GPU.Driver,
		}
	}

	// Device passthrough.
	cfg.Devices = buildDeviceConfig(bp.Spec.Devices)

	// Network policy.
	if opts.NetworkName != "" {
		// Explicit network name takes precedence (e.g. matrix isolation).
		cfg.Network.Mode = opts.NetworkName
	} else {
		switch bp.Spec.Network.Policy {
		case v1alpha1.NetworkPolicyNone:
			cfg.Network.Mode = "none"
		case v1alpha1.NetworkPolicyHost:
			cfg.Network.Mode = "host"
		case v1alpha1.NetworkPolicyIsolate:
			cfg.Network.Mode = "none" // isolated per-sandbox; no external access
		case v1alpha1.NetworkPolicyBridge, "":
			cfg.Network.Mode = "bridge"
		default:
			cfg.Network.Mode = string(bp.Spec.Network.Policy)
		}
	}

	// Allow DNS resolution when requested alongside restrictive policies.
	if bp.Spec.Network.AllowDNS {
		cfg.Network.DNS = []string{"8.8.8.8", "8.8.4.4"}
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
			Source:   opts.WorkspaceDir,
			Target:   mountPath,
			ReadOnly: bp.Spec.Workspace.ReadOnly,
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
			BlueprintRef:  bp.Metadata.Name,
			BlueprintPath: opts.BlueprintPath,
			Resources:     bp.Spec.Resources,
		},
		Status: v1alpha1.SandboxStatus{
			State: v1alpha1.SandboxStateCreating,
		},
	}

	if opts.WorkspaceDir != "" {
		sb.Spec.Workspace = v1alpha1.WorkspaceSpec{
			MountPath: bp.Spec.Workspace.MountPath,
			Source:    opts.WorkspaceDir,
			ReadOnly:  bp.Spec.Workspace.ReadOnly,
		}
	}

	if err := c.store.Save(sb); err != nil {
		recordOp("create", "error", start)
		return nil, fmt.Errorf("save state: %w", err)
	}

	// Create container via runtime.
	runtimeID, err := c.runtime.Create(ctx, cfg)
	if err != nil {
		sb.Status.State = v1alpha1.SandboxStateError
		sb.Status.Message = err.Error()
		_ = c.store.Save(sb)
		recordOp("create", "error", start)
		return nil, fmt.Errorf("create runtime: %w", err)
	}

	// Start the container (cleans up on failure).
	if err := c.startAndTrack(ctx, runtimeID, sb); err != nil {
		recordOp("create", "error", start)
		return nil, err
	}

	// Run readiness probe if configured.
	if bp.Spec.ReadinessProbe != nil {
		_ = c.store.Save(sb) // persist Running state before probe
		runner := probe.NewRunner(c.runtime)
		if err := runner.WaitForReady(ctx, runtimeID, sb.Status.IP, bp.Spec.ReadinessProbe); err != nil {
			sb.Status.ProbeError = err.Error()
			sb.Status.State = v1alpha1.SandboxStateError
			sb.Status.Message = "readiness probe failed: " + err.Error()
			_ = c.store.Save(sb)
			recordOp("create", "error", start)
			return sb, fmt.Errorf("readiness probe failed: %w", err)
		}
		readyAt := time.Now()
		sb.Status.ReadyAt = &readyAt
		sb.Status.State = v1alpha1.SandboxStateReady
	}

	if err := c.store.Save(sb); err != nil {
		recordOp("create", "error", start)
		return nil, fmt.Errorf("save state: %w", err)
	}

	observability.Metrics.SandboxesActive.Inc()
	recordOp("create", "success", start)
	slog.Info("sandbox created", "name", opts.Name, "runtimeID", runtimeID, "duration", time.Since(start))
	return sb, nil
}

// Stop stops a running sandbox.
func (c *Controller) Stop(ctx context.Context, name string) error {
	start := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()

	sb, err := c.store.Get(name)
	if err != nil {
		recordOp("stop", "error", start)
		return err
	}
	if sb.Status.State != v1alpha1.SandboxStateRunning && sb.Status.State != v1alpha1.SandboxStateReady {
		recordOp("stop", "error", start)
		return fmt.Errorf("sandbox %q is not running (state: %s)", name, sb.Status.State)
	}

	if err := c.runtime.Stop(ctx, sb.Status.RuntimeID); err != nil {
		recordOp("stop", "error", start)
		return fmt.Errorf("stop runtime: %w", err)
	}

	now := time.Now()
	sb.Status.State = v1alpha1.SandboxStateStopped
	sb.Status.StoppedAt = &now
	sb.Metadata.UpdatedAt = now

	observability.Metrics.SandboxesActive.Dec()
	recordOp("stop", "success", start)
	slog.Info("sandbox stopped", "name", name)
	return c.store.Save(sb)
}

// Start starts a stopped sandbox.
func (c *Controller) Start(ctx context.Context, name string) error {
	start := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()

	sb, err := c.store.Get(name)
	if err != nil {
		recordOp("start", "error", start)
		return err
	}
	if sb.Status.State != v1alpha1.SandboxStateStopped {
		recordOp("start", "error", start)
		return fmt.Errorf("sandbox %q is not stopped (state: %s)", name, sb.Status.State)
	}

	if err := c.runtime.Start(ctx, sb.Status.RuntimeID); err != nil {
		recordOp("start", "error", start)
		return fmt.Errorf("start runtime: %w", err)
	}

	now := time.Now()
	sb.Status.State = v1alpha1.SandboxStateRunning
	sb.Status.StartedAt = &now
	sb.Status.StoppedAt = nil
	sb.Metadata.UpdatedAt = now

	observability.Metrics.SandboxesActive.Inc()
	recordOp("start", "success", start)
	slog.Info("sandbox started", "name", name)
	return c.store.Save(sb)
}

// Destroy removes a sandbox and cleans up resources.
func (c *Controller) Destroy(ctx context.Context, name string) error {
	start := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()

	sb, err := c.store.Get(name)
	if err != nil {
		recordOp("destroy", "error", start)
		return err
	}

	wasActive := sb.Status.State == v1alpha1.SandboxStateRunning || sb.Status.State == v1alpha1.SandboxStateReady

	if sb.Status.RuntimeID != "" {
		if err := c.runtime.Destroy(ctx, sb.Status.RuntimeID); err != nil {
			recordOp("destroy", "error", start)
			return fmt.Errorf("destroy runtime: %w", err)
		}
	}

	if wasActive {
		observability.Metrics.SandboxesActive.Dec()
	}
	recordOp("destroy", "success", start)
	slog.Info("sandbox destroyed", "name", name)
	return c.store.Delete(name)
}

// Exec executes a command in a running sandbox.
func (c *Controller) Exec(ctx context.Context, name string, cfg *runtime.ExecConfig) (runtime.ExecResult, error) {
	start := time.Now()

	sb, err := c.store.Get(name)
	if err != nil {
		observability.Metrics.ExecTotal.WithLabelValues(name, "error").Inc()
		return runtime.ExecResult{ExitCode: -1}, err
	}
	if sb.Status.State != v1alpha1.SandboxStateRunning && sb.Status.State != v1alpha1.SandboxStateReady {
		observability.Metrics.ExecTotal.WithLabelValues(name, "error").Inc()
		return runtime.ExecResult{ExitCode: -1}, fmt.Errorf("sandbox %q is not running (state: %s)", name, sb.Status.State)
	}

	result, err := c.runtime.Exec(ctx, sb.Status.RuntimeID, cfg)

	duration := time.Since(start)
	observability.Metrics.ExecDuration.WithLabelValues(name).Observe(duration.Seconds())
	if err != nil {
		observability.Metrics.ExecTotal.WithLabelValues(name, "error").Inc()
	} else {
		observability.Metrics.ExecTotal.WithLabelValues(name, "success").Inc()
	}
	return result, err
}

// Snapshot creates a point-in-time snapshot of a sandbox.
func (c *Controller) Snapshot(ctx context.Context, name, tag string) (string, error) {
	start := time.Now()
	sb, err := c.store.Get(name)
	if err != nil {
		recordOp("snapshot", "error", start)
		return "", err
	}
	if sb.Status.RuntimeID == "" {
		recordOp("snapshot", "error", start)
		return "", fmt.Errorf("sandbox %q has no runtime ID", name)
	}

	snapshotID, err := c.runtime.Snapshot(ctx, sb.Status.RuntimeID, tag)
	if err != nil {
		recordOp("snapshot", "error", start)
		return "", fmt.Errorf("snapshot runtime: %w", err)
	}

	recordOp("snapshot", "success", start)
	slog.Info("snapshot created", "sandbox", name, "tag", tag)
	return snapshotID, nil
}

// Restore creates a new sandbox from a snapshot.
func (c *Controller) Restore(ctx context.Context, name, snapshotID, newName string) (*v1alpha1.Sandbox, error) {
	start := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check the source sandbox exists.
	srcSb, err := c.store.Get(name)
	if err != nil {
		recordOp("restore", "error", start)
		return nil, err
	}

	// Check for duplicate new name.
	if _, err := c.store.Get(newName); err == nil {
		recordOp("restore", "error", start)
		return nil, fmt.Errorf("sandbox %q already exists", newName)
	}

	// Build runtime config from the source sandbox.
	cfg := &runtime.CreateConfig{
		Name:   "smx-" + newName,
		Image:  snapshotID,
		CPU:    srcSb.Spec.Resources.CPU,
		Memory: srcSb.Spec.Resources.Memory,
		Labels: map[string]string{
			"sandboxmatrix/sandbox":   newName,
			"sandboxmatrix/blueprint": srcSb.Spec.BlueprintRef,
			"sandboxmatrix/restored":  "true",
		},
	}

	// GPU passthrough.
	if srcSb.Spec.Resources.GPU != nil {
		cfg.GPU = &runtime.GPUConfig{
			Count:  srcSb.Spec.Resources.GPU.Count,
			Driver: srcSb.Spec.Resources.GPU.Driver,
		}
	}

	// Restore device passthrough from the source blueprint path.
	bpPath := srcSb.Spec.BlueprintPath
	if bpPath != "" {
		bp, _ := blueprint.ParseFile(bpPath)
		if bp != nil {
			cfg.Devices = buildDeviceConfig(bp.Spec.Devices)
		}
	}

	// Create sandbox state record.
	now := time.Now()
	sb := &v1alpha1.Sandbox{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Sandbox"},
		Metadata: v1alpha1.ObjectMeta{
			Name:      newName,
			CreatedAt: now,
			UpdatedAt: now,
			Labels: map[string]string{
				"blueprint":     srcSb.Spec.BlueprintRef,
				"restored":      "true",
				"restored-from": name,
			},
		},
		Spec: v1alpha1.SandboxSpec{
			BlueprintRef:  srcSb.Spec.BlueprintRef,
			BlueprintPath: srcSb.Spec.BlueprintPath,
			Resources:     srcSb.Spec.Resources,
			Workspace:     srcSb.Spec.Workspace,
		},
		Status: v1alpha1.SandboxStatus{
			State: v1alpha1.SandboxStateCreating,
		},
	}

	if err := c.store.Save(sb); err != nil {
		recordOp("restore", "error", start)
		return nil, fmt.Errorf("save state: %w", err)
	}

	// Restore via runtime.
	runtimeID, err := c.runtime.Restore(ctx, snapshotID, cfg)
	if err != nil {
		sb.Status.State = v1alpha1.SandboxStateError
		sb.Status.Message = err.Error()
		_ = c.store.Save(sb)
		recordOp("restore", "error", start)
		return nil, fmt.Errorf("restore runtime: %w", err)
	}

	// Start the container (cleans up on failure).
	if err := c.startAndTrack(ctx, runtimeID, sb); err != nil {
		recordOp("restore", "error", start)
		return nil, err
	}

	if err := c.store.Save(sb); err != nil {
		recordOp("restore", "error", start)
		return nil, fmt.Errorf("save state: %w", err)
	}

	observability.Metrics.SandboxesActive.Inc()
	recordOp("restore", "success", start)
	slog.Info("sandbox restored", "name", newName, "from", name)
	return sb, nil
}

// ListSnapshots returns all snapshots for a sandbox.
func (c *Controller) ListSnapshots(ctx context.Context, name string) ([]runtime.SnapshotInfo, error) {
	sb, err := c.store.Get(name)
	if err != nil {
		return nil, err
	}
	if sb.Status.RuntimeID == "" {
		return nil, fmt.Errorf("sandbox %q has no runtime ID", name)
	}

	return c.runtime.ListSnapshots(ctx, sb.Status.RuntimeID)
}

// DeleteSnapshot removes a snapshot.
func (c *Controller) DeleteSnapshot(ctx context.Context, snapshotID string) error {
	return c.runtime.DeleteSnapshot(ctx, snapshotID)
}

// Stats returns resource usage statistics for a running sandbox.
func (c *Controller) Stats(ctx context.Context, name string) (runtime.Stats, error) {
	sb, err := c.store.Get(name)
	if err != nil {
		return runtime.Stats{}, err
	}
	if sb.Status.State != v1alpha1.SandboxStateRunning && sb.Status.State != v1alpha1.SandboxStateReady {
		return runtime.Stats{}, fmt.Errorf("sandbox %q is not running", name)
	}
	return c.runtime.Stats(ctx, sb.Status.RuntimeID)
}

// Get returns a sandbox by name.
func (c *Controller) Get(name string) (*v1alpha1.Sandbox, error) {
	return c.store.Get(name)
}

// List returns all sandboxes.
func (c *Controller) List() ([]*v1alpha1.Sandbox, error) {
	return c.store.List()
}

// Runtime returns the underlying runtime, allowing callers (e.g. the probe
// runner) to access low-level operations.
func (c *Controller) Runtime() runtime.Runtime {
	return c.runtime
}
