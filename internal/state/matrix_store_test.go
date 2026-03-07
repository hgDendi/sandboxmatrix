package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	v1alpha1 "github.com/hg-dendi/sandboxmatrix/pkg/api/v1alpha1"
)

// ---------------------------------------------------------------------------
// MemoryMatrixStore tests
// ---------------------------------------------------------------------------

func TestMemoryMatrixStore_SaveAndGet(t *testing.T) {
	store := NewMemoryMatrixStore()
	m := newTestMatrix("test-matrix")

	if err := store.Save(m); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := store.Get("test-matrix")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Metadata.Name != "test-matrix" {
		t.Fatalf("expected name test-matrix, got %q", got.Metadata.Name)
	}
	if len(got.Members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(got.Members))
	}
	if got.State != v1alpha1.MatrixStateActive {
		t.Fatalf("expected state Active, got %s", got.State)
	}
}

func TestMemoryMatrixStore_GetNotFound(t *testing.T) {
	store := NewMemoryMatrixStore()
	_, err := store.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent matrix")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found' error, got: %v", err)
	}
}

func TestMemoryMatrixStore_List(t *testing.T) {
	store := NewMemoryMatrixStore()
	if err := store.Save(newTestMatrix("alpha")); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(newTestMatrix("beta")); err != nil {
		t.Fatal(err)
	}

	list, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 matrices, got %d", len(list))
	}
}

func TestMemoryMatrixStore_ListEmpty(t *testing.T) {
	store := NewMemoryMatrixStore()
	list, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected 0 matrices, got %d", len(list))
	}
}

func TestMemoryMatrixStore_Delete(t *testing.T) {
	store := NewMemoryMatrixStore()
	if err := store.Save(newTestMatrix("del-me")); err != nil {
		t.Fatal(err)
	}

	if err := store.Delete("del-me"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err := store.Get("del-me")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestMemoryMatrixStore_DeleteNotFound(t *testing.T) {
	store := NewMemoryMatrixStore()
	err := store.Delete("ghost")
	if err == nil {
		t.Fatal("expected error deleting non-existent matrix")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found' error, got: %v", err)
	}
}

func TestMemoryMatrixStore_SaveOverwrite(t *testing.T) {
	store := NewMemoryMatrixStore()

	m := newTestMatrix("overwrite")
	if err := store.Save(m); err != nil {
		t.Fatal(err)
	}

	m.State = v1alpha1.MatrixStateStopped
	if err := store.Save(m); err != nil {
		t.Fatal(err)
	}

	got, _ := store.Get("overwrite")
	if got.State != v1alpha1.MatrixStateStopped {
		t.Fatalf("expected state Stopped after overwrite, got %s", got.State)
	}
}

func TestMemoryMatrixStore_IsolatesCopies(t *testing.T) {
	store := NewMemoryMatrixStore()
	m := newTestMatrix("isolate")
	if err := store.Save(m); err != nil {
		t.Fatal(err)
	}

	// Mutate the original; it should not affect the store.
	m.State = v1alpha1.MatrixStateDestroyed
	got, _ := store.Get("isolate")
	if got.State != v1alpha1.MatrixStateActive {
		t.Fatalf("expected state Active (isolated), got %s", got.State)
	}
}

// ---------------------------------------------------------------------------
// FileMatrixStore tests
// ---------------------------------------------------------------------------

func TestFileMatrixStore_SaveAndGet(t *testing.T) {
	store := newTempFileMatrixStore(t)
	m := newTestMatrix("file-test")

	if err := store.Save(m); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := store.Get("file-test")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Metadata.Name != "file-test" {
		t.Fatalf("expected name file-test, got %q", got.Metadata.Name)
	}
	if len(got.Members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(got.Members))
	}
}

func TestFileMatrixStore_GetNotFound(t *testing.T) {
	store := newTempFileMatrixStore(t)
	_, err := store.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent matrix")
	}
}

func TestFileMatrixStore_List(t *testing.T) {
	store := newTempFileMatrixStore(t)
	store.Save(newTestMatrix("a"))
	store.Save(newTestMatrix("b"))

	list, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 matrices, got %d", len(list))
	}
}

func TestFileMatrixStore_Delete(t *testing.T) {
	store := newTempFileMatrixStore(t)
	store.Save(newTestMatrix("del-me"))

	if err := store.Delete("del-me"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err := store.Get("del-me")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestFileMatrixStore_DeleteNotFound(t *testing.T) {
	store := newTempFileMatrixStore(t)
	err := store.Delete("ghost")
	if err == nil {
		t.Fatal("expected error deleting non-existent matrix")
	}
}

func TestFileMatrixStore_Persistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "matrices.json")

	// Create store and save a matrix.
	store1, err := NewFileMatrixStoreWithPath(path)
	if err != nil {
		t.Fatalf("NewFileMatrixStoreWithPath: %v", err)
	}
	store1.Save(newTestMatrix("persist"))

	// Create a second store pointing to the same file.
	store2, err := NewFileMatrixStoreWithPath(path)
	if err != nil {
		t.Fatalf("NewFileMatrixStoreWithPath: %v", err)
	}
	got, err := store2.Get("persist")
	if err != nil {
		t.Fatalf("Get from second store: %v", err)
	}
	if got.Metadata.Name != "persist" {
		t.Fatalf("expected name persist, got %q", got.Metadata.Name)
	}
}

func TestFileMatrixStore_AtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "matrices.json")
	store, err := NewFileMatrixStoreWithPath(path)
	if err != nil {
		t.Fatalf("NewFileMatrixStoreWithPath: %v", err)
	}

	store.Save(newTestMatrix("atomic"))

	// Verify no temp files remain.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("found leftover temp file: %s", e.Name())
		}
	}
}

func TestFileMatrixStore_CreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "dir")
	path := filepath.Join(dir, "matrices.json")

	_, err := NewFileMatrixStoreWithPath(path)
	if err != nil {
		t.Fatalf("NewFileMatrixStoreWithPath: %v", err)
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Fatal("expected directory to be created")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newTestMatrix(name string) *v1alpha1.Matrix {
	now := time.Now()
	return &v1alpha1.Matrix{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Matrix"},
		Metadata: v1alpha1.ObjectMeta{
			Name:      name,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Members: []v1alpha1.MatrixMember{
			{Name: "frontend", Blueprint: "blueprints/node-dev.yaml"},
			{Name: "backend", Blueprint: "blueprints/python-dev.yaml"},
		},
		State: v1alpha1.MatrixStateActive,
	}
}

func newTempFileMatrixStore(t *testing.T) *FileMatrixStore {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "matrices.json")
	store, err := NewFileMatrixStoreWithPath(path)
	if err != nil {
		t.Fatalf("NewFileMatrixStoreWithPath: %v", err)
	}
	return store
}
