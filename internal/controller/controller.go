// Package controller manages sandbox lifecycle operations.
package controller

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"

	imagepkg "github.com/hg-dendi/sandboxmatrix/internal/image"
	"github.com/hg-dendi/sandboxmatrix/internal/observability"
	"github.com/hg-dendi/sandboxmatrix/internal/probe"
	"github.com/hg-dendi/sandboxmatrix/internal/quota"
	"github.com/hg-dendi/sandboxmatrix/internal/runtime"
	"github.com/hg-dendi/sandboxmatrix/internal/state"
	v1alpha1 "github.com/hg-dendi/sandboxmatrix/pkg/api/v1alpha1"
	"github.com/hg-dendi/sandboxmatrix/pkg/blueprint"
)

// QuotaChecker validates resource quotas before creating resources.
type QuotaChecker interface {
	CheckQuota(ctx context.Context, team string, request quota.QuotaRequest) error
}

// ImageChecker can check whether a pre-built image exists for a blueprint.
type ImageChecker interface {
	InspectImage(ctx context.Context, ref string) (string, error)
}

// Controller orchestrates sandbox lifecycle through runtime and state.
type Controller struct {
	mu           sync.Mutex
	runtime      runtime.Runtime
	store        state.Store
	sessions     state.SessionStore
	matrices     state.MatrixStore
	quotaChecker QuotaChecker
}

// ControllerOption configures the Controller.
type ControllerOption func(*Controller)

// WithQuotaChecker sets the quota checker for the controller.
func WithQuotaChecker(qc QuotaChecker) ControllerOption {
	return func(c *Controller) { c.quotaChecker = qc }
}

// New creates a new Controller. The sessions and matrices parameters are
// optional; if nil, the corresponding methods will return an error.
func New(rt runtime.Runtime, store state.Store, sessions state.SessionStore, matrices state.MatrixStore, opts ...ControllerOption) *Controller {
	c := &Controller{runtime: rt, store: store, sessions: sessions, matrices: matrices}
	for _, o := range opts {
		o(c)
	}
	return c
}

// CreateOptions holds options for creating a sandbox.
type CreateOptions struct {
	Name          string
	BlueprintPath string
	WorkspaceDir  string
	NetworkName   string            // optional: override network (e.g. for matrix isolation)
	Env           map[string]string // optional: environment variables to inject (overrides blueprint env)
	Team          string            // optional: team namespace for quota enforcement
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

// resolveSecrets resolves SecretRef values into a map of environment variables.
// Sources can be "env:<HOST_VAR>" (host environment variable), "file:<path>"
// (file contents), or a literal value. File read errors are handled gracefully
// by setting an empty string.
func resolveSecrets(secrets []v1alpha1.SecretRef) map[string]string {
	result := make(map[string]string)
	for _, s := range secrets {
		if strings.HasPrefix(s.Source, "env:") {
			envName := strings.TrimPrefix(s.Source, "env:")
			result[s.Name] = os.Getenv(envName)
		} else if strings.HasPrefix(s.Source, "file:") {
			filePath := strings.TrimPrefix(s.Source, "file:")
			data, err := os.ReadFile(filePath)
			if err == nil {
				result[s.Name] = strings.TrimSpace(string(data))
			} else {
				result[s.Name] = ""
			}
		} else {
			result[s.Name] = s.Source
		}
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
	ctx, span := observability.StartSpan(ctx, "controller", "CreateSandbox",
		attribute.String(observability.AttrSandboxName, opts.Name),
		attribute.String(observability.AttrBlueprintName, opts.BlueprintPath),
	)
	defer span.End()

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

	// Enforce team quota if configured.
	if opts.Team != "" && c.quotaChecker != nil {
		gpus := 0
		if bp.Spec.Resources.GPU != nil {
			gpus = bp.Spec.Resources.GPU.Count
		}
		qr := quota.QuotaRequest{
			Sandboxes: 1,
			CPU:       bp.Spec.Resources.CPU,
			Memory:    bp.Spec.Resources.Memory,
			Disk:      bp.Spec.Resources.Disk,
			GPUs:      gpus,
		}
		if err := c.quotaChecker.CheckQuota(ctx, opts.Team, qr); err != nil {
			recordOp("create", "error", start)
			return nil, fmt.Errorf("quota check: %w", err)
		}
	}

	// Check for a cached pre-built image; use it to skip setup steps.
	baseImage := bp.Spec.Base
	usedCache := false
	if cachedTag, ok := c.CachedImage(ctx, bp); ok {
		slog.Info("using cached image", "name", opts.Name, "image", cachedTag)
		baseImage = cachedTag
		usedCache = true
	}

	// Build runtime config from blueprint.
	cfg := &runtime.CreateConfig{
		Name:   "smx-" + opts.Name,
		Image:  baseImage,
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

	// Build environment variables: blueprint env -> resolved secrets -> explicit options.
	envVars := make(map[string]string)
	for k, v := range bp.Spec.Env {
		envVars[k] = v
	}
	for k, v := range resolveSecrets(bp.Spec.Secrets) {
		envVars[k] = v
	}
	for k, v := range opts.Env {
		envVars[k] = v
	}
	if len(envVars) > 0 {
		cfg.Env = envVars
	}

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

	// Validate workspace directory.
	if opts.WorkspaceDir != "" {
		cleanedWS := filepath.Clean(opts.WorkspaceDir)
		if filepath.IsAbs(cleanedWS) {
			allowed := false
			for _, prefix := range []string{"/home/", "/Users/", "/tmp/", "/workspace/", "/opt/"} {
				if strings.HasPrefix(cleanedWS, prefix) {
					allowed = true
					break
				}
			}
			if !allowed {
				recordOp("create", "error", start)
				return nil, fmt.Errorf("workspace path %q is not in an allowed directory", opts.WorkspaceDir)
			}
		}
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
			Team:          opts.Team,
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

	// Run setup commands if no cached image was used.
	if !usedCache && len(bp.Spec.Setup) > 0 {
		for i, step := range bp.Spec.Setup {
			result, execErr := c.runtime.Exec(ctx, runtimeID, &runtime.ExecConfig{
				Cmd: []string{"sh", "-c", step.Run},
			})
			if execErr != nil {
				_ = c.runtime.Destroy(ctx, runtimeID)
				sb.Status.State = v1alpha1.SandboxStateError
				sb.Status.Message = fmt.Sprintf("setup step %d failed: %v", i+1, execErr)
				_ = c.store.Save(sb)
				recordOp("create", "error", start)
				return nil, fmt.Errorf("setup step %d failed: %w", i+1, execErr)
			}
			if result.ExitCode != 0 {
				_ = c.runtime.Destroy(ctx, runtimeID)
				sb.Status.State = v1alpha1.SandboxStateError
				sb.Status.Message = fmt.Sprintf("setup step %d exited with code %d: %s", i+1, result.ExitCode, step.Run)
				_ = c.store.Save(sb)
				recordOp("create", "error", start)
				return nil, fmt.Errorf("setup step %d exited with code %d: %s", i+1, result.ExitCode, step.Run)
			}
		}
	}

	// Run readiness probe if configured.
	if bp.Spec.ReadinessProbe != nil {
		_ = c.store.Save(sb) // persist Running state before probe
		runner := probe.NewRunner(c.runtime)
		if err := runner.WaitForReady(ctx, runtimeID, sb.Status.IP, bp.Spec.ReadinessProbe); err != nil {
			// Destroy the container to prevent leaks.
			_ = c.runtime.Destroy(ctx, runtimeID)
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
	ctx, span := observability.StartSpan(ctx, "controller", "StopSandbox",
		attribute.String(observability.AttrSandboxName, name),
	)
	defer span.End()

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

	if err := c.store.Save(sb); err != nil {
		recordOp("stop", "error", start)
		return err
	}
	observability.Metrics.SandboxesActive.Dec()
	recordOp("stop", "success", start)
	slog.Info("sandbox stopped", "name", name)
	return nil
}

// Start starts a stopped sandbox.
func (c *Controller) Start(ctx context.Context, name string) error {
	ctx, span := observability.StartSpan(ctx, "controller", "StartSandbox",
		attribute.String(observability.AttrSandboxName, name),
	)
	defer span.End()

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

	if err := c.store.Save(sb); err != nil {
		recordOp("start", "error", start)
		return err
	}
	observability.Metrics.SandboxesActive.Inc()
	recordOp("start", "success", start)
	slog.Info("sandbox started", "name", name)
	return nil
}

// Destroy removes a sandbox and cleans up resources.
func (c *Controller) Destroy(ctx context.Context, name string) error {
	ctx, span := observability.StartSpan(ctx, "controller", "DestroySandbox",
		attribute.String(observability.AttrSandboxName, name),
	)
	defer span.End()

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
	ctx, span := observability.StartSpan(ctx, "controller", "Exec",
		attribute.String(observability.AttrSandboxName, name),
		attribute.String(observability.AttrExecCommand, strings.Join(cfg.Cmd, " ")),
	)
	defer span.End()

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
	ctx, span := observability.StartSpan(ctx, "controller", "Snapshot",
		attribute.String(observability.AttrSandboxName, name),
	)
	defer span.End()

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
	ctx, span := observability.StartSpan(ctx, "controller", "Restore",
		attribute.String(observability.AttrSandboxName, name),
	)
	defer span.End()

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
	ctx, span := observability.StartSpan(ctx, "controller", "Stats",
		attribute.String(observability.AttrSandboxName, name),
	)
	defer span.End()

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

// CachedImage checks if a pre-built image exists for this blueprint.
// Returns the image tag and true if a cached image is found, or empty
// string and false if not available.
func (c *Controller) CachedImage(ctx context.Context, bp *v1alpha1.Blueprint) (string, bool) {
	checker, ok := c.runtime.(ImageChecker)
	if !ok {
		return "", false
	}

	tag := imagepkg.Tag(bp)
	imageID, err := checker.InspectImage(ctx, tag)
	if err != nil || imageID == "" {
		return "", false
	}
	return tag, true
}

// --------------------------------------------------------------------
// File operations
// --------------------------------------------------------------------

// FileInfo holds metadata about a file inside a sandbox.
type FileInfo struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	IsDir   bool   `json:"isDir"`
	ModTime string `json:"modTime"`
}

// validatePath checks that a file path is absolute and does not contain
// directory traversal sequences.
func validatePath(path string) error {
	if path == "" {
		return fmt.Errorf("path is required")
	}
	if !filepath.IsAbs(path) {
		return fmt.Errorf("path must be absolute")
	}
	if strings.Contains(path, "..") {
		return fmt.Errorf("path must not contain '..'")
	}
	return nil
}

// runningSandboxID returns the runtime ID of a running sandbox, or an error
// if the sandbox does not exist or is not in a running/ready state.
func (c *Controller) runningSandboxID(name string) (string, error) {
	sb, err := c.store.Get(name)
	if err != nil {
		return "", err
	}
	if sb.Status.State != v1alpha1.SandboxStateRunning && sb.Status.State != v1alpha1.SandboxStateReady {
		return "", fmt.Errorf("sandbox %q is not running (state: %s)", name, sb.Status.State)
	}
	return sb.Status.RuntimeID, nil
}

// WriteFile writes content to a file inside a sandbox.
func (c *Controller) WriteFile(ctx context.Context, name, path string, content io.Reader) error {
	if err := validatePath(path); err != nil {
		return err
	}
	runtimeID, err := c.runningSandboxID(name)
	if err != nil {
		return err
	}
	return c.runtime.CopyToContainer(ctx, runtimeID, path, content)
}

// ReadFile reads a file from a sandbox.
func (c *Controller) ReadFile(ctx context.Context, name, path string) (io.ReadCloser, error) {
	if err := validatePath(path); err != nil {
		return nil, err
	}
	runtimeID, err := c.runningSandboxID(name)
	if err != nil {
		return nil, err
	}
	return c.runtime.CopyFromContainer(ctx, runtimeID, path)
}

// ListFiles lists files in a directory inside a sandbox.
func (c *Controller) ListFiles(ctx context.Context, name, path string) ([]FileInfo, error) {
	if err := validatePath(path); err != nil {
		return nil, err
	}
	runtimeID, err := c.runningSandboxID(name)
	if err != nil {
		return nil, err
	}

	// Use stat -c to get structured output for each entry.
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	result, err := c.runtime.Exec(ctx, runtimeID, &runtime.ExecConfig{
		Cmd:    []string{"sh", "-c", fmt.Sprintf("stat -c '%%n\t%%s\t%%F\t%%Y' %s/* %s/.* 2>/dev/null || true", path, path)},
		Stdout: stdout,
		Stderr: stderr,
	})
	if err != nil {
		return nil, fmt.Errorf("list files: %w", err)
	}
	if result.ExitCode != 0 {
		return nil, fmt.Errorf("list files failed (exit %d): %s", result.ExitCode, stderr.String())
	}

	var files []FileInfo
	for _, line := range strings.Split(stdout.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 4)
		if len(parts) < 4 {
			continue
		}

		fname := filepath.Base(parts[0])
		// Skip . and .. entries.
		if fname == "." || fname == ".." {
			continue
		}

		size, _ := strconv.ParseInt(parts[1], 10, 64)
		isDir := strings.Contains(parts[2], "directory")

		modEpoch, _ := strconv.ParseInt(parts[3], 10, 64)
		modTime := time.Unix(modEpoch, 0).UTC().Format(time.RFC3339)

		files = append(files, FileInfo{
			Name:    fname,
			Path:    filepath.Join(path, fname),
			Size:    size,
			IsDir:   isDir,
			ModTime: modTime,
		})
	}
	return files, nil
}
