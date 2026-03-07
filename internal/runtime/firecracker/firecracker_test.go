package firecracker

import (
	"context"
	"errors"
	"testing"

	"github.com/hg-dendi/sandboxmatrix/internal/runtime"
)

// TestFirecrackerImplementsRuntime verifies at compile time that *Runtime satisfies runtime.Runtime.
var _ runtime.Runtime = (*Runtime)(nil)

func TestFirecrackerName(t *testing.T) {
	r := New()
	if got := r.Name(); got != "firecracker" {
		t.Fatalf("expected Name() = %q, got %q", "firecracker", got)
	}
}

func TestFirecrackerMethodsReturnNotImplemented(t *testing.T) {
	r := New()
	ctx := context.Background()

	_, err := r.Create(ctx, &runtime.CreateConfig{})
	if !errors.Is(err, ErrNotImplemented) {
		t.Errorf("Create: expected ErrNotImplemented, got %v", err)
	}

	if err := r.Start(ctx, "id"); !errors.Is(err, ErrNotImplemented) {
		t.Errorf("Start: expected ErrNotImplemented, got %v", err)
	}

	if err := r.Stop(ctx, "id"); !errors.Is(err, ErrNotImplemented) {
		t.Errorf("Stop: expected ErrNotImplemented, got %v", err)
	}

	if err := r.Destroy(ctx, "id"); !errors.Is(err, ErrNotImplemented) {
		t.Errorf("Destroy: expected ErrNotImplemented, got %v", err)
	}

	_, err = r.Exec(ctx, "id", &runtime.ExecConfig{})
	if !errors.Is(err, ErrNotImplemented) {
		t.Errorf("Exec: expected ErrNotImplemented, got %v", err)
	}

	_, err = r.Info(ctx, "id")
	if !errors.Is(err, ErrNotImplemented) {
		t.Errorf("Info: expected ErrNotImplemented, got %v", err)
	}

	_, err = r.Stats(ctx, "id")
	if !errors.Is(err, ErrNotImplemented) {
		t.Errorf("Stats: expected ErrNotImplemented, got %v", err)
	}

	_, err = r.List(ctx)
	if !errors.Is(err, ErrNotImplemented) {
		t.Errorf("List: expected ErrNotImplemented, got %v", err)
	}

	_, err = r.Snapshot(ctx, "id", "tag")
	if !errors.Is(err, ErrNotImplemented) {
		t.Errorf("Snapshot: expected ErrNotImplemented, got %v", err)
	}

	_, err = r.Restore(ctx, "snap-id", &runtime.CreateConfig{})
	if !errors.Is(err, ErrNotImplemented) {
		t.Errorf("Restore: expected ErrNotImplemented, got %v", err)
	}

	_, err = r.ListSnapshots(ctx, "id")
	if !errors.Is(err, ErrNotImplemented) {
		t.Errorf("ListSnapshots: expected ErrNotImplemented, got %v", err)
	}

	if err := r.DeleteSnapshot(ctx, "snap-id"); !errors.Is(err, ErrNotImplemented) {
		t.Errorf("DeleteSnapshot: expected ErrNotImplemented, got %v", err)
	}

	if err := r.CreateNetwork(ctx, "net", true); !errors.Is(err, ErrNotImplemented) {
		t.Errorf("CreateNetwork: expected ErrNotImplemented, got %v", err)
	}

	if err := r.DeleteNetwork(ctx, "net"); !errors.Is(err, ErrNotImplemented) {
		t.Errorf("DeleteNetwork: expected ErrNotImplemented, got %v", err)
	}
}

func TestFirecrackerRegistration(t *testing.T) {
	reg := runtime.NewRegistry()
	r := New()
	if err := reg.Register(r); err != nil {
		t.Fatalf("Register firecracker failed: %v", err)
	}

	rt, err := reg.Get("firecracker")
	if err != nil {
		t.Fatalf("Get firecracker failed: %v", err)
	}
	if rt.Name() != "firecracker" {
		t.Fatalf("expected runtime name %q, got %q", "firecracker", rt.Name())
	}
}
