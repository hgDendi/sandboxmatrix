package gvisor

import (
	"testing"

	"github.com/hg-dendi/sandboxmatrix/internal/runtime"
)

func TestGVisorName(t *testing.T) {
	// We cannot call New() in unit tests because it requires a Docker daemon.
	// Instead, verify the type at compile time and test Name() on a zero-value wrapper.
	r := &Runtime{}
	if got := r.Name(); got != "gvisor" {
		t.Fatalf("expected Name() = %q, got %q", "gvisor", got)
	}
}

// TestGVisorImplementsRuntime verifies at compile time that *Runtime satisfies runtime.Runtime.
var _ runtime.Runtime = (*Runtime)(nil)

func TestAvailable(t *testing.T) {
	// Available() should return a bool without panicking regardless of environment.
	_ = Available()
}

func TestGVisorRegistration(t *testing.T) {
	// Demonstrate that gVisor can be registered in the runtime registry.
	// We use a zero-value Runtime (embedded docker.Runtime is nil) just to test registration.
	reg := runtime.NewRegistry()
	r := &Runtime{}
	if err := reg.Register(r); err != nil {
		t.Fatalf("Register gvisor failed: %v", err)
	}

	rt, err := reg.Get("gvisor")
	if err != nil {
		t.Fatalf("Get gvisor failed: %v", err)
	}
	if rt.Name() != "gvisor" {
		t.Fatalf("expected runtime name %q, got %q", "gvisor", rt.Name())
	}
}
