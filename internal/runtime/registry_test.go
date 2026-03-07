package runtime

import (
	"context"
	"testing"
)

// mockRuntime implements Runtime for testing.
type mockRuntime struct {
	name string
}

func (m *mockRuntime) Name() string                                                  { return m.name }
func (m *mockRuntime) Create(_ context.Context, _ CreateConfig) (string, error)      { return "", nil }
func (m *mockRuntime) Start(_ context.Context, _ string) error                       { return nil }
func (m *mockRuntime) Stop(_ context.Context, _ string) error                        { return nil }
func (m *mockRuntime) Destroy(_ context.Context, _ string) error                     { return nil }
func (m *mockRuntime) Exec(_ context.Context, _ string, _ ExecConfig) (ExecResult, error) {
	return ExecResult{}, nil
}
func (m *mockRuntime) Info(_ context.Context, _ string) (Info, error)   { return Info{}, nil }
func (m *mockRuntime) Stats(_ context.Context, _ string) (Stats, error) { return Stats{}, nil }
func (m *mockRuntime) List(_ context.Context) ([]Info, error)           { return nil, nil }

func TestRegistry(t *testing.T) {
	reg := NewRegistry()

	// Register a runtime.
	mock := &mockRuntime{name: "test"}
	if err := reg.Register(mock); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Duplicate registration should fail.
	if err := reg.Register(mock); err == nil {
		t.Fatal("expected error on duplicate registration")
	}

	// Get registered runtime.
	rt, err := reg.Get("test")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if rt.Name() != "test" {
		t.Fatalf("expected name 'test', got %q", rt.Name())
	}

	// Get non-existent runtime.
	if _, err := reg.Get("nonexistent"); err == nil {
		t.Fatal("expected error for nonexistent runtime")
	}

	// List runtimes.
	names := reg.List()
	if len(names) != 1 || names[0] != "test" {
		t.Fatalf("expected [test], got %v", names)
	}
}
