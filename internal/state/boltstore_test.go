package state

import (
	"path/filepath"
	"testing"
	"time"

	v1alpha1 "github.com/hg-dendi/sandboxmatrix/pkg/api/v1alpha1"
)

// newTestBoltStore creates a BoltStore backed by a temp directory.
func newTestBoltStore(t *testing.T) *BoltStore {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "state.db")
	bs, err := NewBoltStoreWithPath(path)
	if err != nil {
		t.Fatalf("NewBoltStoreWithPath: %v", err)
	}
	t.Cleanup(func() { bs.Close() })
	return bs
}

func newTestSandboxBolt(name string) *v1alpha1.Sandbox {
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

func newTestSessionBolt(id, sandbox string) *v1alpha1.Session {
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

// ---------------------------------------------------------------------------
// Store interface tests (sandboxes)
// ---------------------------------------------------------------------------

func TestBoltStoreSaveAndGet(t *testing.T) {
	bs := newTestBoltStore(t)

	sb := newTestSandboxBolt("bolt-test-1")
	if err := bs.Save(sb); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := bs.Get("bolt-test-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Metadata.Name != "bolt-test-1" {
		t.Fatalf("expected name bolt-test-1, got %q", got.Metadata.Name)
	}
	if got.Status.State != v1alpha1.SandboxStatePending {
		t.Fatalf("expected state Pending, got %q", got.Status.State)
	}
}

func TestBoltStoreGetNotFound(t *testing.T) {
	bs := newTestBoltStore(t)

	if _, err := bs.Get("nonexistent"); err == nil {
		t.Fatal("expected error for nonexistent sandbox")
	}
}

func TestBoltStoreMutationSafety(t *testing.T) {
	bs := newTestBoltStore(t)

	sb := newTestSandboxBolt("bolt-mut-1")
	if err := bs.Save(sb); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, _ := bs.Get("bolt-mut-1")
	got.Metadata.Name = "mutated"

	got2, _ := bs.Get("bolt-mut-1")
	if got2.Metadata.Name != "bolt-mut-1" {
		t.Fatal("store was mutated by external modification")
	}
}

func TestBoltStoreList(t *testing.T) {
	bs := newTestBoltStore(t)

	bs.Save(newTestSandboxBolt("list-1"))
	bs.Save(newTestSandboxBolt("list-2"))
	bs.Save(newTestSandboxBolt("list-3"))

	list, err := bs.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 sandboxes, got %d", len(list))
	}
}

func TestBoltStoreDelete(t *testing.T) {
	bs := newTestBoltStore(t)

	bs.Save(newTestSandboxBolt("del-1"))
	bs.Save(newTestSandboxBolt("del-2"))

	if err := bs.Delete("del-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := bs.Get("del-1"); err == nil {
		t.Fatal("expected error after delete")
	}

	list, _ := bs.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 sandbox after delete, got %d", len(list))
	}
}

func TestBoltStoreDeleteNotFound(t *testing.T) {
	bs := newTestBoltStore(t)

	if err := bs.Delete("nope"); err == nil {
		t.Fatal("expected error deleting non-existent sandbox")
	}
}

func TestBoltStoreOverwrite(t *testing.T) {
	bs := newTestBoltStore(t)

	sb := newTestSandboxBolt("overwrite-1")
	sb.Status.State = v1alpha1.SandboxStatePending
	bs.Save(sb)

	sb2 := newTestSandboxBolt("overwrite-1")
	sb2.Status.State = v1alpha1.SandboxStateRunning
	bs.Save(sb2)

	got, _ := bs.Get("overwrite-1")
	if got.Status.State != v1alpha1.SandboxStateRunning {
		t.Fatalf("expected Running after overwrite, got %q", got.Status.State)
	}

	list, _ := bs.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 sandbox after overwrite, got %d", len(list))
	}
}

func TestBoltStoreCreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	deep := filepath.Join(dir, "a", "b", "c", "state.db")

	bs, err := NewBoltStoreWithPath(deep)
	if err != nil {
		t.Fatalf("NewBoltStoreWithPath: %v", err)
	}
	defer bs.Close()

	if err := bs.Save(newTestSandboxBolt("deep-1")); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := bs.Get("deep-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Metadata.Name != "deep-1" {
		t.Fatalf("expected deep-1, got %q", got.Metadata.Name)
	}
}

func TestBoltStorePersistenceAcrossCloseReopen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.db")

	// First instance writes data.
	bs1, err := NewBoltStoreWithPath(path)
	if err != nil {
		t.Fatalf("NewBoltStoreWithPath (1): %v", err)
	}
	bs1.Save(newTestSandboxBolt("persist-1"))
	bs1.SaveSession(newTestSessionBolt("sess-persist-1", "persist-1"))
	bs1.Close()

	// Second instance should see the data.
	bs2, err := NewBoltStoreWithPath(path)
	if err != nil {
		t.Fatalf("NewBoltStoreWithPath (2): %v", err)
	}
	defer bs2.Close()

	got, err := bs2.Get("persist-1")
	if err != nil {
		t.Fatalf("Get from second instance: %v", err)
	}
	if got.Metadata.Name != "persist-1" {
		t.Fatalf("expected persist-1, got %q", got.Metadata.Name)
	}

	sess, err := bs2.GetSession("sess-persist-1")
	if err != nil {
		t.Fatalf("GetSession from second instance: %v", err)
	}
	if sess.Metadata.Name != "sess-persist-1" {
		t.Fatalf("expected sess-persist-1, got %q", sess.Metadata.Name)
	}
}

func TestBoltStoreImplementsStoreInterface(t *testing.T) {
	bs := newTestBoltStore(t)

	// Compile-time check that BoltStore satisfies Store.
	var _ Store = bs
}

// ---------------------------------------------------------------------------
// SessionStore interface tests (sessions)
// ---------------------------------------------------------------------------

func TestBoltStoreSessionSaveAndGet(t *testing.T) {
	bs := newTestBoltStore(t)

	s := newTestSessionBolt("sess-1", "my-sandbox")
	if err := bs.SaveSession(s); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	got, err := bs.GetSession("sess-1")
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

func TestBoltStoreSessionGetNotFound(t *testing.T) {
	bs := newTestBoltStore(t)

	if _, err := bs.GetSession("nonexistent"); err == nil {
		t.Fatal("expected error for nonexistent session")
	}
}

func TestBoltStoreSessionMutationSafety(t *testing.T) {
	bs := newTestBoltStore(t)

	s := newTestSessionBolt("sess-mut", "sandbox-1")
	if err := bs.SaveSession(s); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	got, _ := bs.GetSession("sess-mut")
	got.Metadata.Name = "mutated"

	got2, _ := bs.GetSession("sess-mut")
	if got2.Metadata.Name != "sess-mut" {
		t.Fatal("store was mutated by external modification")
	}
}

func TestBoltStoreListSessions(t *testing.T) {
	bs := newTestBoltStore(t)

	bs.SaveSession(newTestSessionBolt("list-1", "sb-1"))
	bs.SaveSession(newTestSessionBolt("list-2", "sb-2"))
	bs.SaveSession(newTestSessionBolt("list-3", "sb-1"))

	list, err := bs.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 sessions, got %d", len(list))
	}
}

func TestBoltStoreListSessionsBySandbox(t *testing.T) {
	bs := newTestBoltStore(t)

	bs.SaveSession(newTestSessionBolt("filter-1", "sb-alpha"))
	bs.SaveSession(newTestSessionBolt("filter-2", "sb-beta"))
	bs.SaveSession(newTestSessionBolt("filter-3", "sb-alpha"))
	bs.SaveSession(newTestSessionBolt("filter-4", "sb-beta"))

	alphaList, err := bs.ListSessionsBySandbox("sb-alpha")
	if err != nil {
		t.Fatalf("ListSessionsBySandbox(sb-alpha): %v", err)
	}
	if len(alphaList) != 2 {
		t.Fatalf("expected 2 sessions for sb-alpha, got %d", len(alphaList))
	}

	betaList, err := bs.ListSessionsBySandbox("sb-beta")
	if err != nil {
		t.Fatalf("ListSessionsBySandbox(sb-beta): %v", err)
	}
	if len(betaList) != 2 {
		t.Fatalf("expected 2 sessions for sb-beta, got %d", len(betaList))
	}

	emptyList, err := bs.ListSessionsBySandbox("sb-gamma")
	if err != nil {
		t.Fatalf("ListSessionsBySandbox(sb-gamma): %v", err)
	}
	if len(emptyList) != 0 {
		t.Fatalf("expected 0 sessions for sb-gamma, got %d", len(emptyList))
	}
}

func TestBoltStoreSessionDelete(t *testing.T) {
	bs := newTestBoltStore(t)

	bs.SaveSession(newTestSessionBolt("del-1", "sb-1"))
	bs.SaveSession(newTestSessionBolt("del-2", "sb-1"))

	if err := bs.DeleteSession("del-1"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if _, err := bs.GetSession("del-1"); err == nil {
		t.Fatal("expected error after delete")
	}

	list, _ := bs.ListSessions()
	if len(list) != 1 {
		t.Fatalf("expected 1 session after delete, got %d", len(list))
	}
}

func TestBoltStoreSessionDeleteNotFound(t *testing.T) {
	bs := newTestBoltStore(t)

	if err := bs.DeleteSession("nope"); err == nil {
		t.Fatal("expected error deleting non-existent session")
	}
}

func TestBoltStoreSessionOverwrite(t *testing.T) {
	bs := newTestBoltStore(t)

	s := newTestSessionBolt("overwrite-1", "sb-1")
	s.State = v1alpha1.SessionStateActive
	bs.SaveSession(s)

	s2 := newTestSessionBolt("overwrite-1", "sb-1")
	s2.State = v1alpha1.SessionStateCompleted
	s2.ExecCount = 5
	bs.SaveSession(s2)

	got, _ := bs.GetSession("overwrite-1")
	if got.State != v1alpha1.SessionStateCompleted {
		t.Fatalf("expected Completed after overwrite, got %q", got.State)
	}
	if got.ExecCount != 5 {
		t.Fatalf("expected execCount 5 after overwrite, got %d", got.ExecCount)
	}

	list, _ := bs.ListSessions()
	if len(list) != 1 {
		t.Fatalf("expected 1 session after overwrite, got %d", len(list))
	}
}

func TestBoltStoreImplementsSessionStoreInterface(t *testing.T) {
	bs := newTestBoltStore(t)

	// Compile-time check that BoltStore satisfies SessionStore.
	var _ SessionStore = bs
}

// ---------------------------------------------------------------------------
// Combined store tests
// ---------------------------------------------------------------------------

func TestBoltStoreSandboxesAndSessionsAreIndependent(t *testing.T) {
	bs := newTestBoltStore(t)

	// Save a sandbox and a session with the same key to verify bucket isolation.
	sb := newTestSandboxBolt("shared-key")
	sess := newTestSessionBolt("shared-key", "some-sandbox")

	if err := bs.Save(sb); err != nil {
		t.Fatalf("Save sandbox: %v", err)
	}
	if err := bs.SaveSession(sess); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	// Deleting the sandbox should not affect the session.
	if err := bs.Delete("shared-key"); err != nil {
		t.Fatalf("Delete sandbox: %v", err)
	}

	got, err := bs.GetSession("shared-key")
	if err != nil {
		t.Fatalf("GetSession after sandbox delete: %v", err)
	}
	if got.Metadata.Name != "shared-key" {
		t.Fatalf("expected session shared-key, got %q", got.Metadata.Name)
	}

	// The sandbox should be gone.
	if _, err := bs.Get("shared-key"); err == nil {
		t.Fatal("expected error for deleted sandbox")
	}
}

func TestBoltStoreClose(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.db")

	bs, err := NewBoltStoreWithPath(path)
	if err != nil {
		t.Fatalf("NewBoltStoreWithPath: %v", err)
	}

	bs.Save(newTestSandboxBolt("close-test"))

	if err := bs.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// After close, operations should fail.
	if _, err := bs.Get("close-test"); err == nil {
		t.Fatal("expected error after close")
	}
}
