package controller

import (
	"context"
	"fmt"
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
	nextID     int

	// Optional hooks – when non-nil they override default behaviour so tests
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

func (m *ctrlMockRuntime) Create(_ context.Context, cfg runtime.CreateConfig) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.createErr != nil {
		return "", m.createErr
	}
	m.nextID++
	id := fmt.Sprintf("mock-%d", m.nextID)
	m.containers[id] = &ctrlMockContainer{id: id, cfg: cfg, running: false}
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

func (m *ctrlMockRuntime) Exec(_ context.Context, id string, _ runtime.ExecConfig) (runtime.ExecResult, error) {
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
		filepath.Join(wd, "../../blueprints/python-dev.yaml"),
		filepath.Join(wd, "blueprints/python-dev.yaml"),
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
	return New(rt, store), rt
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
	ctrl := New(rt, store)

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
	ctrl := New(rt, store)

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

	result, err := ctrl.Exec(context.Background(), "exec-box", runtime.ExecConfig{
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

	_, err := ctrl.Exec(context.Background(), "exec-stopped", runtime.ExecConfig{
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
	_, err := ctrl.Exec(context.Background(), "ghost", runtime.ExecConfig{
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
	ctrl := New(rt, store)

	createTestSandbox(t, ctrl, "exec-err")

	rt.execErr = fmt.Errorf("execution timed out")
	_, err := ctrl.Exec(context.Background(), "exec-err", runtime.ExecConfig{
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
	realPath := filepath.Join(wd, "../../blueprints/python-dev.yaml")
	abs, _ := filepath.Abs(realPath)
	if _, err := os.Stat(abs); err != nil {
		t.Skipf("real blueprint file not found at %s, skipping", abs)
	}

	rt := newCtrlMockRuntime()
	store := state.NewMemoryStore()
	ctrl := New(rt, store)

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
