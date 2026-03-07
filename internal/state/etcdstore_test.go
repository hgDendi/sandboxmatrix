package state

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	v1alpha1 "github.com/hg-dendi/sandboxmatrix/pkg/api/v1alpha1"
)

// ---------------------------------------------------------------------------
// Compile-time interface checks
// ---------------------------------------------------------------------------

var (
	_ Store        = (*EtcdStore)(nil)
	_ SessionStore = (*EtcdStore)(nil)
	_ MatrixStore  = (*EtcdMatrixStore)(nil)
)

// ---------------------------------------------------------------------------
// Unit tests (no etcd required)
// ---------------------------------------------------------------------------

func TestEtcdStore_KeyPrefix(t *testing.T) {
	// Verify key layout without needing an etcd connection.
	s := &EtcdStore{prefix: "/sandboxmatrix/"}

	if got := s.sandboxKey("my-sb"); got != "/sandboxmatrix/sandboxes/my-sb" {
		t.Errorf("sandboxKey = %q, want %q", got, "/sandboxmatrix/sandboxes/my-sb")
	}
	if got := s.sandboxPrefix(); got != "/sandboxmatrix/sandboxes/" {
		t.Errorf("sandboxPrefix = %q, want %q", got, "/sandboxmatrix/sandboxes/")
	}
	if got := s.sessionKey("sess-1"); got != "/sandboxmatrix/sessions/sess-1" {
		t.Errorf("sessionKey = %q, want %q", got, "/sandboxmatrix/sessions/sess-1")
	}
	if got := s.sessionPrefix(); got != "/sandboxmatrix/sessions/" {
		t.Errorf("sessionPrefix = %q, want %q", got, "/sandboxmatrix/sessions/")
	}
}

func TestEtcdStore_CustomPrefix(t *testing.T) {
	s := &EtcdStore{prefix: "/custom/prefix/"}

	if got := s.sandboxKey("sb1"); got != "/custom/prefix/sandboxes/sb1" {
		t.Errorf("sandboxKey = %q, want %q", got, "/custom/prefix/sandboxes/sb1")
	}
	if got := s.sessionKey("s1"); got != "/custom/prefix/sessions/s1" {
		t.Errorf("sessionKey = %q, want %q", got, "/custom/prefix/sessions/s1")
	}
}

func TestEtcdMatrixStore_KeyPrefix(t *testing.T) {
	m := &EtcdMatrixStore{prefix: "/sandboxmatrix/"}

	if got := m.matrixKey("my-matrix"); got != "/sandboxmatrix/matrices/my-matrix" {
		t.Errorf("matrixKey = %q, want %q", got, "/sandboxmatrix/matrices/my-matrix")
	}
	if got := m.matrixPrefix(); got != "/sandboxmatrix/matrices/" {
		t.Errorf("matrixPrefix = %q, want %q", got, "/sandboxmatrix/matrices/")
	}
}

func TestEtcdStore_SandboxJSONRoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)
	sb := &v1alpha1.Sandbox{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "v1alpha1", Kind: "Sandbox"},
		Metadata: v1alpha1.ObjectMeta{
			Name:      "test-sandbox",
			Labels:    map[string]string{"env": "dev"},
			CreatedAt: now,
		},
		Spec: v1alpha1.SandboxSpec{
			BlueprintRef: "base-python",
		},
		Status: v1alpha1.SandboxStatus{
			State: v1alpha1.SandboxStateRunning,
			IP:    "10.0.0.1",
		},
	}

	data, err := json.Marshal(sb)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got v1alpha1.Sandbox
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Metadata.Name != sb.Metadata.Name {
		t.Errorf("name = %q, want %q", got.Metadata.Name, sb.Metadata.Name)
	}
	if got.Status.State != sb.Status.State {
		t.Errorf("state = %q, want %q", got.Status.State, sb.Status.State)
	}
	if got.Status.IP != sb.Status.IP {
		t.Errorf("ip = %q, want %q", got.Status.IP, sb.Status.IP)
	}
}

func TestEtcdStore_SessionJSONRoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)
	sess := &v1alpha1.Session{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "v1alpha1", Kind: "Session"},
		Metadata: v1alpha1.ObjectMeta{
			Name:      "sess-abc",
			CreatedAt: now,
		},
		Sandbox:   "my-sandbox",
		State:     v1alpha1.SessionStateActive,
		StartedAt: &now,
		ExecCount: 5,
	}

	data, err := json.Marshal(sess)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got v1alpha1.Session
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Metadata.Name != sess.Metadata.Name {
		t.Errorf("name = %q, want %q", got.Metadata.Name, sess.Metadata.Name)
	}
	if got.Sandbox != sess.Sandbox {
		t.Errorf("sandbox = %q, want %q", got.Sandbox, sess.Sandbox)
	}
	if got.ExecCount != sess.ExecCount {
		t.Errorf("execCount = %d, want %d", got.ExecCount, sess.ExecCount)
	}
}

func TestEtcdStore_MatrixJSONRoundTrip(t *testing.T) {
	mat := &v1alpha1.Matrix{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "v1alpha1", Kind: "Matrix"},
		Metadata: v1alpha1.ObjectMeta{Name: "my-matrix"},
		Members: []v1alpha1.MatrixMember{
			{Name: "sb-1", Blueprint: "python"},
			{Name: "sb-2", Blueprint: "node"},
		},
		State: v1alpha1.MatrixStateActive,
	}

	data, err := json.Marshal(mat)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got v1alpha1.Matrix
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Metadata.Name != mat.Metadata.Name {
		t.Errorf("name = %q, want %q", got.Metadata.Name, mat.Metadata.Name)
	}
	if len(got.Members) != len(mat.Members) {
		t.Fatalf("members len = %d, want %d", len(got.Members), len(mat.Members))
	}
	if got.Members[0].Name != "sb-1" || got.Members[1].Blueprint != "node" {
		t.Errorf("members mismatch: %+v", got.Members)
	}
}

// ---------------------------------------------------------------------------
// Integration tests (require a running etcd; set ETCD_ENDPOINTS)
// ---------------------------------------------------------------------------

func etcdEndpoints(t *testing.T) []string {
	t.Helper()
	ep := os.Getenv("ETCD_ENDPOINTS")
	if ep == "" {
		t.Skip("set ETCD_ENDPOINTS to run etcd integration tests")
	}
	return []string{ep}
}

func TestEtcdStore_Integration_SandboxCRUD(t *testing.T) {
	endpoints := etcdEndpoints(t)

	// Use a unique prefix per test to avoid collisions.
	prefix := "/sandboxmatrix-test-sandbox-crud/"
	store, err := NewEtcdStoreWithPrefix(endpoints, prefix)
	if err != nil {
		t.Fatalf("NewEtcdStoreWithPrefix: %v", err)
	}
	defer store.Close()

	sb := &v1alpha1.Sandbox{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "v1alpha1", Kind: "Sandbox"},
		Metadata: v1alpha1.ObjectMeta{Name: "int-test-sb"},
		Spec:     v1alpha1.SandboxSpec{BlueprintRef: "base"},
		Status:   v1alpha1.SandboxStatus{State: v1alpha1.SandboxStateRunning},
	}

	// Save
	if err := store.Save(sb); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Get
	got, err := store.Get("int-test-sb")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Metadata.Name != "int-test-sb" {
		t.Errorf("name = %q, want %q", got.Metadata.Name, "int-test-sb")
	}
	if got.Status.State != v1alpha1.SandboxStateRunning {
		t.Errorf("state = %q, want %q", got.Status.State, v1alpha1.SandboxStateRunning)
	}

	// List
	list, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) < 1 {
		t.Fatalf("List returned %d items, want >= 1", len(list))
	}

	// Delete
	if err := store.Delete("int-test-sb"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Get after delete should fail
	_, err = store.Get("int-test-sb")
	if err == nil {
		t.Fatal("Get after Delete should return error")
	}

	// Delete non-existent should fail
	err = store.Delete("int-test-sb")
	if err == nil {
		t.Fatal("Delete non-existent should return error")
	}
}

func TestEtcdStore_Integration_SessionCRUD(t *testing.T) {
	endpoints := etcdEndpoints(t)

	prefix := "/sandboxmatrix-test-session-crud/"
	store, err := NewEtcdStoreWithPrefix(endpoints, prefix)
	if err != nil {
		t.Fatalf("NewEtcdStoreWithPrefix: %v", err)
	}
	defer store.Close()

	now := time.Now().Truncate(time.Millisecond)
	sess := &v1alpha1.Session{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "v1alpha1", Kind: "Session"},
		Metadata: v1alpha1.ObjectMeta{Name: "int-sess-1"},
		Sandbox:  "my-sandbox",
		State:    v1alpha1.SessionStateActive,
		StartedAt: &now,
	}

	// SaveSession
	if err := store.SaveSession(sess); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	// GetSession
	got, err := store.GetSession("int-sess-1")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.Sandbox != "my-sandbox" {
		t.Errorf("sandbox = %q, want %q", got.Sandbox, "my-sandbox")
	}

	// Save another session for a different sandbox
	sess2 := &v1alpha1.Session{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "v1alpha1", Kind: "Session"},
		Metadata: v1alpha1.ObjectMeta{Name: "int-sess-2"},
		Sandbox:  "other-sandbox",
		State:    v1alpha1.SessionStateActive,
	}
	if err := store.SaveSession(sess2); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	// ListSessions
	all, err := store.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(all) < 2 {
		t.Fatalf("ListSessions returned %d, want >= 2", len(all))
	}

	// ListSessionsBySandbox
	filtered, err := store.ListSessionsBySandbox("my-sandbox")
	if err != nil {
		t.Fatalf("ListSessionsBySandbox: %v", err)
	}
	if len(filtered) != 1 {
		t.Fatalf("ListSessionsBySandbox returned %d, want 1", len(filtered))
	}
	if filtered[0].Metadata.Name != "int-sess-1" {
		t.Errorf("filtered session name = %q, want %q", filtered[0].Metadata.Name, "int-sess-1")
	}

	// DeleteSession
	if err := store.DeleteSession("int-sess-1"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if err := store.DeleteSession("int-sess-2"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	// DeleteSession non-existent
	if err := store.DeleteSession("int-sess-1"); err == nil {
		t.Fatal("DeleteSession non-existent should return error")
	}
}

func TestEtcdStore_Integration_MatrixCRUD(t *testing.T) {
	endpoints := etcdEndpoints(t)

	prefix := "/sandboxmatrix-test-matrix-crud/"
	store, err := NewEtcdStoreWithPrefix(endpoints, prefix)
	if err != nil {
		t.Fatalf("NewEtcdStoreWithPrefix: %v", err)
	}
	defer store.Close()

	ms := store.MatrixStore()

	mat := &v1alpha1.Matrix{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "v1alpha1", Kind: "Matrix"},
		Metadata: v1alpha1.ObjectMeta{Name: "int-matrix-1"},
		Members: []v1alpha1.MatrixMember{
			{Name: "sb-a", Blueprint: "python"},
		},
		State: v1alpha1.MatrixStateActive,
	}

	// Save
	if err := ms.Save(mat); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Get
	got, err := ms.Get("int-matrix-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Metadata.Name != "int-matrix-1" {
		t.Errorf("name = %q, want %q", got.Metadata.Name, "int-matrix-1")
	}
	if len(got.Members) != 1 {
		t.Fatalf("members len = %d, want 1", len(got.Members))
	}

	// List
	list, err := ms.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) < 1 {
		t.Fatalf("List returned %d, want >= 1", len(list))
	}

	// Delete
	if err := ms.Delete("int-matrix-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Get after delete
	_, err = ms.Get("int-matrix-1")
	if err == nil {
		t.Fatal("Get after Delete should return error")
	}

	// Delete non-existent
	if err := ms.Delete("int-matrix-1"); err == nil {
		t.Fatal("Delete non-existent should return error")
	}
}
