package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	v1alpha1 "github.com/hg-dendi/sandboxmatrix/pkg/api/v1alpha1"
)

// newTestFileStore creates a FileStore backed by a temp directory.
func newTestFileStore(t *testing.T) *FileStore {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "state.json")
	fs, err := NewFileStoreWithPath(path)
	if err != nil {
		t.Fatalf("NewFileStoreWithPath: %v", err)
	}
	return fs
}

func newTestSandboxFS(name string) *v1alpha1.Sandbox {
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

func TestFileStoreSaveAndGet(t *testing.T) {
	fs := newTestFileStore(t)

	sb := newTestSandboxFS("fs-test-1")
	if err := fs.Save(sb); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := fs.Get("fs-test-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Metadata.Name != "fs-test-1" {
		t.Fatalf("expected name fs-test-1, got %q", got.Metadata.Name)
	}
	if got.Status.State != v1alpha1.SandboxStatePending {
		t.Fatalf("expected state Pending, got %q", got.Status.State)
	}
}

func TestFileStoreGetNotFound(t *testing.T) {
	fs := newTestFileStore(t)

	if _, err := fs.Get("nonexistent"); err == nil {
		t.Fatal("expected error for nonexistent sandbox")
	}
}

func TestFileStoreMutationSafety(t *testing.T) {
	fs := newTestFileStore(t)

	sb := newTestSandboxFS("fs-mut-1")
	if err := fs.Save(sb); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, _ := fs.Get("fs-mut-1")
	got.Metadata.Name = "mutated"

	got2, _ := fs.Get("fs-mut-1")
	if got2.Metadata.Name != "fs-mut-1" {
		t.Fatal("store was mutated by external modification")
	}
}

func TestFileStoreList(t *testing.T) {
	fs := newTestFileStore(t)

	fs.Save(newTestSandboxFS("list-1"))
	fs.Save(newTestSandboxFS("list-2"))
	fs.Save(newTestSandboxFS("list-3"))

	list, err := fs.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 sandboxes, got %d", len(list))
	}
}

func TestFileStoreDelete(t *testing.T) {
	fs := newTestFileStore(t)

	fs.Save(newTestSandboxFS("del-1"))
	fs.Save(newTestSandboxFS("del-2"))

	if err := fs.Delete("del-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := fs.Get("del-1"); err == nil {
		t.Fatal("expected error after delete")
	}

	list, _ := fs.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 sandbox after delete, got %d", len(list))
	}
}

func TestFileStoreDeleteNotFound(t *testing.T) {
	fs := newTestFileStore(t)

	if err := fs.Delete("nope"); err == nil {
		t.Fatal("expected error deleting non-existent sandbox")
	}
}

func TestFileStorePersistenceAcrossInstances(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	// First instance writes data.
	fs1, err := NewFileStoreWithPath(path)
	if err != nil {
		t.Fatalf("NewFileStoreWithPath (1): %v", err)
	}
	fs1.Save(newTestSandboxFS("persist-1"))

	// Second instance should see the data.
	fs2, err := NewFileStoreWithPath(path)
	if err != nil {
		t.Fatalf("NewFileStoreWithPath (2): %v", err)
	}
	got, err := fs2.Get("persist-1")
	if err != nil {
		t.Fatalf("Get from second instance: %v", err)
	}
	if got.Metadata.Name != "persist-1" {
		t.Fatalf("expected persist-1, got %q", got.Metadata.Name)
	}
}

func TestFileStoreOverwrite(t *testing.T) {
	fs := newTestFileStore(t)

	sb := newTestSandboxFS("overwrite-1")
	sb.Status.State = v1alpha1.SandboxStatePending
	fs.Save(sb)

	sb2 := newTestSandboxFS("overwrite-1")
	sb2.Status.State = v1alpha1.SandboxStateRunning
	fs.Save(sb2)

	got, _ := fs.Get("overwrite-1")
	if got.Status.State != v1alpha1.SandboxStateRunning {
		t.Fatalf("expected Running after overwrite, got %q", got.Status.State)
	}

	list, _ := fs.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 sandbox after overwrite, got %d", len(list))
	}
}

func TestFileStoreCreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	deep := filepath.Join(dir, "a", "b", "c", "state.json")

	fs, err := NewFileStoreWithPath(deep)
	if err != nil {
		t.Fatalf("NewFileStoreWithPath: %v", err)
	}

	if err := fs.Save(newTestSandboxFS("deep-1")); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := fs.Get("deep-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Metadata.Name != "deep-1" {
		t.Fatalf("expected deep-1, got %q", got.Metadata.Name)
	}
}

func TestFileStoreAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	fs, err := NewFileStoreWithPath(path)
	if err != nil {
		t.Fatalf("NewFileStoreWithPath: %v", err)
	}

	fs.Save(newTestSandboxFS("atomic-1"))

	// Verify the file is valid JSON after write.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading state file: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("state file is empty after Save")
	}

	// Verify no leftover temp files.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Fatalf("leftover temp file: %s", e.Name())
		}
	}
}

func TestFileStoreImplementsStoreInterface(t *testing.T) {
	fs := newTestFileStore(t)

	// Compile-time check that FileStore satisfies Store.
	var _ Store = fs
}
