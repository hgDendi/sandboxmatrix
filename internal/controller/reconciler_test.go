package controller

import (
	"context"
	"fmt"
	"testing"

	"github.com/hg-dendi/sandboxmatrix/internal/runtime"
	"github.com/hg-dendi/sandboxmatrix/internal/state"
	v1alpha1 "github.com/hg-dendi/sandboxmatrix/pkg/api/v1alpha1"
)

// ---------- mock runtime ----------

type mockRuntime struct {
	containers []runtime.Info
	listErr    error
}

func (m *mockRuntime) Name() string { return "mock" }

func (m *mockRuntime) Create(_ context.Context, _ runtime.CreateConfig) (string, error) {
	return "", fmt.Errorf("not implemented")
}

func (m *mockRuntime) Start(_ context.Context, _ string) error {
	return fmt.Errorf("not implemented")
}

func (m *mockRuntime) Stop(_ context.Context, _ string) error {
	return fmt.Errorf("not implemented")
}

func (m *mockRuntime) Destroy(_ context.Context, _ string) error {
	return fmt.Errorf("not implemented")
}

func (m *mockRuntime) Exec(_ context.Context, _ string, _ runtime.ExecConfig) (runtime.ExecResult, error) {
	return runtime.ExecResult{}, fmt.Errorf("not implemented")
}

func (m *mockRuntime) Info(_ context.Context, _ string) (runtime.Info, error) {
	return runtime.Info{}, fmt.Errorf("not implemented")
}

func (m *mockRuntime) Stats(_ context.Context, _ string) (runtime.Stats, error) {
	return runtime.Stats{}, fmt.Errorf("not implemented")
}

func (m *mockRuntime) List(_ context.Context) ([]runtime.Info, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.containers, nil
}

func (m *mockRuntime) Snapshot(_ context.Context, _ string, _ string) (string, error) {
	return "", nil
}

func (m *mockRuntime) Restore(_ context.Context, _ string, _ runtime.CreateConfig) (string, error) {
	return "", nil
}

func (m *mockRuntime) ListSnapshots(_ context.Context, _ string) ([]runtime.SnapshotInfo, error) {
	return nil, nil
}

func (m *mockRuntime) DeleteSnapshot(_ context.Context, _ string) error {
	return nil
}

// ---------- tests ----------

func TestReconcile_ImportsOrphanedContainers(t *testing.T) {
	rt := &mockRuntime{
		containers: []runtime.Info{
			{
				ID:    "abc123def456",
				Name:  "smx-dev",
				Image: "ubuntu:22.04",
				State: "running",
				IP:    "172.17.0.2",
				Labels: map[string]string{
					"sandboxmatrix/managed":   "true",
					"sandboxmatrix/sandbox":   "dev",
					"sandboxmatrix/blueprint": "go-dev",
				},
			},
		},
	}
	store := state.NewMemoryStore()

	ctrl := New(rt, store, nil, nil)
	if err := ctrl.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	sb, err := store.Get("dev")
	if err != nil {
		t.Fatalf("expected sandbox 'dev' in store, got error: %v", err)
	}

	if sb.Status.State != v1alpha1.SandboxStateRunning {
		t.Errorf("expected state Running, got %s", sb.Status.State)
	}
	if sb.Status.RuntimeID != "abc123def456" {
		t.Errorf("expected runtime ID abc123def456, got %s", sb.Status.RuntimeID)
	}
	if sb.Spec.BlueprintRef != "go-dev" {
		t.Errorf("expected blueprint ref go-dev, got %s", sb.Spec.BlueprintRef)
	}
	if sb.Status.IP != "172.17.0.2" {
		t.Errorf("expected IP 172.17.0.2, got %s", sb.Status.IP)
	}
	if sb.Metadata.Labels["reconciled"] != "true" {
		t.Error("expected reconciled label to be set")
	}
}

func TestReconcile_SkipsAlreadyKnownSandboxes(t *testing.T) {
	rt := &mockRuntime{
		containers: []runtime.Info{
			{
				ID:    "abc123def456",
				Name:  "smx-existing",
				Image: "ubuntu:22.04",
				State: "running",
				Labels: map[string]string{
					"sandboxmatrix/managed":   "true",
					"sandboxmatrix/sandbox":   "existing",
					"sandboxmatrix/blueprint": "bp",
				},
			},
		},
	}
	store := state.NewMemoryStore()

	// Pre-populate the store with a record for this sandbox.
	original := &v1alpha1.Sandbox{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Sandbox"},
		Metadata: v1alpha1.ObjectMeta{Name: "existing"},
		Spec:     v1alpha1.SandboxSpec{BlueprintRef: "original-bp"},
		Status:   v1alpha1.SandboxStatus{State: v1alpha1.SandboxStateStopped, RuntimeID: "old-id"},
	}
	if err := store.Save(original); err != nil {
		t.Fatalf("store.Save: %v", err)
	}

	ctrl := New(rt, store, nil, nil)
	if err := ctrl.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	sb, _ := store.Get("existing")
	// The original record must be unchanged.
	if sb.Spec.BlueprintRef != "original-bp" {
		t.Errorf("expected blueprint ref original-bp (unchanged), got %s", sb.Spec.BlueprintRef)
	}
	if sb.Status.RuntimeID != "old-id" {
		t.Errorf("expected runtime ID old-id (unchanged), got %s", sb.Status.RuntimeID)
	}
}

func TestReconcile_MapsExitedToStopped(t *testing.T) {
	rt := &mockRuntime{
		containers: []runtime.Info{
			{
				ID:    "def456abc789",
				Name:  "smx-stopped",
				Image: "node:18",
				State: "exited",
				Labels: map[string]string{
					"sandboxmatrix/managed":   "true",
					"sandboxmatrix/sandbox":   "stopped",
					"sandboxmatrix/blueprint": "node-bp",
				},
			},
		},
	}
	store := state.NewMemoryStore()

	ctrl := New(rt, store, nil, nil)
	if err := ctrl.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	sb, _ := store.Get("stopped")
	if sb.Status.State != v1alpha1.SandboxStateStopped {
		t.Errorf("expected state Stopped for exited container, got %s", sb.Status.State)
	}
}

func TestReconcile_MapsCreatedToPending(t *testing.T) {
	rt := &mockRuntime{
		containers: []runtime.Info{
			{
				ID:    "ccc111222333",
				Name:  "smx-pending",
				Image: "alpine:3",
				State: "created",
				Labels: map[string]string{
					"sandboxmatrix/managed":   "true",
					"sandboxmatrix/sandbox":   "pending",
					"sandboxmatrix/blueprint": "alpine-bp",
				},
			},
		},
	}
	store := state.NewMemoryStore()

	ctrl := New(rt, store, nil, nil)
	if err := ctrl.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	sb, _ := store.Get("pending")
	if sb.Status.State != v1alpha1.SandboxStatePending {
		t.Errorf("expected state Pending for created container, got %s", sb.Status.State)
	}
}

func TestReconcile_MapsUnknownStateToError(t *testing.T) {
	rt := &mockRuntime{
		containers: []runtime.Info{
			{
				ID:    "eee444555666",
				Name:  "smx-weird",
				Image: "ubuntu:22.04",
				State: "restarting",
				Labels: map[string]string{
					"sandboxmatrix/managed":   "true",
					"sandboxmatrix/sandbox":   "weird",
					"sandboxmatrix/blueprint": "bp",
				},
			},
		},
	}
	store := state.NewMemoryStore()

	ctrl := New(rt, store, nil, nil)
	if err := ctrl.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	sb, _ := store.Get("weird")
	if sb.Status.State != v1alpha1.SandboxStateError {
		t.Errorf("expected state Error for unknown container state, got %s", sb.Status.State)
	}
}

func TestReconcile_SkipsContainersWithoutSandboxLabel(t *testing.T) {
	rt := &mockRuntime{
		containers: []runtime.Info{
			{
				ID:    "fff777888999",
				Name:  "smx-unlabeled",
				Image: "ubuntu:22.04",
				State: "running",
				Labels: map[string]string{
					"sandboxmatrix/managed": "true",
					// Missing sandboxmatrix/sandbox label.
				},
			},
		},
	}
	store := state.NewMemoryStore()

	ctrl := New(rt, store, nil, nil)
	if err := ctrl.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	sandboxes, _ := store.List()
	if len(sandboxes) != 0 {
		t.Errorf("expected no sandboxes in store, got %d", len(sandboxes))
	}
}

func TestReconcile_RuntimeListError(t *testing.T) {
	rt := &mockRuntime{
		listErr: fmt.Errorf("docker daemon unreachable"),
	}
	store := state.NewMemoryStore()

	ctrl := New(rt, store, nil, nil)
	err := ctrl.Reconcile(context.Background())
	if err == nil {
		t.Fatal("expected error when runtime.List fails, got nil")
	}
}

func TestReconcile_MultipleContainers(t *testing.T) {
	rt := &mockRuntime{
		containers: []runtime.Info{
			{
				ID:    "aaa111222333",
				Name:  "smx-first",
				Image: "ubuntu:22.04",
				State: "running",
				Labels: map[string]string{
					"sandboxmatrix/managed":   "true",
					"sandboxmatrix/sandbox":   "first",
					"sandboxmatrix/blueprint": "bp-a",
				},
			},
			{
				ID:    "bbb444555666",
				Name:  "smx-second",
				Image: "node:18",
				State: "exited",
				Labels: map[string]string{
					"sandboxmatrix/managed":   "true",
					"sandboxmatrix/sandbox":   "second",
					"sandboxmatrix/blueprint": "bp-b",
				},
			},
		},
	}
	store := state.NewMemoryStore()

	ctrl := New(rt, store, nil, nil)
	if err := ctrl.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	sandboxes, _ := store.List()
	if len(sandboxes) != 2 {
		t.Fatalf("expected 2 sandboxes, got %d", len(sandboxes))
	}

	first, _ := store.Get("first")
	if first.Status.State != v1alpha1.SandboxStateRunning {
		t.Errorf("expected first=Running, got %s", first.Status.State)
	}
	second, _ := store.Get("second")
	if second.Status.State != v1alpha1.SandboxStateStopped {
		t.Errorf("expected second=Stopped, got %s", second.Status.State)
	}
}

func TestReconcile_EmptyRuntimeNoop(t *testing.T) {
	rt := &mockRuntime{containers: nil}
	store := state.NewMemoryStore()

	ctrl := New(rt, store, nil, nil)
	if err := ctrl.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	sandboxes, _ := store.List()
	if len(sandboxes) != 0 {
		t.Errorf("expected 0 sandboxes, got %d", len(sandboxes))
	}
}
