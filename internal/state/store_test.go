package state

import (
	"testing"
	"time"

	v1alpha1 "github.com/hg-dendi/sandboxmatrix/pkg/api/v1alpha1"
)

func newTestSandbox(name string) *v1alpha1.Sandbox {
	return &v1alpha1.Sandbox{
		Metadata: v1alpha1.ObjectMeta{
			Name:      name,
			CreatedAt: time.Now(),
		},
		Status: v1alpha1.SandboxStatus{
			State: v1alpha1.SandboxStatePending,
		},
	}
}

func TestMemoryStore(t *testing.T) {
	s := NewMemoryStore()

	// Save.
	sb := newTestSandbox("test-1")
	if err := s.Save(sb); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Get.
	got, err := s.Get("test-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Metadata.Name != "test-1" {
		t.Fatalf("expected name test-1, got %q", got.Metadata.Name)
	}

	// Mutation safety: modifying returned copy shouldn't affect store.
	got.Metadata.Name = "mutated"
	got2, _ := s.Get("test-1")
	if got2.Metadata.Name != "test-1" {
		t.Fatal("store was mutated by external modification")
	}

	// List.
	s.Save(newTestSandbox("test-2"))
	list, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 sandboxes, got %d", len(list))
	}

	// Delete.
	if err := s.Delete("test-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get("test-1"); err == nil {
		t.Fatal("expected error after delete")
	}

	// Delete non-existent.
	if err := s.Delete("nope"); err == nil {
		t.Fatal("expected error deleting non-existent")
	}
}
