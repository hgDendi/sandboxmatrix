// Package autoscale provides a dynamic resource autoscaler that monitors host
// resource usage and adjusts sandbox limits accordingly.
package autoscale

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hg-dendi/sandboxmatrix/internal/runtime"
)

// Config configures the autoscaler behavior.
type Config struct {
	Enabled             bool          `yaml:"enabled" json:"enabled"`
	Interval            time.Duration `yaml:"interval" json:"interval"`                       // Check interval (default 10s)
	MemoryHighWater     float64       `yaml:"memoryHighWater" json:"memoryHighWater"`         // Start shrinking at this % (default 0.80)
	MemoryLowWater      float64       `yaml:"memoryLowWater" json:"memoryLowWater"`           // Start expanding at this % (default 0.50)
	CPUHighWater        float64       `yaml:"cpuHighWater" json:"cpuHighWater"`               // CPU pressure threshold (default 0.85)
	CPULowWater         float64       `yaml:"cpuLowWater" json:"cpuLowWater"`                 // CPU recovery threshold (default 0.40)
	MinMemoryPerSandbox int64         `yaml:"minMemoryPerSandbox" json:"minMemoryPerSandbox"` // Floor in bytes (default 64MB)
	MinCPUPerSandbox    float64       `yaml:"minCpuPerSandbox" json:"minCpuPerSandbox"`       // Floor in cores (default 0.1)
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		Enabled:             false,
		Interval:            10 * time.Second,
		MemoryHighWater:     0.80,
		MemoryLowWater:      0.50,
		CPUHighWater:        0.85,
		CPULowWater:         0.40,
		MinMemoryPerSandbox: 64 * 1024 * 1024, // 64MB
		MinCPUPerSandbox:    0.1,
	}
}

// PressureLevel describes current resource pressure.
type PressureLevel int

const (
	PressureNone     PressureLevel = 0 // Below low water
	PressureNormal   PressureLevel = 1 // Between low and high water
	PressureHigh     PressureLevel = 2 // Above high water, need to shrink
	PressureCritical PressureLevel = 3 // Way above high water, need to pause
)

// SandboxState tracks the autoscaler's view of a sandbox.
type SandboxState struct {
	Name           string
	RuntimeID      string
	Priority       runtime.SandboxPriority
	OriginalCPU    string  // Original blueprint CPU limit
	OriginalMemory string  // Original blueprint memory limit
	CurrentScale   float64 // 0.0-1.0, where 1.0 = original limits
	Paused         bool    // Whether we paused this sandbox
}

// Autoscaler monitors host resources and dynamically adjusts sandbox limits.
type Autoscaler struct {
	mu        sync.Mutex
	rt        runtime.Runtime
	cfg       Config
	sandboxes map[string]*SandboxState
	cancel    context.CancelFunc
	running   bool
}

// New creates a new Autoscaler.
func New(rt runtime.Runtime, cfg Config) *Autoscaler {
	return &Autoscaler{
		rt:        rt,
		cfg:       cfg,
		sandboxes: make(map[string]*SandboxState),
	}
}

// Start begins the autoscaler monitoring loop.
func (a *Autoscaler) Start(ctx context.Context) {
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return
	}
	a.running = true
	ctx, a.cancel = context.WithCancel(ctx)
	a.mu.Unlock()

	slog.Info("autoscaler started", "interval", a.cfg.Interval)
	go a.loop(ctx)
}

// Stop stops the autoscaler and restores all sandboxes to original limits.
func (a *Autoscaler) Stop(ctx context.Context) {
	a.mu.Lock()
	if !a.running {
		a.mu.Unlock()
		return
	}
	a.running = false
	if a.cancel != nil {
		a.cancel()
	}
	a.mu.Unlock()

	// Restore all sandboxes to full resources.
	a.restoreAll(ctx)
	slog.Info("autoscaler stopped, all sandboxes restored")
}

// IsRunning returns whether the autoscaler is currently active.
func (a *Autoscaler) IsRunning() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.running
}

// SetPriority sets the priority for a sandbox (call when creating/updating).
func (a *Autoscaler) SetPriority(name string, priority runtime.SandboxPriority) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if s, ok := a.sandboxes[name]; ok {
		s.Priority = priority
	}
}

// RegisterSandbox adds a sandbox to autoscaler tracking.
func (a *Autoscaler) RegisterSandbox(name, runtimeID, cpu, memory string, priority runtime.SandboxPriority) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sandboxes[name] = &SandboxState{
		Name:           name,
		RuntimeID:      runtimeID,
		Priority:       priority,
		OriginalCPU:    cpu,
		OriginalMemory: memory,
		CurrentScale:   1.0,
	}
}

// UnregisterSandbox removes a sandbox from tracking.
func (a *Autoscaler) UnregisterSandbox(name string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.sandboxes, name)
}

// Status returns the current autoscaler status.
func (a *Autoscaler) Status(ctx context.Context) (*StatusInfo, error) {
	host, err := a.rt.HostInfo(ctx)
	if err != nil {
		return nil, err
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	memPressure := a.memoryPressure(host)
	cpuPressure := a.cpuPressure(host)

	managed := make([]SandboxStatus, 0, len(a.sandboxes))
	for _, s := range a.sandboxes {
		managed = append(managed, SandboxStatus{
			Name:     s.Name,
			Priority: s.Priority,
			Scale:    s.CurrentScale,
			Paused:   s.Paused,
		})
	}

	return &StatusInfo{
		Enabled:        a.cfg.Enabled,
		Running:        a.running,
		HostMemoryUsed: safeDiv(float64(host.UsedMemory), float64(host.TotalMemory)),
		HostCPUUsed:    host.CPUPercent / 100.0,
		MemoryPressure: memPressure,
		CPUPressure:    cpuPressure,
		ManagedCount:   len(a.sandboxes),
		PausedCount:    a.pausedCount(),
		Sandboxes:      managed,
	}, nil
}

// StatusInfo holds the current autoscaler state.
type StatusInfo struct {
	Enabled        bool            `json:"enabled"`
	Running        bool            `json:"running"`
	HostMemoryUsed float64         `json:"hostMemoryUsed"` // 0.0-1.0
	HostCPUUsed    float64         `json:"hostCpuUsed"`    // 0.0-1.0
	MemoryPressure PressureLevel   `json:"memoryPressure"`
	CPUPressure    PressureLevel   `json:"cpuPressure"`
	ManagedCount   int             `json:"managedCount"`
	PausedCount    int             `json:"pausedCount"`
	Sandboxes      []SandboxStatus `json:"sandboxes"`
}

// SandboxStatus is the per-sandbox autoscaler state exposed in the API.
type SandboxStatus struct {
	Name     string                  `json:"name"`
	Priority runtime.SandboxPriority `json:"priority"`
	Scale    float64                 `json:"scale"` // 0.0 = paused, 1.0 = full
	Paused   bool                    `json:"paused"`
}

// loop is the main monitoring loop.
func (a *Autoscaler) loop(ctx context.Context) {
	ticker := time.NewTicker(a.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.reconcile(ctx)
		}
	}
}

// reconcile checks host resources and adjusts sandbox limits.
func (a *Autoscaler) reconcile(ctx context.Context) {
	host, err := a.rt.HostInfo(ctx)
	if err != nil {
		slog.Warn("autoscaler: failed to get host info", "error", err)
		return
	}

	memPressure := a.memoryPressure(host)
	cpuPressure := a.cpuPressure(host)

	// Use the higher pressure level.
	pressure := memPressure
	if cpuPressure > pressure {
		pressure = cpuPressure
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	switch pressure {
	case PressureNone:
		// Resources are plentiful - restore everything.
		a.expandAll(ctx)
	case PressureNormal:
		// Between thresholds - do nothing, maintain current state.
	case PressureHigh:
		// Above high water - shrink sandbox limits.
		a.shrinkByPriority(ctx, 0.5) // Reduce to 50% of original
	case PressureCritical:
		// Way above high water - pause low-priority, shrink the rest.
		a.pauseLowPriority(ctx)
		a.shrinkByPriority(ctx, 0.25) // Reduce to 25% of original
	}
}

func (a *Autoscaler) memoryPressure(host runtime.HostResources) PressureLevel {
	if host.TotalMemory == 0 {
		return PressureNone
	}
	used := float64(host.UsedMemory) / float64(host.TotalMemory)
	if used > a.cfg.MemoryHighWater+0.10 {
		return PressureCritical
	}
	if used > a.cfg.MemoryHighWater {
		return PressureHigh
	}
	if used > a.cfg.MemoryLowWater {
		return PressureNormal
	}
	return PressureNone
}

func (a *Autoscaler) cpuPressure(host runtime.HostResources) PressureLevel {
	used := host.CPUPercent / 100.0
	if used > a.cfg.CPUHighWater+0.10 {
		return PressureCritical
	}
	if used > a.cfg.CPUHighWater {
		return PressureHigh
	}
	if used > a.cfg.CPULowWater {
		return PressureNormal
	}
	return PressureNone
}

func (a *Autoscaler) pausedCount() int {
	n := 0
	for _, s := range a.sandboxes {
		if s.Paused {
			n++
		}
	}
	return n
}

// expandAll restores all sandboxes to original limits and unpauses.
// Caller must hold a.mu.
func (a *Autoscaler) expandAll(ctx context.Context) {
	for _, s := range a.sandboxes {
		if s.Paused {
			if err := a.rt.Unpause(ctx, s.RuntimeID); err != nil {
				slog.Warn("autoscaler: unpause failed", "sandbox", s.Name, "error", err)
				continue
			}
			s.Paused = false
			slog.Info("autoscaler: resumed sandbox", "name", s.Name)
		}
		if s.CurrentScale < 1.0 {
			a.scaleSandbox(ctx, s, 1.0)
		}
	}
}

// shrinkByPriority reduces resource limits, starting with lowest priority.
// Caller must hold a.mu.
func (a *Autoscaler) shrinkByPriority(ctx context.Context, targetScale float64) {
	for _, s := range a.sandboxes {
		if s.Paused {
			continue // Already paused, skip.
		}

		scale := targetScale
		switch s.Priority {
		case runtime.PriorityCritical:
			scale = maxFloat(targetScale, 0.75) // Critical: never below 75%
		case runtime.PriorityHigh:
			scale = maxFloat(targetScale, 0.50) // High: never below 50%
		case runtime.PriorityNormal:
			// Normal: use target scale as-is
		case runtime.PriorityLow:
			scale = targetScale * 0.5 // Low: shrink even more
		}

		if s.CurrentScale != scale {
			a.scaleSandbox(ctx, s, scale)
		}
	}
}

// pauseLowPriority pauses PriorityLow sandboxes.
// Caller must hold a.mu.
func (a *Autoscaler) pauseLowPriority(ctx context.Context) {
	for _, s := range a.sandboxes {
		if s.Priority <= runtime.PriorityLow && !s.Paused {
			if err := a.rt.Pause(ctx, s.RuntimeID); err != nil {
				slog.Warn("autoscaler: pause failed", "sandbox", s.Name, "error", err)
				continue
			}
			s.Paused = true
			s.CurrentScale = 0.0
			slog.Info("autoscaler: paused sandbox", "name", s.Name, "priority", s.Priority)
		}
	}
}

// scaleSandbox applies a new resource scale to a sandbox.
func (a *Autoscaler) scaleSandbox(ctx context.Context, s *SandboxState, scale float64) {
	origMem := parseMemoryBytes(s.OriginalMemory)
	origCPU := parseCPUCores(s.OriginalCPU)

	newMem := int64(float64(origMem) * scale)
	newCPU := origCPU * scale

	// Enforce minimums.
	if newMem < a.cfg.MinMemoryPerSandbox {
		newMem = a.cfg.MinMemoryPerSandbox
	}
	if newCPU < a.cfg.MinCPUPerSandbox {
		newCPU = a.cfg.MinCPUPerSandbox
	}

	update := runtime.ResourceUpdate{
		Memory:   newMem,
		CPUQuota: int64(newCPU * 100000), // Convert cores to microseconds per 100ms
	}

	if err := a.rt.UpdateResources(ctx, s.RuntimeID, update); err != nil {
		slog.Warn("autoscaler: update resources failed", "sandbox", s.Name, "error", err)
		return
	}

	oldScale := s.CurrentScale
	s.CurrentScale = scale
	slog.Info("autoscaler: scaled sandbox",
		"name", s.Name,
		"from", fmt.Sprintf("%.0f%%", oldScale*100),
		"to", fmt.Sprintf("%.0f%%", scale*100),
	)
}

// restoreAll restores all sandboxes to full resources (called on stop).
func (a *Autoscaler) restoreAll(ctx context.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.expandAll(ctx)
}

// parseMemoryBytes converts "2G", "512M", "1Gi", "256Mi" to bytes.
func parseMemoryBytes(s string) int64 {
	if s == "" {
		return 512 * 1024 * 1024 // default 512MB
	}
	s = strings.TrimSpace(s)

	// Handle Kubernetes-style suffixes first.
	if strings.HasSuffix(s, "Gi") {
		if v, err := strconv.ParseFloat(strings.TrimSuffix(s, "Gi"), 64); err == nil {
			return int64(v * 1024 * 1024 * 1024)
		}
		return 512 * 1024 * 1024
	}
	if strings.HasSuffix(s, "Mi") {
		if v, err := strconv.ParseFloat(strings.TrimSuffix(s, "Mi"), 64); err == nil {
			return int64(v * 1024 * 1024)
		}
		return 512 * 1024 * 1024
	}

	multiplier := int64(1)
	unit := s[len(s)-1:]
	numStr := s[:len(s)-1]

	switch strings.ToUpper(unit) {
	case "T":
		multiplier = 1024 * 1024 * 1024 * 1024
	case "G":
		multiplier = 1024 * 1024 * 1024
	case "M":
		multiplier = 1024 * 1024
	case "K":
		multiplier = 1024
	default:
		// Try parsing entire string as bytes.
		if v, err := strconv.ParseInt(s, 10, 64); err == nil {
			return v
		}
		return 512 * 1024 * 1024
	}

	if v, err := strconv.ParseFloat(numStr, 64); err == nil {
		return int64(v * float64(multiplier))
	}
	return 512 * 1024 * 1024
}

// parseCPUCores converts "2", "0.5" to float64 cores.
func parseCPUCores(s string) float64 {
	if s == "" {
		return 1.0
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || v <= 0 {
		return 1.0
	}
	return v
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func safeDiv(num, denom float64) float64 {
	if denom == 0 {
		return 0
	}
	return num / denom
}
