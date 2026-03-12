package controller

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/hg-dendi/sandboxmatrix/internal/runtime"
	"github.com/hg-dendi/sandboxmatrix/internal/state"
	v1alpha1 "github.com/hg-dendi/sandboxmatrix/pkg/api/v1alpha1"
)

// ---------------------------------------------------------------------------
// Mock Runtime
// ---------------------------------------------------------------------------

// ctrlMockContainer tracks the state of a container managed by the mock runtime.
type ctrlMockContainer struct {
	id      string
	cfg     runtime.CreateConfig
	running bool
}

// ctrlMockRuntime implements runtime.Runtime entirely in memory.
type ctrlMockRuntime struct {
	mu         sync.Mutex
	containers map[string]*ctrlMockContainer
	networks   map[string]bool // name -> internal flag
	nextID     int

	// Optional hooks – when non-nil they override default behavior so tests
	// can inject errors at specific points.
	createErr  error
	startErr   error
	stopErr    error
	destroyErr error
	execResult *runtime.ExecResult
	execErr    error
}

func newCtrlMockRuntime() *ctrlMockRuntime {
	return &ctrlMockRuntime{
		containers: make(map[string]*ctrlMockContainer),
	}
}

func (m *ctrlMockRuntime) Name() string { return "mock" }

func (m *ctrlMockRuntime) Create(_ context.Context, cfg *runtime.CreateConfig) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.createErr != nil {
		return "", m.createErr
	}
	m.nextID++
	id := fmt.Sprintf("mock-%d", m.nextID)
	m.containers[id] = &ctrlMockContainer{id: id, cfg: *cfg, running: false}
	return id, nil
}

func (m *ctrlMockRuntime) Start(_ context.Context, id string) error {
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

func (m *ctrlMockRuntime) Stop(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopErr != nil {
		return m.stopErr
	}
	c, ok := m.containers[id]
	if !ok {
		return fmt.Errorf("container %q not found", id)
	}
	c.running = false
	return nil
}

func (m *ctrlMockRuntime) Destroy(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.destroyErr != nil {
		return m.destroyErr
	}
	if _, ok := m.containers[id]; !ok {
		return fmt.Errorf("container %q not found", id)
	}
	delete(m.containers, id)
	return nil
}

func (m *ctrlMockRuntime) Exec(_ context.Context, id string, _ *runtime.ExecConfig) (runtime.ExecResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.execErr != nil {
		return runtime.ExecResult{ExitCode: -1}, m.execErr
	}
	c, ok := m.containers[id]
	if !ok {
		return runtime.ExecResult{ExitCode: -1}, fmt.Errorf("container %q not found", id)
	}
	if !c.running {
		return runtime.ExecResult{ExitCode: -1}, fmt.Errorf("container %q is not running", id)
	}
	if m.execResult != nil {
		return *m.execResult, nil
	}
	return runtime.ExecResult{ExitCode: 0}, nil
}

func (m *ctrlMockRuntime) Info(_ context.Context, id string) (runtime.Info, error) {
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
		IP:     "172.17.0.2",
		Labels: c.cfg.Labels,
	}, nil
}

func (m *ctrlMockRuntime) Stats(_ context.Context, id string) (runtime.Stats, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.containers[id]; !ok {
		return runtime.Stats{}, fmt.Errorf("container %q not found", id)
	}
	return runtime.Stats{
		CPUUsage:    0.5,
		MemoryUsage: 1024 * 1024 * 100,
		MemoryLimit: 1024 * 1024 * 2048,
	}, nil
}

func (m *ctrlMockRuntime) List(_ context.Context) ([]runtime.Info, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var infos []runtime.Info
	for _, c := range m.containers {
		st := "stopped"
		if c.running {
			st = "running"
		}
		infos = append(infos, runtime.Info{
			ID:    c.id,
			Name:  c.cfg.Name,
			Image: c.cfg.Image,
			State: st,
		})
	}
	return infos, nil
}

func (m *ctrlMockRuntime) Snapshot(_ context.Context, id, tag string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.containers[id]; !ok {
		return "", fmt.Errorf("container %q not found", id)
	}
	return fmt.Sprintf("sha256:snap-%s-%s", id, tag), nil
}

func (m *ctrlMockRuntime) Restore(_ context.Context, snapshotID string, cfg *runtime.CreateConfig) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.createErr != nil {
		return "", m.createErr
	}
	m.nextID++
	id := fmt.Sprintf("mock-%d", m.nextID)
	cfg.Image = snapshotID
	m.containers[id] = &ctrlMockContainer{id: id, cfg: *cfg, running: false}
	return id, nil
}

func (m *ctrlMockRuntime) ListSnapshots(_ context.Context, _ string) ([]runtime.SnapshotInfo, error) {
	return nil, nil
}

func (m *ctrlMockRuntime) DeleteSnapshot(_ context.Context, _ string) error {
	return nil
}

func (m *ctrlMockRuntime) CreateNetwork(_ context.Context, name string, internal bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.networks == nil {
		m.networks = make(map[string]bool)
	}
	m.networks[name] = internal
	return nil
}

func (m *ctrlMockRuntime) DeleteNetwork(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.networks != nil {
		delete(m.networks, name)
	}
	return nil
}

func (m *ctrlMockRuntime) CopyToContainer(_ context.Context, _ string, _ string, _ io.Reader) error {
	return nil
}

func (m *ctrlMockRuntime) CopyFromContainer(_ context.Context, _ string, _ string) (io.ReadCloser, error) {
	return nil, nil
}

func (m *ctrlMockRuntime) Pause(_ context.Context, _ string) error   { return nil }
func (m *ctrlMockRuntime) Unpause(_ context.Context, _ string) error { return nil }
func (m *ctrlMockRuntime) UpdateResources(_ context.Context, _ string, _ runtime.ResourceUpdate) error {
	return nil
}
func (m *ctrlMockRuntime) HostInfo(_ context.Context) (runtime.HostResources, error) {
	return runtime.HostResources{TotalCPUs: 4, TotalMemory: 16 * 1024 * 1024 * 1024}, nil
}

// containerCount returns the number of containers tracked by the mock.
func (m *ctrlMockRuntime) containerCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.containers)
}

// isRunning checks whether the container with the given id is running.
func (m *ctrlMockRuntime) isRunning(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.containers[id]
	return ok && c.running
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// writeTempBlueprint writes the sample blueprint YAML into a temp file and
// returns its path. The caller should defer os.Remove(path).
func writeTempBlueprint(t *testing.T) string {
	t.Helper()
	content := `apiVersion: smx/v1alpha1
kind: Blueprint
metadata:
  name: python-dev
  version: "1.0.0"
spec:
  base: python:3.12-slim
  runtime: docker
  resources:
    cpu: "2"
    memory: 2Gi
    disk: 10Gi
  setup:
    - run: pip install poetry ruff mypy
  toolchains:
    - name: python-lsp
      image: smx-toolchains/pylsp:latest
  workspace:
    mountPath: /workspace
  network:
    expose: [8000]
`
	tmp, err := os.CreateTemp(t.TempDir(), "blueprint-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tmp.WriteString(content); err != nil {
		t.Fatal(err)
	}
	tmp.Close()
	return tmp.Name()
}

// blueprintPath tries to locate the real blueprints/python-dev.yaml file.
// Falls back to writing a temp file if the real file cannot be found.
func blueprintPath(t *testing.T) string {
	t.Helper()
	// Walk up from the working directory to find the repo root.
	wd, err := os.Getwd()
	if err != nil {
		return writeTempBlueprint(t)
	}
	// The test runs from internal/controller, so walk up twice to reach the repo root.
	candidates := []string{
		filepath.Join(wd, "..", "..", "blueprints", "python-dev.yaml"),
		filepath.Join(wd, "blueprints", "python-dev.yaml"),
	}
	for _, c := range candidates {
		abs, _ := filepath.Abs(c)
		if _, err := os.Stat(abs); err == nil {
			return abs
		}
	}
	return writeTempBlueprint(t)
}

// newTestController returns a Controller backed by a mock runtime and an
// in-memory store, along with the mock runtime for inspection.
func newTestController(t *testing.T) (*Controller, *ctrlMockRuntime) {
	t.Helper()
	rt := newCtrlMockRuntime()
	store := state.NewMemoryStore()
	return New(rt, store, nil, nil), rt
}

// createTestSandbox is a convenience that creates a sandbox through the
// controller and fails the test on error.
func createTestSandbox(t *testing.T, ctrl *Controller, name string) *v1alpha1.Sandbox {
	t.Helper()
	bp := blueprintPath(t)
	sb, err := ctrl.Create(context.Background(), CreateOptions{
		Name:          name,
		BlueprintPath: bp,
	})
	if err != nil {
		t.Fatalf("Create(%q): %v", name, err)
	}
	return sb
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestCreateSandbox(t *testing.T) {
	ctrl, rt := newTestController(t)
	sb := createTestSandbox(t, ctrl, "my-sandbox")

	if sb.Metadata.Name != "my-sandbox" {
		t.Fatalf("expected name my-sandbox, got %q", sb.Metadata.Name)
	}
	if sb.Status.State != v1alpha1.SandboxStateRunning {
		t.Fatalf("expected state Running, got %s", sb.Status.State)
	}
	if sb.Status.RuntimeID == "" {
		t.Fatal("expected RuntimeID to be set")
	}
	if sb.Status.IP != "172.17.0.2" {
		t.Fatalf("expected IP 172.17.0.2, got %q", sb.Status.IP)
	}
	if sb.Status.StartedAt == nil {
		t.Fatal("expected StartedAt to be set")
	}
	if sb.Spec.BlueprintRef != "python-dev" {
		t.Fatalf("expected blueprintRef python-dev, got %q", sb.Spec.BlueprintRef)
	}
	if sb.Spec.Resources.CPU != "2" {
		t.Fatalf("expected CPU 2, got %q", sb.Spec.Resources.CPU)
	}
	if sb.Spec.Resources.Memory != "2Gi" {
		t.Fatalf("expected Memory 2Gi, got %q", sb.Spec.Resources.Memory)
	}
	if sb.Metadata.Labels["blueprint"] != "python-dev" {
		t.Fatalf("expected label blueprint=python-dev, got %v", sb.Metadata.Labels)
	}

	// The mock runtime should have exactly one running container.
	if rt.containerCount() != 1 {
		t.Fatalf("expected 1 container, got %d", rt.containerCount())
	}
	if !rt.isRunning(sb.Status.RuntimeID) {
		t.Fatal("expected container to be running")
	}
}

func TestCreateWithWorkspace(t *testing.T) {
	ctrl, _ := newTestController(t)
	bp := blueprintPath(t)
	sb, err := ctrl.Create(context.Background(), CreateOptions{
		Name:          "ws-sandbox",
		BlueprintPath: bp,
		WorkspaceDir:  "/home/user/project",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if sb.Spec.Workspace.MountPath != "/workspace" {
		t.Fatalf("expected mount /workspace, got %q", sb.Spec.Workspace.MountPath)
	}
	if sb.Spec.Workspace.Source != "/home/user/project" {
		t.Fatalf("expected source /home/user/project, got %q", sb.Spec.Workspace.Source)
	}
}

func TestCreateDuplicateName(t *testing.T) {
	ctrl, _ := newTestController(t)
	createTestSandbox(t, ctrl, "dup")

	bp := blueprintPath(t)
	_, err := ctrl.Create(context.Background(), CreateOptions{
		Name:          "dup",
		BlueprintPath: bp,
	})
	if err == nil {
		t.Fatal("expected error for duplicate name")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected 'already exists' error, got: %v", err)
	}
}

func TestCreateInvalidBlueprint(t *testing.T) {
	ctrl, _ := newTestController(t)
	_, err := ctrl.Create(context.Background(), CreateOptions{
		Name:          "bad",
		BlueprintPath: "/nonexistent/blueprint.yaml",
	})
	if err == nil {
		t.Fatal("expected error for invalid blueprint path")
	}
	if !strings.Contains(err.Error(), "invalid blueprint") {
		t.Fatalf("expected 'invalid blueprint' error, got: %v", err)
	}
}

func TestCreateRuntimeError(t *testing.T) {
	rt := newCtrlMockRuntime()
	rt.createErr = fmt.Errorf("docker daemon unavailable")
	store := state.NewMemoryStore()
	ctrl := New(rt, store, nil, nil)

	bp := blueprintPath(t)
	_, err := ctrl.Create(context.Background(), CreateOptions{
		Name:          "fail-create",
		BlueprintPath: bp,
	})
	if err == nil {
		t.Fatal("expected error from runtime Create failure")
	}
	if !strings.Contains(err.Error(), "create runtime") {
		t.Fatalf("expected 'create runtime' error, got: %v", err)
	}

	// The sandbox should be saved in Error state.
	sb, storeErr := store.Get("fail-create")
	if storeErr != nil {
		t.Fatalf("expected sandbox to be saved in store, got: %v", storeErr)
	}
	if sb.Status.State != v1alpha1.SandboxStateError {
		t.Fatalf("expected Error state, got %s", sb.Status.State)
	}
	if sb.Status.Message != "docker daemon unavailable" {
		t.Fatalf("expected error message in status, got %q", sb.Status.Message)
	}
}

func TestCreateStartError(t *testing.T) {
	rt := newCtrlMockRuntime()
	rt.startErr = fmt.Errorf("out of memory")
	store := state.NewMemoryStore()
	ctrl := New(rt, store, nil, nil)

	bp := blueprintPath(t)
	_, err := ctrl.Create(context.Background(), CreateOptions{
		Name:          "fail-start",
		BlueprintPath: bp,
	})
	if err == nil {
		t.Fatal("expected error from runtime Start failure")
	}
	if !strings.Contains(err.Error(), "start runtime") {
		t.Fatalf("expected 'start runtime' error, got: %v", err)
	}

	sb, _ := store.Get("fail-start")
	if sb.Status.State != v1alpha1.SandboxStateError {
		t.Fatalf("expected Error state, got %s", sb.Status.State)
	}
}

func TestFullLifecycle(t *testing.T) {
	ctrl, rt := newTestController(t)

	// Step 1: Create (starts automatically).
	sb := createTestSandbox(t, ctrl, "lifecycle")
	runtimeID := sb.Status.RuntimeID

	if sb.Status.State != v1alpha1.SandboxStateRunning {
		t.Fatalf("after create: expected Running, got %s", sb.Status.State)
	}

	// Step 2: Stop.
	if err := ctrl.Stop(context.Background(), "lifecycle"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	sb, _ = ctrl.Get("lifecycle")
	if sb.Status.State != v1alpha1.SandboxStateStopped {
		t.Fatalf("after stop: expected Stopped, got %s", sb.Status.State)
	}
	if sb.Status.StoppedAt == nil {
		t.Fatal("expected StoppedAt to be set after Stop")
	}
	if rt.isRunning(runtimeID) {
		t.Fatal("expected container to be stopped in runtime")
	}

	// Step 3: Start (restart).
	if err := ctrl.Start(context.Background(), "lifecycle"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	sb, _ = ctrl.Get("lifecycle")
	if sb.Status.State != v1alpha1.SandboxStateRunning {
		t.Fatalf("after restart: expected Running, got %s", sb.Status.State)
	}
	if sb.Status.StoppedAt != nil {
		t.Fatal("expected StoppedAt to be nil after Start")
	}
	if !rt.isRunning(runtimeID) {
		t.Fatal("expected container to be running in runtime after Start")
	}

	// Step 4: Destroy.
	if err := ctrl.Destroy(context.Background(), "lifecycle"); err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if _, err := ctrl.Get("lifecycle"); err == nil {
		t.Fatal("expected Get to fail after Destroy")
	}
	if rt.containerCount() != 0 {
		t.Fatalf("expected 0 containers after Destroy, got %d", rt.containerCount())
	}
}

func TestStopNonRunning(t *testing.T) {
	ctrl, _ := newTestController(t)
	createTestSandbox(t, ctrl, "stop-me")

	// Stop it first so it's in Stopped state.
	if err := ctrl.Stop(context.Background(), "stop-me"); err != nil {
		t.Fatalf("first Stop: %v", err)
	}

	// Try to stop again — should fail.
	err := ctrl.Stop(context.Background(), "stop-me")
	if err == nil {
		t.Fatal("expected error stopping a non-running sandbox")
	}
	if !strings.Contains(err.Error(), "not running") {
		t.Fatalf("expected 'not running' error, got: %v", err)
	}
}

func TestStopNonExistent(t *testing.T) {
	ctrl, _ := newTestController(t)
	err := ctrl.Stop(context.Background(), "ghost")
	if err == nil {
		t.Fatal("expected error stopping non-existent sandbox")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found' error, got: %v", err)
	}
}

func TestStartNonStopped(t *testing.T) {
	ctrl, _ := newTestController(t)
	createTestSandbox(t, ctrl, "running-one")

	// It's Running; calling Start should fail because it's not Stopped.
	err := ctrl.Start(context.Background(), "running-one")
	if err == nil {
		t.Fatal("expected error starting a running sandbox")
	}
	if !strings.Contains(err.Error(), "not stopped") {
		t.Fatalf("expected 'not stopped' error, got: %v", err)
	}
}

func TestStartNonExistent(t *testing.T) {
	ctrl, _ := newTestController(t)
	err := ctrl.Start(context.Background(), "ghost")
	if err == nil {
		t.Fatal("expected error starting non-existent sandbox")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found' error, got: %v", err)
	}
}

func TestDestroyNonExistent(t *testing.T) {
	ctrl, _ := newTestController(t)
	err := ctrl.Destroy(context.Background(), "ghost")
	if err == nil {
		t.Fatal("expected error destroying non-existent sandbox")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found' error, got: %v", err)
	}
}

func TestDestroyStoppedSandbox(t *testing.T) {
	ctrl, rt := newTestController(t)
	createTestSandbox(t, ctrl, "to-destroy")

	if err := ctrl.Stop(context.Background(), "to-destroy"); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	if err := ctrl.Destroy(context.Background(), "to-destroy"); err != nil {
		t.Fatalf("Destroy stopped sandbox: %v", err)
	}

	if _, err := ctrl.Get("to-destroy"); err == nil {
		t.Fatal("expected Get to fail after Destroy")
	}
	if rt.containerCount() != 0 {
		t.Fatalf("expected 0 containers, got %d", rt.containerCount())
	}
}

func TestExecOnRunning(t *testing.T) {
	ctrl, _ := newTestController(t)
	createTestSandbox(t, ctrl, "exec-box")

	result, err := ctrl.Exec(context.Background(), "exec-box", &runtime.ExecConfig{
		Cmd: []string{"python", "-c", "print('hello')"},
	})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", result.ExitCode)
	}
}

func TestExecOnStoppedSandbox(t *testing.T) {
	ctrl, _ := newTestController(t)
	createTestSandbox(t, ctrl, "exec-stopped")

	if err := ctrl.Stop(context.Background(), "exec-stopped"); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	_, err := ctrl.Exec(context.Background(), "exec-stopped", &runtime.ExecConfig{
		Cmd: []string{"echo", "test"},
	})
	if err == nil {
		t.Fatal("expected error exec-ing on stopped sandbox")
	}
	if !strings.Contains(err.Error(), "not running") {
		t.Fatalf("expected 'not running' error, got: %v", err)
	}
}

func TestExecOnNonExistent(t *testing.T) {
	ctrl, _ := newTestController(t)
	_, err := ctrl.Exec(context.Background(), "ghost", &runtime.ExecConfig{
		Cmd: []string{"echo", "hello"},
	})
	if err == nil {
		t.Fatal("expected error exec-ing on non-existent sandbox")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found' error, got: %v", err)
	}
}

func TestExecRuntimeError(t *testing.T) {
	rt := newCtrlMockRuntime()
	store := state.NewMemoryStore()
	ctrl := New(rt, store, nil, nil)

	createTestSandbox(t, ctrl, "exec-err")

	rt.execErr = fmt.Errorf("execution timed out")
	_, err := ctrl.Exec(context.Background(), "exec-err", &runtime.ExecConfig{
		Cmd: []string{"sleep", "infinity"},
	})
	if err == nil {
		t.Fatal("expected error from runtime Exec failure")
	}
	if !strings.Contains(err.Error(), "execution timed out") {
		t.Fatalf("expected 'execution timed out' error, got: %v", err)
	}
}

func TestGetSandbox(t *testing.T) {
	ctrl, _ := newTestController(t)
	createTestSandbox(t, ctrl, "get-me")

	sb, err := ctrl.Get("get-me")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if sb.Metadata.Name != "get-me" {
		t.Fatalf("expected name get-me, got %q", sb.Metadata.Name)
	}
}

func TestGetNonExistent(t *testing.T) {
	ctrl, _ := newTestController(t)
	_, err := ctrl.Get("ghost")
	if err == nil {
		t.Fatal("expected error getting non-existent sandbox")
	}
}

func TestListEmpty(t *testing.T) {
	ctrl, _ := newTestController(t)
	list, err := ctrl.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected 0 sandboxes, got %d", len(list))
	}
}

func TestListMultiple(t *testing.T) {
	ctrl, _ := newTestController(t)
	createTestSandbox(t, ctrl, "alpha")
	createTestSandbox(t, ctrl, "beta")
	createTestSandbox(t, ctrl, "gamma")

	list, err := ctrl.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 sandboxes, got %d", len(list))
	}

	names := make(map[string]bool)
	for _, sb := range list {
		names[sb.Metadata.Name] = true
	}
	for _, expected := range []string{"alpha", "beta", "gamma"} {
		if !names[expected] {
			t.Fatalf("expected sandbox %q in list, got %v", expected, names)
		}
	}
}

func TestListAfterDestroyReflectsRemoval(t *testing.T) {
	ctrl, _ := newTestController(t)
	createTestSandbox(t, ctrl, "a")
	createTestSandbox(t, ctrl, "b")

	if err := ctrl.Destroy(context.Background(), "a"); err != nil {
		t.Fatalf("Destroy: %v", err)
	}

	list, err := ctrl.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 sandbox after destroy, got %d", len(list))
	}
	if list[0].Metadata.Name != "b" {
		t.Fatalf("expected remaining sandbox to be 'b', got %q", list[0].Metadata.Name)
	}
}

func TestTypeMeta(t *testing.T) {
	ctrl, _ := newTestController(t)
	sb := createTestSandbox(t, ctrl, "meta-check")

	if sb.APIVersion != "smx/v1alpha1" {
		t.Fatalf("expected apiVersion smx/v1alpha1, got %q", sb.APIVersion)
	}
	if sb.Kind != "Sandbox" {
		t.Fatalf("expected kind Sandbox, got %q", sb.Kind)
	}
}

func TestCreateSetsCreatedAtAndUpdatedAt(t *testing.T) {
	ctrl, _ := newTestController(t)
	sb := createTestSandbox(t, ctrl, "timestamps")

	if sb.Metadata.CreatedAt.IsZero() {
		t.Fatal("expected CreatedAt to be set")
	}
	if sb.Metadata.UpdatedAt.IsZero() {
		t.Fatal("expected UpdatedAt to be set")
	}
}

func TestStopUpdatesTimestamps(t *testing.T) {
	ctrl, _ := newTestController(t)
	createTestSandbox(t, ctrl, "ts-stop")

	if err := ctrl.Stop(context.Background(), "ts-stop"); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	sb, _ := ctrl.Get("ts-stop")
	if sb.Status.StoppedAt == nil {
		t.Fatal("expected StoppedAt to be set after Stop")
	}
	if sb.Metadata.UpdatedAt.Before(sb.Metadata.CreatedAt) {
		t.Fatal("expected UpdatedAt >= CreatedAt after Stop")
	}
}

func TestStartClearsStoppedAt(t *testing.T) {
	ctrl, _ := newTestController(t)
	createTestSandbox(t, ctrl, "ts-start")

	ctrl.Stop(context.Background(), "ts-start")
	ctrl.Start(context.Background(), "ts-start")

	sb, _ := ctrl.Get("ts-start")
	if sb.Status.StoppedAt != nil {
		t.Fatal("expected StoppedAt to be nil after Start")
	}
	if sb.Status.StartedAt == nil {
		t.Fatal("expected StartedAt to be set after Start")
	}
}

func TestRealBlueprintFile(t *testing.T) {
	// This test verifies that the real blueprint file from the project root
	// can be parsed and used by the controller.
	wd, err := os.Getwd()
	if err != nil {
		t.Skip("cannot determine working directory")
	}
	realPath := filepath.Join(wd, "..", "..", "blueprints", "python-dev.yaml")
	abs, _ := filepath.Abs(realPath)
	if _, err := os.Stat(abs); err != nil {
		t.Skipf("real blueprint file not found at %s, skipping", abs)
	}

	rt := newCtrlMockRuntime()
	store := state.NewMemoryStore()
	ctrl := New(rt, store, nil, nil)

	sb, createErr := ctrl.Create(context.Background(), CreateOptions{
		Name:          "real-bp",
		BlueprintPath: abs,
	})
	if createErr != nil {
		t.Fatalf("Create with real blueprint: %v", createErr)
	}
	if sb.Spec.BlueprintRef != "python-dev" {
		t.Fatalf("expected blueprintRef python-dev, got %q", sb.Spec.BlueprintRef)
	}
	if sb.Spec.Resources.CPU != "2" {
		t.Fatalf("expected CPU 2, got %q", sb.Spec.Resources.CPU)
	}
}

// ---------------------------------------------------------------------------
// In-memory SessionStore for testing
// ---------------------------------------------------------------------------

type memSessionStore struct {
	mu       sync.Mutex
	sessions map[string]*v1alpha1.Session
}

func newMemSessionStore() *memSessionStore {
	return &memSessionStore{sessions: make(map[string]*v1alpha1.Session)}
}

func (m *memSessionStore) GetSession(id string) (*v1alpha1.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return nil, fmt.Errorf("session %q not found", id)
	}
	cp := *s
	return &cp, nil
}

func (m *memSessionStore) ListSessions() ([]*v1alpha1.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*v1alpha1.Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		cp := *s
		result = append(result, &cp)
	}
	return result, nil
}

func (m *memSessionStore) ListSessionsBySandbox(sandbox string) ([]*v1alpha1.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []*v1alpha1.Session
	for _, s := range m.sessions {
		if s.Sandbox == sandbox {
			cp := *s
			result = append(result, &cp)
		}
	}
	return result, nil
}

func (m *memSessionStore) SaveSession(s *v1alpha1.Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *s
	m.sessions[s.Metadata.Name] = &cp
	return nil
}

func (m *memSessionStore) DeleteSession(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.sessions[id]; !ok {
		return fmt.Errorf("session %q not found", id)
	}
	delete(m.sessions, id)
	return nil
}

// ---------------------------------------------------------------------------
// Helper: controller with all stores
// ---------------------------------------------------------------------------

// newTestControllerWithStores returns a Controller backed by a mock runtime,
// in-memory sandbox store, in-memory session store, and in-memory matrix store.
func newTestControllerWithStores(t *testing.T) (*Controller, *ctrlMockRuntime) {
	t.Helper()
	rt := newCtrlMockRuntime()
	store := state.NewMemoryStore()
	sessions := newMemSessionStore()
	matrices := state.NewMemoryMatrixStore()
	return New(rt, store, sessions, matrices), rt
}

// ---------------------------------------------------------------------------
// Session Tests
// ---------------------------------------------------------------------------

func TestStartSession(t *testing.T) {
	ctrl, _ := newTestControllerWithStores(t)
	createTestSandbox(t, ctrl, "sess-box")

	session, err := ctrl.StartSession(context.Background(), "sess-box")
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	if session.Sandbox != "sess-box" {
		t.Fatalf("expected sandbox sess-box, got %q", session.Sandbox)
	}
	if session.State != v1alpha1.SessionStateActive {
		t.Fatalf("expected state Active, got %s", session.State)
	}
	if session.StartedAt == nil {
		t.Fatal("expected StartedAt to be set")
	}
	if session.ExecCount != 0 {
		t.Fatalf("expected ExecCount 0, got %d", session.ExecCount)
	}
	if session.Metadata.Name == "" {
		t.Fatal("expected session ID to be set")
	}
	if session.APIVersion != "smx/v1alpha1" {
		t.Fatalf("expected apiVersion smx/v1alpha1, got %q", session.APIVersion)
	}
	if session.Kind != "Session" {
		t.Fatalf("expected kind Session, got %q", session.Kind)
	}

	// Verify the session is retrievable.
	got, err := ctrl.GetSession(session.Metadata.Name)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.Metadata.Name != session.Metadata.Name {
		t.Fatalf("expected session ID %q, got %q", session.Metadata.Name, got.Metadata.Name)
	}
}

func TestStartSession_SandboxNotRunning(t *testing.T) {
	ctrl, _ := newTestControllerWithStores(t)
	createTestSandbox(t, ctrl, "stopped-box")

	if err := ctrl.Stop(context.Background(), "stopped-box"); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	_, err := ctrl.StartSession(context.Background(), "stopped-box")
	if err == nil {
		t.Fatal("expected error starting session on stopped sandbox")
	}
	if !strings.Contains(err.Error(), "not running") {
		t.Fatalf("expected 'not running' error, got: %v", err)
	}
}

func TestEndSession(t *testing.T) {
	ctrl, _ := newTestControllerWithStores(t)
	createTestSandbox(t, ctrl, "end-sess-box")

	session, err := ctrl.StartSession(context.Background(), "end-sess-box")
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	if err := ctrl.EndSession(context.Background(), session.Metadata.Name); err != nil {
		t.Fatalf("EndSession: %v", err)
	}

	got, err := ctrl.GetSession(session.Metadata.Name)
	if err != nil {
		t.Fatalf("GetSession after end: %v", err)
	}
	if got.State != v1alpha1.SessionStateCompleted {
		t.Fatalf("expected state Completed, got %s", got.State)
	}
	if got.EndedAt == nil {
		t.Fatal("expected EndedAt to be set")
	}
}

func TestEndSession_AlreadyCompleted(t *testing.T) {
	ctrl, _ := newTestControllerWithStores(t)
	createTestSandbox(t, ctrl, "done-sess-box")

	session, err := ctrl.StartSession(context.Background(), "done-sess-box")
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	if err := ctrl.EndSession(context.Background(), session.Metadata.Name); err != nil {
		t.Fatalf("EndSession: %v", err)
	}

	// Try ending again.
	err = ctrl.EndSession(context.Background(), session.Metadata.Name)
	if err == nil {
		t.Fatal("expected error ending already-completed session")
	}
	if !strings.Contains(err.Error(), "not active") {
		t.Fatalf("expected 'not active' error, got: %v", err)
	}
}

func TestListSessions(t *testing.T) {
	ctrl, _ := newTestControllerWithStores(t)
	createTestSandbox(t, ctrl, "list-sess-box1")
	createTestSandbox(t, ctrl, "list-sess-box2")

	s1, err := ctrl.StartSession(context.Background(), "list-sess-box1")
	if err != nil {
		t.Fatalf("StartSession 1: %v", err)
	}
	s2, err := ctrl.StartSession(context.Background(), "list-sess-box1")
	if err != nil {
		t.Fatalf("StartSession 2: %v", err)
	}
	s3, err := ctrl.StartSession(context.Background(), "list-sess-box2")
	if err != nil {
		t.Fatalf("StartSession 3: %v", err)
	}

	// List all sessions.
	all, err := ctrl.ListSessions("")
	if err != nil {
		t.Fatalf("ListSessions all: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 sessions, got %d", len(all))
	}

	// List sessions for list-sess-box1 only.
	filtered, err := ctrl.ListSessions("list-sess-box1")
	if err != nil {
		t.Fatalf("ListSessions filtered: %v", err)
	}
	if len(filtered) != 2 {
		t.Fatalf("expected 2 sessions for list-sess-box1, got %d", len(filtered))
	}

	// Verify IDs are present in the full list.
	ids := make(map[string]bool)
	for _, s := range all {
		ids[s.Metadata.Name] = true
	}
	for _, id := range []string{s1.Metadata.Name, s2.Metadata.Name, s3.Metadata.Name} {
		if !ids[id] {
			t.Fatalf("expected session %q in list", id)
		}
	}
}

func TestExecInSession(t *testing.T) {
	ctrl, _ := newTestControllerWithStores(t)
	createTestSandbox(t, ctrl, "exec-sess-box")

	session, err := ctrl.StartSession(context.Background(), "exec-sess-box")
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	result, err := ctrl.ExecInSession(context.Background(), session.Metadata.Name, &runtime.ExecConfig{
		Cmd: []string{"echo", "hello"},
	})
	if err != nil {
		t.Fatalf("ExecInSession: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", result.ExitCode)
	}

	// Verify exec count was incremented.
	got, err := ctrl.GetSession(session.Metadata.Name)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.ExecCount != 1 {
		t.Fatalf("expected ExecCount 1, got %d", got.ExecCount)
	}

	// Run another exec.
	_, err = ctrl.ExecInSession(context.Background(), session.Metadata.Name, &runtime.ExecConfig{
		Cmd: []string{"echo", "world"},
	})
	if err != nil {
		t.Fatalf("ExecInSession 2: %v", err)
	}

	got, err = ctrl.GetSession(session.Metadata.Name)
	if err != nil {
		t.Fatalf("GetSession 2: %v", err)
	}
	if got.ExecCount != 2 {
		t.Fatalf("expected ExecCount 2, got %d", got.ExecCount)
	}
}

// ---------------------------------------------------------------------------
// Matrix Tests
// ---------------------------------------------------------------------------

func TestCreateMatrix(t *testing.T) {
	ctrl, rt := newTestControllerWithStores(t)
	bp := writeTempBlueprint(t)

	members := []v1alpha1.MatrixMember{
		{Name: "worker-1", Blueprint: bp},
		{Name: "worker-2", Blueprint: bp},
	}

	mx, err := ctrl.CreateMatrix(context.Background(), "test-matrix", members)
	if err != nil {
		t.Fatalf("CreateMatrix: %v", err)
	}
	if mx.Metadata.Name != "test-matrix" {
		t.Fatalf("expected name test-matrix, got %q", mx.Metadata.Name)
	}
	if mx.State != v1alpha1.MatrixStateActive {
		t.Fatalf("expected state Active, got %s", mx.State)
	}
	if len(mx.Members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(mx.Members))
	}
	if mx.APIVersion != "smx/v1alpha1" {
		t.Fatalf("expected apiVersion smx/v1alpha1, got %q", mx.APIVersion)
	}
	if mx.Kind != "Matrix" {
		t.Fatalf("expected kind Matrix, got %q", mx.Kind)
	}

	// Verify member sandboxes were created.
	sb1, err := ctrl.Get("test-matrix-worker-1")
	if err != nil {
		t.Fatalf("Get member sandbox 1: %v", err)
	}
	if sb1.Status.State != v1alpha1.SandboxStateRunning {
		t.Fatalf("expected member 1 Running, got %s", sb1.Status.State)
	}

	sb2, err := ctrl.Get("test-matrix-worker-2")
	if err != nil {
		t.Fatalf("Get member sandbox 2: %v", err)
	}
	if sb2.Status.State != v1alpha1.SandboxStateRunning {
		t.Fatalf("expected member 2 Running, got %s", sb2.Status.State)
	}

	// Verify containers were created in the runtime.
	if rt.containerCount() != 2 {
		t.Fatalf("expected 2 containers, got %d", rt.containerCount())
	}
}

func TestStopMatrix(t *testing.T) {
	ctrl, rt := newTestControllerWithStores(t)
	bp := writeTempBlueprint(t)

	members := []v1alpha1.MatrixMember{
		{Name: "a", Blueprint: bp},
		{Name: "b", Blueprint: bp},
	}

	_, err := ctrl.CreateMatrix(context.Background(), "stop-mx", members)
	if err != nil {
		t.Fatalf("CreateMatrix: %v", err)
	}

	// Stop the matrix.
	if err := ctrl.StopMatrix(context.Background(), "stop-mx"); err != nil {
		t.Fatalf("StopMatrix: %v", err)
	}

	// Verify matrix state.
	mx, err := ctrl.GetMatrix("stop-mx")
	if err != nil {
		t.Fatalf("GetMatrix after stop: %v", err)
	}
	if mx.State != v1alpha1.MatrixStateStopped {
		t.Fatalf("expected matrix state Stopped, got %s", mx.State)
	}

	// Verify member sandboxes are stopped.
	sbA, _ := ctrl.Get("stop-mx-a")
	if sbA.Status.State != v1alpha1.SandboxStateStopped {
		t.Fatalf("expected member a Stopped, got %s", sbA.Status.State)
	}
	sbB, _ := ctrl.Get("stop-mx-b")
	if sbB.Status.State != v1alpha1.SandboxStateStopped {
		t.Fatalf("expected member b Stopped, got %s", sbB.Status.State)
	}

	// Verify containers are stopped in runtime.
	if rt.isRunning(sbA.Status.RuntimeID) {
		t.Fatal("expected member a container to be stopped")
	}
	if rt.isRunning(sbB.Status.RuntimeID) {
		t.Fatal("expected member b container to be stopped")
	}
}

func TestStartMatrix(t *testing.T) {
	ctrl, rt := newTestControllerWithStores(t)
	bp := writeTempBlueprint(t)

	members := []v1alpha1.MatrixMember{
		{Name: "x", Blueprint: bp},
		{Name: "y", Blueprint: bp},
	}

	_, err := ctrl.CreateMatrix(context.Background(), "start-mx", members)
	if err != nil {
		t.Fatalf("CreateMatrix: %v", err)
	}

	// Stop first so we can restart.
	if err := ctrl.StopMatrix(context.Background(), "start-mx"); err != nil {
		t.Fatalf("StopMatrix: %v", err)
	}

	// Start the matrix.
	if err := ctrl.StartMatrix(context.Background(), "start-mx"); err != nil {
		t.Fatalf("StartMatrix: %v", err)
	}

	// Verify matrix state.
	mx, err := ctrl.GetMatrix("start-mx")
	if err != nil {
		t.Fatalf("GetMatrix after start: %v", err)
	}
	if mx.State != v1alpha1.MatrixStateActive {
		t.Fatalf("expected matrix state Active, got %s", mx.State)
	}

	// Verify member sandboxes are running.
	sbX, _ := ctrl.Get("start-mx-x")
	if sbX.Status.State != v1alpha1.SandboxStateRunning {
		t.Fatalf("expected member x Running, got %s", sbX.Status.State)
	}
	sbY, _ := ctrl.Get("start-mx-y")
	if sbY.Status.State != v1alpha1.SandboxStateRunning {
		t.Fatalf("expected member y Running, got %s", sbY.Status.State)
	}

	// Verify containers are running in runtime.
	if !rt.isRunning(sbX.Status.RuntimeID) {
		t.Fatal("expected member x container to be running")
	}
	if !rt.isRunning(sbY.Status.RuntimeID) {
		t.Fatal("expected member y container to be running")
	}
}

func TestDestroyMatrix(t *testing.T) {
	ctrl, rt := newTestControllerWithStores(t)
	bp := writeTempBlueprint(t)

	members := []v1alpha1.MatrixMember{
		{Name: "p", Blueprint: bp},
		{Name: "q", Blueprint: bp},
	}

	_, err := ctrl.CreateMatrix(context.Background(), "destroy-mx", members)
	if err != nil {
		t.Fatalf("CreateMatrix: %v", err)
	}
	if rt.containerCount() != 2 {
		t.Fatalf("expected 2 containers before destroy, got %d", rt.containerCount())
	}

	// Destroy the matrix.
	if err := ctrl.DestroyMatrix(context.Background(), "destroy-mx"); err != nil {
		t.Fatalf("DestroyMatrix: %v", err)
	}

	// Verify matrix is gone.
	_, err = ctrl.GetMatrix("destroy-mx")
	if err == nil {
		t.Fatal("expected error getting destroyed matrix")
	}

	// Verify member sandboxes are gone.
	if _, err := ctrl.Get("destroy-mx-p"); err == nil {
		t.Fatal("expected member p sandbox to be destroyed")
	}
	if _, err := ctrl.Get("destroy-mx-q"); err == nil {
		t.Fatal("expected member q sandbox to be destroyed")
	}

	// Verify containers are removed from runtime.
	if rt.containerCount() != 0 {
		t.Fatalf("expected 0 containers after destroy, got %d", rt.containerCount())
	}
}

func TestGetMatrix(t *testing.T) {
	ctrl, _ := newTestControllerWithStores(t)
	bp := writeTempBlueprint(t)

	members := []v1alpha1.MatrixMember{
		{Name: "solo", Blueprint: bp},
	}

	created, err := ctrl.CreateMatrix(context.Background(), "get-mx", members)
	if err != nil {
		t.Fatalf("CreateMatrix: %v", err)
	}

	got, err := ctrl.GetMatrix("get-mx")
	if err != nil {
		t.Fatalf("GetMatrix: %v", err)
	}
	if got.Metadata.Name != created.Metadata.Name {
		t.Fatalf("expected name %q, got %q", created.Metadata.Name, got.Metadata.Name)
	}
	if len(got.Members) != 1 {
		t.Fatalf("expected 1 member, got %d", len(got.Members))
	}
	if got.Members[0].Name != "solo" {
		t.Fatalf("expected member name solo, got %q", got.Members[0].Name)
	}
}

func TestListMatrices(t *testing.T) {
	ctrl, _ := newTestControllerWithStores(t)
	bp := writeTempBlueprint(t)

	members := []v1alpha1.MatrixMember{
		{Name: "m1", Blueprint: bp},
	}

	_, err := ctrl.CreateMatrix(context.Background(), "list-mx-1", members)
	if err != nil {
		t.Fatalf("CreateMatrix 1: %v", err)
	}
	_, err = ctrl.CreateMatrix(context.Background(), "list-mx-2", members)
	if err != nil {
		t.Fatalf("CreateMatrix 2: %v", err)
	}
	_, err = ctrl.CreateMatrix(context.Background(), "list-mx-3", members)
	if err != nil {
		t.Fatalf("CreateMatrix 3: %v", err)
	}

	list, err := ctrl.ListMatrices()
	if err != nil {
		t.Fatalf("ListMatrices: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 matrices, got %d", len(list))
	}

	names := make(map[string]bool)
	for _, mx := range list {
		names[mx.Metadata.Name] = true
	}
	for _, expected := range []string{"list-mx-1", "list-mx-2", "list-mx-3"} {
		if !names[expected] {
			t.Fatalf("expected matrix %q in list, got %v", expected, names)
		}
	}
}

func TestCreateMatrix_DuplicateName(t *testing.T) {
	ctrl, _ := newTestControllerWithStores(t)
	bp := writeTempBlueprint(t)

	members := []v1alpha1.MatrixMember{
		{Name: "w", Blueprint: bp},
	}

	_, err := ctrl.CreateMatrix(context.Background(), "dup-mx", members)
	if err != nil {
		t.Fatalf("CreateMatrix: %v", err)
	}

	_, err = ctrl.CreateMatrix(context.Background(), "dup-mx", members)
	if err == nil {
		t.Fatal("expected error creating duplicate matrix")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected 'already exists' error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Snapshot / Restore / Stats / Runtime / ListSnapshots / DeleteSnapshot
// ---------------------------------------------------------------------------

func TestSnapshot(t *testing.T) {
	ctrl, _ := newTestController(t)
	createTestSandbox(t, ctrl, "snap-box")

	snapshotID, err := ctrl.Snapshot(context.Background(), "snap-box", "v1")
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if snapshotID == "" {
		t.Fatal("expected non-empty snapshot ID")
	}
	if !strings.Contains(snapshotID, "v1") {
		t.Errorf("expected snapshot ID to contain tag v1, got %q", snapshotID)
	}
}

func TestSnapshotNonExistent(t *testing.T) {
	ctrl, _ := newTestController(t)
	_, err := ctrl.Snapshot(context.Background(), "ghost", "v1")
	if err == nil {
		t.Fatal("expected error snapshotting non-existent sandbox")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found' error, got: %v", err)
	}
}

func TestSnapshotNoRuntimeID(t *testing.T) {
	// Manually insert a sandbox with no runtime ID.
	store := state.NewMemoryStore()
	rt := newCtrlMockRuntime()
	ctrl := New(rt, store, nil, nil)

	sb := &v1alpha1.Sandbox{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Sandbox"},
		Metadata: v1alpha1.ObjectMeta{Name: "no-rt"},
		Spec:     v1alpha1.SandboxSpec{BlueprintRef: "bp"},
		Status:   v1alpha1.SandboxStatus{State: v1alpha1.SandboxStateRunning, RuntimeID: ""},
	}
	if err := store.Save(sb); err != nil {
		t.Fatalf("save: %v", err)
	}

	_, err := ctrl.Snapshot(context.Background(), "no-rt", "v1")
	if err == nil {
		t.Fatal("expected error for sandbox with no runtime ID")
	}
	if !strings.Contains(err.Error(), "no runtime ID") {
		t.Fatalf("expected 'no runtime ID' error, got: %v", err)
	}
}

func TestRestore(t *testing.T) {
	ctrl, rt := newTestController(t)
	sb := createTestSandbox(t, ctrl, "src-box")

	// Create a snapshot first.
	snapshotID, err := ctrl.Snapshot(context.Background(), "src-box", "restore-v1")
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	// Restore from the snapshot.
	restored, err := ctrl.Restore(context.Background(), "src-box", snapshotID, "restored-box")
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if restored.Metadata.Name != "restored-box" {
		t.Fatalf("expected name restored-box, got %q", restored.Metadata.Name)
	}
	if restored.Status.State != v1alpha1.SandboxStateRunning {
		t.Fatalf("expected state Running, got %s", restored.Status.State)
	}
	if restored.Metadata.Labels["restored"] != "true" {
		t.Errorf("expected label restored=true, got %v", restored.Metadata.Labels)
	}
	if restored.Metadata.Labels["restored-from"] != "src-box" {
		t.Errorf("expected label restored-from=src-box, got %v", restored.Metadata.Labels)
	}
	if restored.Spec.BlueprintRef != sb.Spec.BlueprintRef {
		t.Errorf("expected blueprintRef %q, got %q", sb.Spec.BlueprintRef, restored.Spec.BlueprintRef)
	}

	// Should now have 2 containers (original + restored).
	if rt.containerCount() != 2 {
		t.Fatalf("expected 2 containers, got %d", rt.containerCount())
	}
}

func TestRestoreDuplicateName(t *testing.T) {
	ctrl, _ := newTestController(t)
	createTestSandbox(t, ctrl, "restore-dup")

	snapshotID, err := ctrl.Snapshot(context.Background(), "restore-dup", "v1")
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	// Try to restore with the same name as an existing sandbox.
	_, err = ctrl.Restore(context.Background(), "restore-dup", snapshotID, "restore-dup")
	if err == nil {
		t.Fatal("expected error for duplicate restore name")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected 'already exists' error, got: %v", err)
	}
}

func TestRestoreNonExistentSource(t *testing.T) {
	ctrl, _ := newTestController(t)
	_, err := ctrl.Restore(context.Background(), "ghost", "snap-id", "new-name")
	if err == nil {
		t.Fatal("expected error restoring from non-existent sandbox")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found' error, got: %v", err)
	}
}

func TestListSnapshots(t *testing.T) {
	ctrl, _ := newTestController(t)
	createTestSandbox(t, ctrl, "snaplist-box")

	snapshots, err := ctrl.ListSnapshots(context.Background(), "snaplist-box")
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	// Mock runtime returns nil/empty.
	if len(snapshots) != 0 {
		t.Errorf("expected 0 snapshots, got %d", len(snapshots))
	}
}

func TestListSnapshotsNonExistent(t *testing.T) {
	ctrl, _ := newTestController(t)
	_, err := ctrl.ListSnapshots(context.Background(), "ghost")
	if err == nil {
		t.Fatal("expected error for non-existent sandbox")
	}
}

func TestListSnapshotsNoRuntimeID(t *testing.T) {
	store := state.NewMemoryStore()
	rt := newCtrlMockRuntime()
	ctrl := New(rt, store, nil, nil)

	sb := &v1alpha1.Sandbox{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Sandbox"},
		Metadata: v1alpha1.ObjectMeta{Name: "no-rt-snap"},
		Spec:     v1alpha1.SandboxSpec{BlueprintRef: "bp"},
		Status:   v1alpha1.SandboxStatus{State: v1alpha1.SandboxStateRunning, RuntimeID: ""},
	}
	if err := store.Save(sb); err != nil {
		t.Fatalf("save: %v", err)
	}

	_, err := ctrl.ListSnapshots(context.Background(), "no-rt-snap")
	if err == nil {
		t.Fatal("expected error for sandbox with no runtime ID")
	}
}

func TestDeleteSnapshot(t *testing.T) {
	ctrl, _ := newTestController(t)
	err := ctrl.DeleteSnapshot(context.Background(), "any-snap-id")
	if err != nil {
		t.Fatalf("DeleteSnapshot: %v", err)
	}
}

func TestStats(t *testing.T) {
	ctrl, _ := newTestController(t)
	createTestSandbox(t, ctrl, "stats-box")

	stats, err := ctrl.Stats(context.Background(), "stats-box")
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.CPUUsage == 0 {
		t.Error("expected non-zero CPUUsage")
	}
	if stats.MemoryUsage == 0 {
		t.Error("expected non-zero MemoryUsage")
	}
}

func TestStatsNonExistent(t *testing.T) {
	ctrl, _ := newTestController(t)
	_, err := ctrl.Stats(context.Background(), "ghost")
	if err == nil {
		t.Fatal("expected error for non-existent sandbox")
	}
}

func TestStatsStoppedSandbox(t *testing.T) {
	ctrl, _ := newTestController(t)
	createTestSandbox(t, ctrl, "stats-stopped")

	if err := ctrl.Stop(context.Background(), "stats-stopped"); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	_, err := ctrl.Stats(context.Background(), "stats-stopped")
	if err == nil {
		t.Fatal("expected error for stopped sandbox stats")
	}
	if !strings.Contains(err.Error(), "not running") {
		t.Fatalf("expected 'not running' error, got: %v", err)
	}
}

func TestRuntimeAccessor(t *testing.T) {
	ctrl, rt := newTestController(t)
	got := ctrl.Runtime()
	if got != rt {
		t.Fatal("expected Runtime() to return the mock runtime")
	}
	if got.Name() != "mock" {
		t.Fatalf("expected runtime name mock, got %q", got.Name())
	}
}

// ---------------------------------------------------------------------------
// Session edge case tests
// ---------------------------------------------------------------------------

func TestSessionsNotConfigured(t *testing.T) {
	ctrl, _ := newTestController(t) // nil session store

	_, err := ctrl.StartSession(context.Background(), "any")
	if err == nil {
		t.Fatal("expected error when sessions not configured")
	}
	if !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("expected 'not configured' error, got: %v", err)
	}

	err = ctrl.EndSession(context.Background(), "any")
	if err == nil {
		t.Fatal("expected error when sessions not configured")
	}

	_, err = ctrl.GetSession("any")
	if err == nil {
		t.Fatal("expected error when sessions not configured")
	}

	_, err = ctrl.ListSessions("")
	if err == nil {
		t.Fatal("expected error when sessions not configured")
	}

	_, err = ctrl.ExecInSession(context.Background(), "any", &runtime.ExecConfig{Cmd: []string{"echo"}})
	if err == nil {
		t.Fatal("expected error when sessions not configured")
	}
}

func TestStartSession_SandboxNotFound(t *testing.T) {
	ctrl, _ := newTestControllerWithStores(t)
	_, err := ctrl.StartSession(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent sandbox")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found' error, got: %v", err)
	}
}

func TestExecInSession_NotActive(t *testing.T) {
	ctrl, _ := newTestControllerWithStores(t)
	createTestSandbox(t, ctrl, "exec-ended-sess")

	session, err := ctrl.StartSession(context.Background(), "exec-ended-sess")
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	// End the session.
	if err := ctrl.EndSession(context.Background(), session.Metadata.Name); err != nil {
		t.Fatalf("EndSession: %v", err)
	}

	// Try exec in ended session.
	_, err = ctrl.ExecInSession(context.Background(), session.Metadata.Name, &runtime.ExecConfig{
		Cmd: []string{"echo"},
	})
	if err == nil {
		t.Fatal("expected error exec in ended session")
	}
	if !strings.Contains(err.Error(), "not active") {
		t.Fatalf("expected 'not active' error, got: %v", err)
	}
}

func TestExecInSession_NotFound(t *testing.T) {
	ctrl, _ := newTestControllerWithStores(t)
	_, err := ctrl.ExecInSession(context.Background(), "nonexistent", &runtime.ExecConfig{
		Cmd: []string{"echo"},
	})
	if err == nil {
		t.Fatal("expected error for nonexistent session")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found' error, got: %v", err)
	}
}

func TestEndSession_NotFound(t *testing.T) {
	ctrl, _ := newTestControllerWithStores(t)
	err := ctrl.EndSession(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent session")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found' error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Matrix edge case tests
// ---------------------------------------------------------------------------

func TestMatrixNotConfigured(t *testing.T) {
	ctrl, _ := newTestController(t) // nil matrix store

	_, err := ctrl.CreateMatrix(context.Background(), "any", nil)
	if err == nil {
		t.Fatal("expected error when matrices not configured")
	}
	if !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("expected 'not configured' error, got: %v", err)
	}

	err = ctrl.StopMatrix(context.Background(), "any")
	if err == nil {
		t.Fatal("expected error")
	}

	err = ctrl.StartMatrix(context.Background(), "any")
	if err == nil {
		t.Fatal("expected error")
	}

	err = ctrl.DestroyMatrix(context.Background(), "any")
	if err == nil {
		t.Fatal("expected error")
	}

	_, err = ctrl.GetMatrix("any")
	if err == nil {
		t.Fatal("expected error")
	}

	_, err = ctrl.ListMatrices()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestStopMatrix_NotActive(t *testing.T) {
	ctrl, _ := newTestControllerWithStores(t)
	bp := writeTempBlueprint(t)

	members := []v1alpha1.MatrixMember{
		{Name: "w", Blueprint: bp},
	}
	_, err := ctrl.CreateMatrix(context.Background(), "stop-notactive", members)
	if err != nil {
		t.Fatalf("CreateMatrix: %v", err)
	}

	// Stop it first.
	if err := ctrl.StopMatrix(context.Background(), "stop-notactive"); err != nil {
		t.Fatalf("StopMatrix: %v", err)
	}

	// Try to stop again.
	err = ctrl.StopMatrix(context.Background(), "stop-notactive")
	if err == nil {
		t.Fatal("expected error stopping non-active matrix")
	}
	if !strings.Contains(err.Error(), "not active") {
		t.Fatalf("expected 'not active' error, got: %v", err)
	}
}

func TestStartMatrix_NotStopped(t *testing.T) {
	ctrl, _ := newTestControllerWithStores(t)
	bp := writeTempBlueprint(t)

	members := []v1alpha1.MatrixMember{
		{Name: "w", Blueprint: bp},
	}
	_, err := ctrl.CreateMatrix(context.Background(), "start-notactive", members)
	if err != nil {
		t.Fatalf("CreateMatrix: %v", err)
	}

	// It's Active, not Stopped. Starting should fail.
	err = ctrl.StartMatrix(context.Background(), "start-notactive")
	if err == nil {
		t.Fatal("expected error starting an active matrix")
	}
	if !strings.Contains(err.Error(), "not stopped") {
		t.Fatalf("expected 'not stopped' error, got: %v", err)
	}
}
