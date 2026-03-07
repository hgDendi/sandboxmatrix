package pool

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/hg-dendi/sandboxmatrix/internal/runtime"
	"github.com/hg-dendi/sandboxmatrix/internal/state"
)

// ---------------------------------------------------------------------------
// Mock Runtime
// ---------------------------------------------------------------------------

type mockContainer struct {
	id      string
	cfg     runtime.CreateConfig
	running bool
}

type mockRuntime struct {
	mu         sync.Mutex
	containers map[string]*mockContainer
	nextID     int

	// Optional hooks for injecting errors.
	createErr error
	startErr  error
}

func newMockRuntime() *mockRuntime {
	return &mockRuntime{
		containers: make(map[string]*mockContainer),
	}
}

func (m *mockRuntime) Name() string { return "mock" }

func (m *mockRuntime) Create(_ context.Context, cfg *runtime.CreateConfig) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.createErr != nil {
		return "", m.createErr
	}
	m.nextID++
	id := fmt.Sprintf("mock-%d", m.nextID)
	m.containers[id] = &mockContainer{id: id, cfg: *cfg, running: false}
	return id, nil
}

func (m *mockRuntime) Start(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.startErr != nil {
		return m.startErr
	}
	c, ok := m.containers[id]
	if !ok {
		return fmt.Errorf("container %q not found", id)
	}
	c.running = true
	return nil
}

func (m *mockRuntime) Stop(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.containers[id]
	if !ok {
		return fmt.Errorf("container %q not found", id)
	}
	c.running = false
	return nil
}

func (m *mockRuntime) Destroy(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.containers[id]; !ok {
		return fmt.Errorf("container %q not found", id)
	}
	delete(m.containers, id)
	return nil
}

func (m *mockRuntime) Exec(_ context.Context, _ string, _ *runtime.ExecConfig) (runtime.ExecResult, error) {
	return runtime.ExecResult{ExitCode: 0}, nil
}

func (m *mockRuntime) Info(_ context.Context, id string) (runtime.Info, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.containers[id]
	if !ok {
		return runtime.Info{}, fmt.Errorf("container %q not found", id)
	}
	st := "stopped"
	if c.running {
		st = "running"
	}
	return runtime.Info{
		ID:     c.id,
		Name:   c.cfg.Name,
		Image:  c.cfg.Image,
		State:  st,
		Labels: c.cfg.Labels,
	}, nil
}

func (m *mockRuntime) Stats(_ context.Context, _ string) (runtime.Stats, error) {
	return runtime.Stats{}, nil
}

func (m *mockRuntime) List(_ context.Context) ([]runtime.Info, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var infos []runtime.Info
	for _, c := range m.containers {
		st := "stopped"
		if c.running {
			st = "running"
		}
		infos = append(infos, runtime.Info{
			ID:     c.id,
			Name:   c.cfg.Name,
			Image:  c.cfg.Image,
			State:  st,
			Labels: c.cfg.Labels,
		})
	}
	return infos, nil
}

func (m *mockRuntime) Snapshot(_ context.Context, id, tag string) (string, error) {
	return fmt.Sprintf("sha256:snap-%s-%s", id, tag), nil
}

func (m *mockRuntime) Restore(_ context.Context, snapshotID string, cfg *runtime.CreateConfig) (string, error) {
	cfg.Image = snapshotID
	return m.Create(context.Background(), cfg)
}

func (m *mockRuntime) ListSnapshots(_ context.Context, _ string) ([]runtime.SnapshotInfo, error) {
	return nil, nil
}

func (m *mockRuntime) DeleteSnapshot(_ context.Context, _ string) error {
	return nil
}

func (m *mockRuntime) CreateNetwork(_ context.Context, _ string, _ bool) error {
	return nil
}

func (m *mockRuntime) DeleteNetwork(_ context.Context, _ string) error {
	return nil
}

func (m *mockRuntime) containerCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.containers)
}

func (m *mockRuntime) containerLabels(id string) map[string]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.containers[id]
	if !ok {
		return nil
	}
	return c.cfg.Labels
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// writeTempBlueprint creates a valid blueprint YAML in a temp file.
func writeTempBlueprint(t *testing.T) string {
	t.Helper()
	content := `apiVersion: smx/v1alpha1
kind: Blueprint
metadata:
  name: test-bp
  version: "1.0.0"
spec:
  base: python:3.12-slim
  runtime: docker
  resources:
    cpu: "1"
    memory: 512Mi
  workspace:
    mountPath: /workspace
`
	dir := t.TempDir()
	p := filepath.Join(dir, "blueprint.yaml")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func newTestPool(t *testing.T) (*Pool, *mockRuntime, string) {
	t.Helper()
	rt := newMockRuntime()
	store := state.NewMemoryStore()
	pl := New(rt, store)
	bp := writeTempBlueprint(t)
	return pl, rt, bp
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestConfigurePool(t *testing.T) {
	pl, _, bp := newTestPool(t)

	// Valid configuration.
	err := pl.Configure(Config{
		BlueprintPath: bp,
		MinReady:      2,
		MaxSize:       5,
	})
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}

	// Verify pool was created.
	pl.mu.Lock()
	pool, ok := pl.pools[bp]
	pl.mu.Unlock()
	if !ok {
		t.Fatal("expected pool to be configured")
	}
	if pool.MinReady != 2 {
		t.Fatalf("expected MinReady=2, got %d", pool.MinReady)
	}
	if pool.MaxSize != 5 {
		t.Fatalf("expected MaxSize=5, got %d", pool.MaxSize)
	}
	if pool.Blueprint != "test-bp" {
		t.Fatalf("expected Blueprint=test-bp, got %q", pool.Blueprint)
	}
	if pool.image != "python:3.12-slim" {
		t.Fatalf("expected image=python:3.12-slim, got %q", pool.image)
	}
}

func TestConfigurePoolErrors(t *testing.T) {
	pl, _, bp := newTestPool(t)

	// Empty blueprint path.
	if err := pl.Configure(Config{}); err == nil {
		t.Fatal("expected error for empty blueprint path")
	}

	// Negative MinReady.
	if err := pl.Configure(Config{BlueprintPath: bp, MinReady: -1}); err == nil {
		t.Fatal("expected error for negative MinReady")
	}

	// Negative MaxSize.
	if err := pl.Configure(Config{BlueprintPath: bp, MaxSize: -1}); err == nil {
		t.Fatal("expected error for negative MaxSize")
	}

	// MinReady > MaxSize.
	if err := pl.Configure(Config{BlueprintPath: bp, MinReady: 5, MaxSize: 2}); err == nil {
		t.Fatal("expected error for MinReady > MaxSize")
	}

	// Invalid blueprint file.
	if err := pl.Configure(Config{BlueprintPath: "/nonexistent/blueprint.yaml", MinReady: 1, MaxSize: 5}); err == nil {
		t.Fatal("expected error for invalid blueprint path")
	}
}

func TestClaimFromWarmPool(t *testing.T) {
	pl, rt, bp := newTestPool(t)

	err := pl.Configure(Config{
		BlueprintPath: bp,
		MinReady:      2,
		MaxSize:       5,
	})
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}

	// Start warming and wait for it to fill.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := pl.Warm(ctx); err != nil {
		t.Fatalf("Warm: %v", err)
	}

	// Wait for the pool to fill.
	deadline := time.After(5 * time.Second)
	for {
		pl.mu.Lock()
		ready := len(pl.pools[bp].Ready)
		pl.mu.Unlock()
		if ready >= 2 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for pool to fill; ready=%d", ready)
		case <-time.After(10 * time.Millisecond):
		}
	}

	// Claim should return instantly from the warm pool.
	start := time.Now()
	id, err := pl.Claim(ctx, bp)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty container ID from Claim")
	}

	// A claim from a warm pool should be very fast (< 100ms).
	if elapsed > 100*time.Millisecond {
		t.Fatalf("claim from warm pool took %v, expected < 100ms", elapsed)
	}

	// Verify the container exists in the runtime.
	if rt.containerCount() < 1 {
		t.Fatal("expected at least 1 container in runtime")
	}

	// Verify the claimed container has the right labels.
	labels := rt.containerLabels(id)
	if labels == nil {
		t.Fatal("expected container to have labels")
	}
	if labels[LabelPool] != "true" {
		t.Fatalf("expected label %s=true, got %q", LabelPool, labels[LabelPool])
	}

	// Clean up.
	cancel()
}

func TestClaimWhenEmpty(t *testing.T) {
	pl, rt, bp := newTestPool(t)

	err := pl.Configure(Config{
		BlueprintPath: bp,
		MinReady:      0, // No pre-warming.
		MaxSize:       5,
	})
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}

	// Claim without warming; should create on demand.
	ctx := context.Background()
	id, err := pl.Claim(ctx, bp)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty container ID")
	}

	// Verify container exists.
	if rt.containerCount() != 1 {
		t.Fatalf("expected 1 container, got %d", rt.containerCount())
	}

	// Verify labels.
	labels := rt.containerLabels(id)
	if labels[LabelPool] != "true" {
		t.Fatalf("expected pool label on on-demand container")
	}
	if labels[LabelPoolBlueprint] != bp {
		t.Fatalf("expected blueprint label %q, got %q", bp, labels[LabelPoolBlueprint])
	}
}

func TestClaimUnconfiguredBlueprint(t *testing.T) {
	pl, _, _ := newTestPool(t)

	_, err := pl.Claim(context.Background(), "/nonexistent/bp.yaml")
	if err == nil {
		t.Fatal("expected error claiming from unconfigured blueprint")
	}
}

func TestRelease(t *testing.T) {
	pl, rt, bp := newTestPool(t)

	err := pl.Configure(Config{
		BlueprintPath: bp,
		MinReady:      0,
		MaxSize:       5,
	})
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}

	ctx := context.Background()

	// Claim creates on-demand.
	id, err := pl.Claim(ctx, bp)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}

	// Release returns it to the pool.
	if err := pl.Release(ctx, id, bp); err != nil {
		t.Fatalf("Release: %v", err)
	}

	// Container should still exist (not destroyed).
	if rt.containerCount() != 1 {
		t.Fatalf("expected 1 container after release, got %d", rt.containerCount())
	}

	// Pool should have 1 ready.
	pl.mu.Lock()
	ready := len(pl.pools[bp].Ready)
	pl.mu.Unlock()
	if ready != 1 {
		t.Fatalf("expected 1 ready after release, got %d", ready)
	}
}

func TestReleaseWhenPoolFull(t *testing.T) {
	pl, rt, bp := newTestPool(t)

	err := pl.Configure(Config{
		BlueprintPath: bp,
		MinReady:      0,
		MaxSize:       1,
	})
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}

	ctx := context.Background()

	// Claim to create one container.
	id1, err := pl.Claim(ctx, bp)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}

	// Claim another.
	id2, err := pl.Claim(ctx, bp)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}

	// Release the first one.
	if err := pl.Release(ctx, id1, bp); err != nil {
		t.Fatalf("Release id1: %v", err)
	}

	// Release the second - pool is at capacity, should destroy.
	if err := pl.Release(ctx, id2, bp); err != nil {
		t.Fatalf("Release id2: %v", err)
	}

	// Only 1 container should remain (the one returned to the pool).
	if rt.containerCount() != 1 {
		t.Fatalf("expected 1 container after full pool release, got %d", rt.containerCount())
	}
}

func TestReleaseUnconfiguredBlueprint(t *testing.T) {
	pl, rt, _ := newTestPool(t)

	// Manually create a container in the runtime.
	ctx := context.Background()
	id, err := rt.Create(ctx, &runtime.CreateConfig{Name: "orphan", Image: "test"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Release with unconfigured blueprint should destroy.
	if err := pl.Release(ctx, id, "/nonexistent/bp.yaml"); err != nil {
		t.Fatalf("Release: %v", err)
	}

	if rt.containerCount() != 0 {
		t.Fatalf("expected 0 containers after releasing unconfigured, got %d", rt.containerCount())
	}
}

func TestDrain(t *testing.T) {
	pl, rt, bp := newTestPool(t)

	err := pl.Configure(Config{
		BlueprintPath: bp,
		MinReady:      3,
		MaxSize:       10,
	})
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}

	ctx := context.Background()
	if err := pl.Warm(ctx); err != nil {
		t.Fatalf("Warm: %v", err)
	}

	// Wait for pool to fill.
	deadline := time.After(5 * time.Second)
	for {
		pl.mu.Lock()
		ready := len(pl.pools[bp].Ready)
		pl.mu.Unlock()
		if ready >= 3 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for pool to fill")
		case <-time.After(10 * time.Millisecond):
		}
	}

	// Drain should destroy all warm instances.
	if err := pl.Drain(context.Background()); err != nil {
		t.Fatalf("Drain: %v", err)
	}

	// All containers should be gone.
	if rt.containerCount() != 0 {
		t.Fatalf("expected 0 containers after drain, got %d", rt.containerCount())
	}

	// Pool should have 0 ready.
	pl.mu.Lock()
	ready := len(pl.pools[bp].Ready)
	pl.mu.Unlock()
	if ready != 0 {
		t.Fatalf("expected 0 ready after drain, got %d", ready)
	}
}

func TestStats(t *testing.T) {
	pl, _, bp := newTestPool(t)

	err := pl.Configure(Config{
		BlueprintPath: bp,
		MinReady:      2,
		MaxSize:       5,
	})
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}

	ctx := context.Background()

	// Start warming.
	if err := pl.Warm(ctx); err != nil {
		t.Fatalf("Warm: %v", err)
	}

	// Wait for pool to fill.
	deadline := time.After(5 * time.Second)
	for {
		pl.mu.Lock()
		ready := len(pl.pools[bp].Ready)
		pl.mu.Unlock()
		if ready >= 2 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for pool to fill")
		case <-time.After(10 * time.Millisecond):
		}
	}

	// Claim one.
	id, err := pl.Claim(ctx, bp)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	_ = id

	// Give the warmer a moment to potentially create a replacement.
	time.Sleep(200 * time.Millisecond)

	stats := pl.Stats()
	s, ok := stats[bp]
	if !ok {
		t.Fatal("expected stats for blueprint")
	}

	if s.MinReady != 2 {
		t.Fatalf("expected MinReady=2, got %d", s.MinReady)
	}
	if s.MaxSize != 5 {
		t.Fatalf("expected MaxSize=5, got %d", s.MaxSize)
	}
	if s.InUse != 1 {
		t.Fatalf("expected InUse=1, got %d", s.InUse)
	}
	if s.TotalCreated < 2 {
		t.Fatalf("expected TotalCreated>=2, got %d", s.TotalCreated)
	}
	if s.AvgClaimTime <= 0 {
		t.Fatalf("expected AvgClaimTime > 0, got %v", s.AvgClaimTime)
	}
	if s.BlueprintPath != bp {
		t.Fatalf("expected BlueprintPath=%q, got %q", bp, s.BlueprintPath)
	}

	// Clean up.
	if err := pl.Drain(context.Background()); err != nil {
		t.Fatalf("Drain: %v", err)
	}
}

func TestDrainWithoutWarm(t *testing.T) {
	pl, _, bp := newTestPool(t)

	err := pl.Configure(Config{
		BlueprintPath: bp,
		MinReady:      0,
		MaxSize:       5,
	})
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}

	// Drain without ever calling Warm should not panic.
	if err := pl.Drain(context.Background()); err != nil {
		t.Fatalf("Drain: %v", err)
	}
}

func TestStatsEmpty(t *testing.T) {
	pl, _, _ := newTestPool(t)

	stats := pl.Stats()
	if len(stats) != 0 {
		t.Fatalf("expected empty stats, got %d entries", len(stats))
	}
}
