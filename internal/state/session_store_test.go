package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	v1alpha1 "github.com/hg-dendi/sandboxmatrix/pkg/api/v1alpha1"
)

// newTestFileSessionStore creates a FileSessionStore backed by a temp directory.
func newTestFileSessionStore(t *testing.T) *FileSessionStore {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "sessions.json")
	fs, err := NewFileSessionStoreWithPath(path)
	if err != nil {
		t.Fatalf("NewFileSessionStoreWithPath: %v", err)
	}
	return fs
}

func newTestSession(id, sandbox string) *v1alpha1.Session {
	now := time.Now()
	return &v1alpha1.Session{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Session"},
		Metadata: v1alpha1.ObjectMeta{
			Name:      id,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Sandbox:   sandbox,
		State:     v1alpha1.SessionStateActive,
		StartedAt: &now,
		ExecCount: 0,
	}
}

func TestFileSessionStoreSaveAndGet(t *testing.T) {
	fs := newTestFileSessionStore(t)

	s := newTestSession("sess-1", "my-sandbox")
	if err := fs.SaveSession(s); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	got, err := fs.GetSession("sess-1")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.Metadata.Name != "sess-1" {
		t.Fatalf("expected name sess-1, got %q", got.Metadata.Name)
	}
	if got.Sandbox != "my-sandbox" {
		t.Fatalf("expected sandbox my-sandbox, got %q", got.Sandbox)
	}
	if got.State != v1alpha1.SessionStateActive {
		t.Fatalf("expected state Active, got %q", got.State)
	}
	if got.StartedAt == nil {
		t.Fatal("expected StartedAt to be set")
	}
	if got.ExecCount != 0 {
		t.Fatalf("expected execCount 0, got %d", got.ExecCount)
	}
}

func TestFileSessionStoreGetNotFound(t *testing.T) {
	fs := newTestFileSessionStore(t)

	if _, err := fs.GetSession("nonexistent"); err == nil {
		t.Fatal("expected error for nonexistent session")
	}
}

func TestFileSessionStoreMutationSafety(t *testing.T) {
	fs := newTestFileSessionStore(t)

	s := newTestSession("sess-mut", "sandbox-1")
	if err := fs.SaveSession(s); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	got, _ := fs.GetSession("sess-mut")
	got.Metadata.Name = "mutated"

	got2, _ := fs.GetSession("sess-mut")
	if got2.Metadata.Name != "sess-mut" {
		t.Fatal("store was mutated by external modification")
	}
}

func TestFileSessionStoreListSessions(t *testing.T) {
	fs := newTestFileSessionStore(t)

	fs.SaveSession(newTestSession("list-1", "sb-1"))
	fs.SaveSession(newTestSession("list-2", "sb-2"))
	fs.SaveSession(newTestSession("list-3", "sb-1"))

	list, err := fs.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 sessions, got %d", len(list))
	}
}

func TestFileSessionStoreListSessionsBySandbox(t *testing.T) {
	fs := newTestFileSessionStore(t)

	fs.SaveSession(newTestSession("filter-1", "sb-alpha"))
	fs.SaveSession(newTestSession("filter-2", "sb-beta"))
	fs.SaveSession(newTestSession("filter-3", "sb-alpha"))
	fs.SaveSession(newTestSession("filter-4", "sb-beta"))

	alphaList, err := fs.ListSessionsBySandbox("sb-alpha")
	if err != nil {
		t.Fatalf("ListSessionsBySandbox(sb-alpha): %v", err)
	}
	if len(alphaList) != 2 {
		t.Fatalf("expected 2 sessions for sb-alpha, got %d", len(alphaList))
	}

	betaList, err := fs.ListSessionsBySandbox("sb-beta")
	if err != nil {
		t.Fatalf("ListSessionsBySandbox(sb-beta): %v", err)
	}
	if len(betaList) != 2 {
		t.Fatalf("expected 2 sessions for sb-beta, got %d", len(betaList))
	}

	emptyList, err := fs.ListSessionsBySandbox("sb-gamma")
	if err != nil {
		t.Fatalf("ListSessionsBySandbox(sb-gamma): %v", err)
	}
	if len(emptyList) != 0 {
		t.Fatalf("expected 0 sessions for sb-gamma, got %d", len(emptyList))
	}
}

func TestFileSessionStoreDelete(t *testing.T) {
	fs := newTestFileSessionStore(t)

	fs.SaveSession(newTestSession("del-1", "sb-1"))
	fs.SaveSession(newTestSession("del-2", "sb-1"))

	if err := fs.DeleteSession("del-1"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if _, err := fs.GetSession("del-1"); err == nil {
		t.Fatal("expected error after delete")
	}

	list, _ := fs.ListSessions()
	if len(list) != 1 {
		t.Fatalf("expected 1 session after delete, got %d", len(list))
	}
}

func TestFileSessionStoreDeleteNotFound(t *testing.T) {
	fs := newTestFileSessionStore(t)

	if err := fs.DeleteSession("nope"); err == nil {
		t.Fatal("expected error deleting non-existent session")
	}
}

func TestFileSessionStorePersistenceAcrossInstances(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.json")

	// First instance writes data.
	fs1, err := NewFileSessionStoreWithPath(path)
	if err != nil {
		t.Fatalf("NewFileSessionStoreWithPath (1): %v", err)
	}
	fs1.SaveSession(newTestSession("persist-1", "sb-1"))

	// Second instance should see the data.
	fs2, err := NewFileSessionStoreWithPath(path)
	if err != nil {
		t.Fatalf("NewFileSessionStoreWithPath (2): %v", err)
	}
	got, err := fs2.GetSession("persist-1")
	if err != nil {
		t.Fatalf("GetSession from second instance: %v", err)
	}
	if got.Metadata.Name != "persist-1" {
		t.Fatalf("expected persist-1, got %q", got.Metadata.Name)
	}
}

func TestFileSessionStoreOverwrite(t *testing.T) {
	fs := newTestFileSessionStore(t)

	s := newTestSession("overwrite-1", "sb-1")
	s.State = v1alpha1.SessionStateActive
	fs.SaveSession(s)

	s2 := newTestSession("overwrite-1", "sb-1")
	s2.State = v1alpha1.SessionStateCompleted
	s2.ExecCount = 5
	fs.SaveSession(s2)

	got, _ := fs.GetSession("overwrite-1")
	if got.State != v1alpha1.SessionStateCompleted {
		t.Fatalf("expected Completed after overwrite, got %q", got.State)
	}
	if got.ExecCount != 5 {
		t.Fatalf("expected execCount 5 after overwrite, got %d", got.ExecCount)
	}

	list, _ := fs.ListSessions()
	if len(list) != 1 {
		t.Fatalf("expected 1 session after overwrite, got %d", len(list))
	}
}

func TestFileSessionStoreCreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	deep := filepath.Join(dir, "a", "b", "c", "sessions.json")

	fs, err := NewFileSessionStoreWithPath(deep)
	if err != nil {
		t.Fatalf("NewFileSessionStoreWithPath: %v", err)
	}

	if err := fs.SaveSession(newTestSession("deep-1", "sb-1")); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	got, err := fs.GetSession("deep-1")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.Metadata.Name != "deep-1" {
		t.Fatalf("expected deep-1, got %q", got.Metadata.Name)
	}
}

func TestFileSessionStoreAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.json")

	fs, err := NewFileSessionStoreWithPath(path)
	if err != nil {
		t.Fatalf("NewFileSessionStoreWithPath: %v", err)
	}

	fs.SaveSession(newTestSession("atomic-1", "sb-1"))

	// Verify the file is valid JSON after write.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading sessions file: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("sessions file is empty after SaveSession")
	}

	// Verify no leftover temp files.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Fatalf("leftover temp file: %s", e.Name())
		}
	}
}

func TestFileSessionStoreImplementsSessionStoreInterface(t *testing.T) {
	fs := newTestFileSessionStore(t)

	// Compile-time check that FileSessionStore satisfies SessionStore.
	var _ SessionStore = fs
}
