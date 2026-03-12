package autoscale

import (
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/hg-dendi/sandboxmatrix/internal/runtime"
)

// mockRuntime implements runtime.Runtime for autoscaler testing.
type mockRuntime struct {
	pauseCalls          []string
	unpauseCalls        []string
	updateResourceCalls []updateCall
	hostInfo            runtime.HostResources
	hostInfoErr         error
}

type updateCall struct {
	id  string
	cfg runtime.ResourceUpdate
}

func newMockRuntime() *mockRuntime {
	return &mockRuntime{
		hostInfo: runtime.HostResources{
			TotalCPUs:   4,
			TotalMemory: 8 * 1024 * 1024 * 1024,
			UsedMemory:  2 * 1024 * 1024 * 1024,
			AvailMemory: 6 * 1024 * 1024 * 1024,
			CPUPercent:  25,
		},
	}
}

func (m *mockRuntime) Name() string { return "mock" }
func (m *mockRuntime) Create(_ context.Context, _ *runtime.CreateConfig) (string, error) {
	return "", nil
}
func (m *mockRuntime) Start(_ context.Context, _ string) error   { return nil }
func (m *mockRuntime) Stop(_ context.Context, _ string) error    { return nil }
func (m *mockRuntime) Destroy(_ context.Context, _ string) error { return nil }
func (m *mockRuntime) Exec(_ context.Context, _ string, _ *runtime.ExecConfig) (runtime.ExecResult, error) {
	return runtime.ExecResult{}, nil
}
func (m *mockRuntime) Info(_ context.Context, _ string) (runtime.Info, error) {
	return runtime.Info{}, nil
}
func (m *mockRuntime) Stats(_ context.Context, _ string) (runtime.Stats, error) {
	return runtime.Stats{}, nil
}
func (m *mockRuntime) List(_ context.Context) ([]runtime.Info, error)          { return nil, nil }
func (m *mockRuntime) Snapshot(_ context.Context, _, _ string) (string, error) { return "", nil }
func (m *mockRuntime) Restore(_ context.Context, _ string, _ *runtime.CreateConfig) (string, error) {
	return "", nil
}
func (m *mockRuntime) ListSnapshots(_ context.Context, _ string) ([]runtime.SnapshotInfo, error) {
	return nil, nil
}
func (m *mockRuntime) DeleteSnapshot(_ context.Context, _ string) error        { return nil }
func (m *mockRuntime) CreateNetwork(_ context.Context, _ string, _ bool) error { return nil }
func (m *mockRuntime) DeleteNetwork(_ context.Context, _ string) error         { return nil }
func (m *mockRuntime) CopyToContainer(_ context.Context, _ string, _ string, _ io.Reader) error {
	return nil
}
func (m *mockRuntime) CopyFromContainer(_ context.Context, _ string, _ string) (io.ReadCloser, error) {
	return nil, nil
}

func (m *mockRuntime) Pause(_ context.Context, id string) error {
	m.pauseCalls = append(m.pauseCalls, id)
	return nil
}

func (m *mockRuntime) Unpause(_ context.Context, id string) error {
	m.unpauseCalls = append(m.unpauseCalls, id)
	return nil
}

func (m *mockRuntime) UpdateResources(_ context.Context, id string, cfg runtime.ResourceUpdate) error {
	m.updateResourceCalls = append(m.updateResourceCalls, updateCall{id: id, cfg: cfg})
	return nil
}

func (m *mockRuntime) HostInfo(_ context.Context) (runtime.HostResources, error) {
	return m.hostInfo, m.hostInfoErr
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Enabled {
		t.Error("default config should be disabled")
	}
	if cfg.MemoryHighWater != 0.80 {
		t.Errorf("expected MemoryHighWater 0.80, got %f", cfg.MemoryHighWater)
	}
	if cfg.MinMemoryPerSandbox != 64*1024*1024 {
		t.Errorf("expected MinMemoryPerSandbox 64MB, got %d", cfg.MinMemoryPerSandbox)
	}
}

func TestNewAutoscaler(t *testing.T) {
	rt := newMockRuntime()
	cfg := DefaultConfig()
	as := New(rt, cfg)
	if as == nil {
		t.Fatal("expected non-nil autoscaler")
	}
	if as.IsRunning() {
		t.Error("new autoscaler should not be running")
	}
}

func TestRegisterUnregisterSandbox(t *testing.T) {
	rt := newMockRuntime()
	as := New(rt, DefaultConfig())

	as.RegisterSandbox("test-sb", "runtime-1", "2", "1Gi", runtime.PriorityNormal)

	as.mu.Lock()
	if len(as.sandboxes) != 1 {
		t.Errorf("expected 1 sandbox, got %d", len(as.sandboxes))
	}
	s := as.sandboxes["test-sb"]
	if s.RuntimeID != "runtime-1" {
		t.Errorf("expected RuntimeID runtime-1, got %q", s.RuntimeID)
	}
	if s.CurrentScale != 1.0 {
		t.Errorf("expected CurrentScale 1.0, got %f", s.CurrentScale)
	}
	as.mu.Unlock()

	as.UnregisterSandbox("test-sb")
	as.mu.Lock()
	if len(as.sandboxes) != 0 {
		t.Errorf("expected 0 sandboxes after unregister, got %d", len(as.sandboxes))
	}
	as.mu.Unlock()
}

func TestSetPriority(t *testing.T) {
	rt := newMockRuntime()
	as := New(rt, DefaultConfig())

	as.RegisterSandbox("sb", "rt-1", "1", "512Mi", runtime.PriorityLow)
	as.SetPriority("sb", runtime.PriorityHigh)

	as.mu.Lock()
	if as.sandboxes["sb"].Priority != runtime.PriorityHigh {
		t.Errorf("expected PriorityHigh, got %d", as.sandboxes["sb"].Priority)
	}
	as.mu.Unlock()
}

func TestMemoryPressureLevels(t *testing.T) {
	rt := newMockRuntime()
	as := New(rt, DefaultConfig())

	tests := []struct {
		usedRatio float64
		expected  PressureLevel
	}{
		{0.20, PressureNone},     // Below low water (0.50)
		{0.55, PressureNormal},   // Between low (0.50) and high (0.80)
		{0.82, PressureHigh},     // Above high water (0.80)
		{0.95, PressureCritical}, // Above high water + 0.10 (0.90)
	}

	total := int64(8 * 1024 * 1024 * 1024)
	for _, tt := range tests {
		host := runtime.HostResources{
			TotalMemory: total,
			UsedMemory:  int64(float64(total) * tt.usedRatio),
		}
		got := as.memoryPressure(host)
		if got != tt.expected {
			t.Errorf("memoryPressure(usedRatio=%.2f) = %d, want %d", tt.usedRatio, got, tt.expected)
		}
	}
}

func TestCPUPressureLevels(t *testing.T) {
	rt := newMockRuntime()
	as := New(rt, DefaultConfig())

	tests := []struct {
		cpuPercent float64
		expected   PressureLevel
	}{
		{20, PressureNone},     // Below low water (0.40)
		{60, PressureNormal},   // Between low (0.40) and high (0.85)
		{87, PressureHigh},     // Above high water (0.85)
		{97, PressureCritical}, // Above high water + 0.10 (0.95)
	}

	for _, tt := range tests {
		host := runtime.HostResources{CPUPercent: tt.cpuPercent}
		got := as.cpuPressure(host)
		if got != tt.expected {
			t.Errorf("cpuPressure(cpu=%.0f%%) = %d, want %d", tt.cpuPercent, got, tt.expected)
		}
	}
}

func TestReconcileNoPressure(t *testing.T) {
	rt := newMockRuntime()
	rt.hostInfo.UsedMemory = 1 * 1024 * 1024 * 1024 // 12.5% of 8GB
	rt.hostInfo.CPUPercent = 20

	as := New(rt, DefaultConfig())
	as.RegisterSandbox("sb1", "rt-1", "1", "512Mi", runtime.PriorityNormal)

	// Simulate that sb1 was previously shrunk.
	as.mu.Lock()
	as.sandboxes["sb1"].CurrentScale = 0.5
	as.mu.Unlock()

	as.reconcile(context.Background())

	// Should have called UpdateResources to scale back to 1.0.
	if len(rt.updateResourceCalls) == 0 {
		t.Error("expected UpdateResources to be called to restore scale")
	}

	as.mu.Lock()
	if as.sandboxes["sb1"].CurrentScale != 1.0 {
		t.Errorf("expected scale 1.0, got %f", as.sandboxes["sb1"].CurrentScale)
	}
	as.mu.Unlock()
}

func TestReconcileHighPressure(t *testing.T) {
	rt := newMockRuntime()
	rt.hostInfo.UsedMemory = int64(float64(rt.hostInfo.TotalMemory) * 0.85) // 85% - high pressure
	rt.hostInfo.CPUPercent = 20                                             // Low CPU

	as := New(rt, DefaultConfig())
	as.RegisterSandbox("sb1", "rt-1", "2", "1Gi", runtime.PriorityNormal)

	as.reconcile(context.Background())

	// Should have shrunk.
	as.mu.Lock()
	if as.sandboxes["sb1"].CurrentScale >= 1.0 {
		t.Errorf("expected scale < 1.0, got %f", as.sandboxes["sb1"].CurrentScale)
	}
	as.mu.Unlock()

	if len(rt.updateResourceCalls) == 0 {
		t.Error("expected UpdateResources to be called")
	}
}

func TestReconcileCriticalPressurePausesLowPriority(t *testing.T) {
	rt := newMockRuntime()
	rt.hostInfo.UsedMemory = int64(float64(rt.hostInfo.TotalMemory) * 0.95) // 95% - critical
	rt.hostInfo.CPUPercent = 20

	as := New(rt, DefaultConfig())
	as.RegisterSandbox("low-sb", "rt-low", "1", "512Mi", runtime.PriorityLow)
	as.RegisterSandbox("high-sb", "rt-high", "1", "512Mi", runtime.PriorityHigh)

	as.reconcile(context.Background())

	// Low priority sandbox should be paused.
	as.mu.Lock()
	lowSb := as.sandboxes["low-sb"]
	highSb := as.sandboxes["high-sb"]
	as.mu.Unlock()

	if !lowSb.Paused {
		t.Error("expected low priority sandbox to be paused")
	}
	if highSb.Paused {
		t.Error("expected high priority sandbox to NOT be paused")
	}

	// Verify Pause was called for the low-priority sandbox.
	found := false
	for _, id := range rt.pauseCalls {
		if id == "rt-low" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected Pause to be called for low priority sandbox")
	}
}

func TestStatusReturnsCorrectInfo(t *testing.T) {
	rt := newMockRuntime()
	as := New(rt, DefaultConfig())
	as.RegisterSandbox("sb1", "rt-1", "2", "1Gi", runtime.PriorityHigh)

	status, err := as.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}

	if status.ManagedCount != 1 {
		t.Errorf("expected ManagedCount 1, got %d", status.ManagedCount)
	}
	if len(status.Sandboxes) != 1 {
		t.Fatalf("expected 1 sandbox in status, got %d", len(status.Sandboxes))
	}
	if status.Sandboxes[0].Name != "sb1" {
		t.Errorf("expected sandbox name sb1, got %q", status.Sandboxes[0].Name)
	}
	if status.Sandboxes[0].Priority != runtime.PriorityHigh {
		t.Errorf("expected PriorityHigh, got %d", status.Sandboxes[0].Priority)
	}
}

func TestStatusWithHostInfoError(t *testing.T) {
	rt := newMockRuntime()
	rt.hostInfoErr = fmt.Errorf("docker not running")

	as := New(rt, DefaultConfig())
	_, err := as.Status(context.Background())
	if err == nil {
		t.Error("expected error when HostInfo fails")
	}
}

func TestStartStop(t *testing.T) {
	rt := newMockRuntime()
	as := New(rt, DefaultConfig())

	if as.IsRunning() {
		t.Error("should not be running before Start")
	}

	ctx := context.Background()
	as.Start(ctx)

	if !as.IsRunning() {
		t.Error("should be running after Start")
	}

	// Starting again should be a no-op.
	as.Start(ctx)
	if !as.IsRunning() {
		t.Error("should still be running after double Start")
	}

	as.Stop(ctx)
	if as.IsRunning() {
		t.Error("should not be running after Stop")
	}

	// Stopping again should be a no-op.
	as.Stop(ctx)
}

func TestExpandAllUnpausesSandboxes(t *testing.T) {
	rt := newMockRuntime()
	as := New(rt, DefaultConfig())
	as.RegisterSandbox("paused-sb", "rt-1", "1", "512Mi", runtime.PriorityLow)

	as.mu.Lock()
	as.sandboxes["paused-sb"].Paused = true
	as.sandboxes["paused-sb"].CurrentScale = 0.0
	as.mu.Unlock()

	// Set low pressure so expandAll is triggered.
	rt.hostInfo.UsedMemory = 1 * 1024 * 1024 * 1024 // 12.5% of 8GB
	rt.hostInfo.CPUPercent = 10

	as.reconcile(context.Background())

	as.mu.Lock()
	sb := as.sandboxes["paused-sb"]
	as.mu.Unlock()

	if sb.Paused {
		t.Error("expected sandbox to be unpaused after low pressure")
	}
	if sb.CurrentScale != 1.0 {
		t.Errorf("expected scale 1.0 after restore, got %f", sb.CurrentScale)
	}

	if len(rt.unpauseCalls) == 0 {
		t.Error("expected Unpause to be called")
	}
}

func TestParseMemoryBytes(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"", 512 * 1024 * 1024},
		{"1Gi", 1024 * 1024 * 1024},
		{"512Mi", 512 * 1024 * 1024},
		{"2G", 2 * 1024 * 1024 * 1024},
		{"256M", 256 * 1024 * 1024},
		{"1024K", 1024 * 1024},
		{"1048576", 1048576},
		{"bogus", 512 * 1024 * 1024},
	}

	for _, tt := range tests {
		got := parseMemoryBytes(tt.input)
		if got != tt.want {
			t.Errorf("parseMemoryBytes(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestParseCPUCores(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"", 1.0},
		{"2", 2.0},
		{"0.5", 0.5},
		{"-1", 1.0},
		{"bogus", 1.0},
	}

	for _, tt := range tests {
		got := parseCPUCores(tt.input)
		if got != tt.want {
			t.Errorf("parseCPUCores(%q) = %f, want %f", tt.input, got, tt.want)
		}
	}
}

func TestPriorityScaling(t *testing.T) {
	rt := newMockRuntime()
	rt.hostInfo.UsedMemory = int64(float64(rt.hostInfo.TotalMemory) * 0.85) // High pressure
	rt.hostInfo.CPUPercent = 20

	as := New(rt, DefaultConfig())
	as.RegisterSandbox("critical-sb", "rt-crit", "2", "1Gi", runtime.PriorityCritical)
	as.RegisterSandbox("normal-sb", "rt-norm", "2", "1Gi", runtime.PriorityNormal)
	as.RegisterSandbox("low-sb", "rt-low", "2", "1Gi", runtime.PriorityLow)

	as.reconcile(context.Background())

	as.mu.Lock()
	critScale := as.sandboxes["critical-sb"].CurrentScale
	normScale := as.sandboxes["normal-sb"].CurrentScale
	lowScale := as.sandboxes["low-sb"].CurrentScale
	as.mu.Unlock()

	// Critical should be scaled least aggressively.
	if critScale < normScale {
		t.Errorf("critical scale (%f) should be >= normal scale (%f)", critScale, normScale)
	}
	// Normal should be scaled less aggressively than low.
	if normScale < lowScale {
		t.Errorf("normal scale (%f) should be >= low scale (%f)", normScale, lowScale)
	}
}
