package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/hg-dendi/sandboxmatrix/internal/controller"
	"github.com/hg-dendi/sandboxmatrix/internal/runtime"
	"github.com/hg-dendi/sandboxmatrix/internal/state"
	v1alpha1 "github.com/hg-dendi/sandboxmatrix/pkg/api/v1alpha1"
)

// --------------------------------------------------------------------
// Mock runtime
// --------------------------------------------------------------------

type mockRuntime struct {
	containers map[string]*mockContainer
	nextID     int
}

type mockContainer struct {
	id    string
	name  string
	image string
	state string
}

func newMockRuntime() *mockRuntime {
	return &mockRuntime{
		containers: make(map[string]*mockContainer),
	}
}

func (m *mockRuntime) Name() string { return "mock" }

func (m *mockRuntime) Create(_ context.Context, cfg *runtime.CreateConfig) (string, error) {
	m.nextID++
	id := "mock-" + cfg.Name
	m.containers[id] = &mockContainer{
		id:    id,
		name:  cfg.Name,
		image: cfg.Image,
		state: "created",
	}
	return id, nil
}

func (m *mockRuntime) Start(_ context.Context, id string) error {
	if c, ok := m.containers[id]; ok {
		c.state = "running"
	}
	return nil
}

func (m *mockRuntime) Stop(_ context.Context, id string) error {
	if c, ok := m.containers[id]; ok {
		c.state = "stopped"
	}
	return nil
}

func (m *mockRuntime) Destroy(_ context.Context, id string) error {
	delete(m.containers, id)
	return nil
}

func (m *mockRuntime) Exec(_ context.Context, _ string, cfg *runtime.ExecConfig) (runtime.ExecResult, error) {
	if cfg.Stdout != nil {
		_, _ = cfg.Stdout.Write([]byte("mock output"))
	}
	return runtime.ExecResult{ExitCode: 0}, nil
}

func (m *mockRuntime) Info(_ context.Context, id string) (runtime.Info, error) {
	return runtime.Info{ID: id, State: "running", IP: "172.17.0.2"}, nil
}

func (m *mockRuntime) Stats(_ context.Context, _ string) (runtime.Stats, error) {
	return runtime.Stats{CPUUsage: 1.5, MemoryUsage: 1024 * 1024 * 50, MemoryLimit: 1024 * 1024 * 256}, nil
}

func (m *mockRuntime) List(_ context.Context) ([]runtime.Info, error) { return nil, nil }

func (m *mockRuntime) Snapshot(_ context.Context, _, tag string) (string, error) {
	return "snap-" + tag, nil
}

func (m *mockRuntime) Restore(_ context.Context, _ string, cfg *runtime.CreateConfig) (string, error) {
	return m.Create(context.Background(), cfg)
}

func (m *mockRuntime) ListSnapshots(_ context.Context, _ string) ([]runtime.SnapshotInfo, error) {
	return []runtime.SnapshotInfo{}, nil
}

func (m *mockRuntime) DeleteSnapshot(_ context.Context, _ string) error { return nil }

func (m *mockRuntime) CreateNetwork(_ context.Context, _ string, _ bool) error { return nil }

func (m *mockRuntime) DeleteNetwork(_ context.Context, _ string) error { return nil }

// --------------------------------------------------------------------
// In-memory session store for tests (the state package doesn't export one).
// --------------------------------------------------------------------

type memorySessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*v1alpha1.Session
}

func newMemorySessionStore() *memorySessionStore {
	return &memorySessionStore{sessions: make(map[string]*v1alpha1.Session)}
}

func (s *memorySessionStore) GetSession(id string) (*v1alpha1.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[id]
	if !ok {
		return nil, fmt.Errorf("session %q not found", id)
	}
	cp := *sess
	return &cp, nil
}

func (s *memorySessionStore) ListSessions() ([]*v1alpha1.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*v1alpha1.Session, 0, len(s.sessions))
	for _, sess := range s.sessions {
		cp := *sess
		result = append(result, &cp)
	}
	return result, nil
}

func (s *memorySessionStore) ListSessionsBySandbox(sandboxName string) ([]*v1alpha1.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*v1alpha1.Session
	for _, sess := range s.sessions {
		if sess.Sandbox == sandboxName {
			cp := *sess
			result = append(result, &cp)
		}
	}
	return result, nil
}

func (s *memorySessionStore) SaveSession(sess *v1alpha1.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *sess
	s.sessions[sess.Metadata.Name] = &cp
	return nil
}

func (s *memorySessionStore) DeleteSession(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sessions[id]; !ok {
		return fmt.Errorf("session %q not found", id)
	}
	delete(s.sessions, id)
	return nil
}

// --------------------------------------------------------------------
// Test helpers
// --------------------------------------------------------------------

// setupTestServer creates a test server backed by a mock runtime and
// in-memory stores.
func setupTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	rt := newMockRuntime()
	store := state.NewMemoryStore()
	sessions := newMemorySessionStore()
	matrices := state.NewMemoryMatrixStore()

	ctrl := controller.New(rt, store, sessions, matrices)
	srv := New(ctrl, ":0")
	return httptest.NewServer(srv.Handler())
}

// setupTestServerFull returns the test server plus the underlying stores
// so tests can pre-populate data.
func setupTestServerFull(t *testing.T) (*httptest.Server, *mockRuntime, *state.MemoryStore, *memorySessionStore, *state.MemoryMatrixStore) {
	t.Helper()

	rt := newMockRuntime()
	store := state.NewMemoryStore()
	sessions := newMemorySessionStore()
	matrices := state.NewMemoryMatrixStore()

	ctrl := controller.New(rt, store, sessions, matrices)
	srv := New(ctrl, ":0")
	ts := httptest.NewServer(srv.Handler())
	return ts, rt, store, sessions, matrices
}

// --------------------------------------------------------------------
// Tests
// --------------------------------------------------------------------

func TestHealthEndpoint(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/health")
	if err != nil {
		t.Fatalf("GET /api/v1/health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("expected status=ok, got %q", body["status"])
	}
}

func TestVersionEndpoint(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/version")
	if err != nil {
		t.Fatalf("GET /api/v1/version: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, ok := body["version"]; !ok {
		t.Error("expected 'version' key in response")
	}
	if _, ok := body["goVersion"]; !ok {
		t.Error("expected 'goVersion' key in response")
	}
}

func TestListSandboxesEmpty(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/sandboxes")
	if err != nil {
		t.Fatalf("GET /api/v1/sandboxes: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var sandboxes []v1alpha1.Sandbox
	if err := json.NewDecoder(resp.Body).Decode(&sandboxes); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(sandboxes) != 0 {
		t.Errorf("expected 0 sandboxes, got %d", len(sandboxes))
	}
}

func TestGetSandboxNotFound(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/sandboxes/nonexistent")
	if err != nil {
		t.Fatalf("GET /api/v1/sandboxes/nonexistent: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", resp.StatusCode)
	}
}

func TestCreateSandboxMissingFields(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	// Missing name.
	body := `{"blueprint": "test.yaml"}`
	resp, err := http.Post(ts.URL+"/api/v1/sandboxes", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST /api/v1/sandboxes: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400 for missing name, got %d", resp.StatusCode)
	}

	// Missing blueprint.
	body = `{"name": "test"}`
	resp2, err := http.Post(ts.URL+"/api/v1/sandboxes", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST /api/v1/sandboxes: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400 for missing blueprint, got %d", resp2.StatusCode)
	}
}

func TestCORSHeaders(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/health")
	if err != nil {
		t.Fatalf("GET /api/v1/health: %v", err)
	}
	defer resp.Body.Close()

	origin := resp.Header.Get("Access-Control-Allow-Origin")
	if origin != "*" {
		t.Errorf("expected CORS origin *, got %q", origin)
	}
}

func TestCORSPreflight(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodOptions, ts.URL+"/api/v1/sandboxes", http.NoBody)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("OPTIONS /api/v1/sandboxes: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected status 204 for OPTIONS preflight, got %d", resp.StatusCode)
	}
	methods := resp.Header.Get("Access-Control-Allow-Methods")
	if methods == "" {
		t.Error("expected Access-Control-Allow-Methods header to be set")
	}
}

func TestJSONContentType(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/health")
	if err != nil {
		t.Fatalf("GET /api/v1/health: %v", err)
	}
	defer resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}
}

func TestListSessions(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/sessions")
	if err != nil {
		t.Fatalf("GET /api/v1/sessions: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var sessions []v1alpha1.Session
	if err := json.NewDecoder(resp.Body).Decode(&sessions); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestListMatrices(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/matrices")
	if err != nil {
		t.Fatalf("GET /api/v1/matrices: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var matrices []v1alpha1.Matrix
	if err := json.NewDecoder(resp.Body).Decode(&matrices); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(matrices) != 0 {
		t.Errorf("expected 0 matrices, got %d", len(matrices))
	}
}

func TestSandboxGetAfterDirectStoreInsert(t *testing.T) {
	ts, _, store, _, _ := setupTestServerFull(t)
	defer ts.Close()

	now := time.Now()
	sb := &v1alpha1.Sandbox{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Sandbox"},
		Metadata: v1alpha1.ObjectMeta{
			Name:      "test-sb",
			CreatedAt: now,
			UpdatedAt: now,
		},
		Spec: v1alpha1.SandboxSpec{BlueprintRef: "test-bp"},
		Status: v1alpha1.SandboxStatus{
			State:     v1alpha1.SandboxStateRunning,
			RuntimeID: "mock-123",
		},
	}
	if err := store.Save(sb); err != nil {
		t.Fatalf("save sandbox: %v", err)
	}

	// GET specific sandbox.
	resp, err := http.Get(ts.URL + "/api/v1/sandboxes/test-sb")
	if err != nil {
		t.Fatalf("GET sandbox: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	var got v1alpha1.Sandbox
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Metadata.Name != "test-sb" {
		t.Errorf("expected name test-sb, got %q", got.Metadata.Name)
	}
	if got.Status.State != v1alpha1.SandboxStateRunning {
		t.Errorf("expected Running, got %s", got.Status.State)
	}

	// LIST sandboxes.
	resp2, err := http.Get(ts.URL + "/api/v1/sandboxes")
	if err != nil {
		t.Fatalf("GET sandboxes: %v", err)
	}
	defer resp2.Body.Close()

	var sandboxes []v1alpha1.Sandbox
	if err := json.NewDecoder(resp2.Body).Decode(&sandboxes); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(sandboxes) != 1 {
		t.Errorf("expected 1 sandbox, got %d", len(sandboxes))
	}
}

func TestExecSandbox(t *testing.T) {
	ts, _, store, _, _ := setupTestServerFull(t)
	defer ts.Close()

	now := time.Now()
	sb := &v1alpha1.Sandbox{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Sandbox"},
		Metadata: v1alpha1.ObjectMeta{Name: "exec-sb", CreatedAt: now, UpdatedAt: now},
		Spec:     v1alpha1.SandboxSpec{BlueprintRef: "test-bp"},
		Status:   v1alpha1.SandboxStatus{State: v1alpha1.SandboxStateRunning, RuntimeID: "mock-exec"},
	}
	if err := store.Save(sb); err != nil {
		t.Fatalf("save sandbox: %v", err)
	}

	body := `{"command": ["echo", "hello"]}`
	resp, err := http.Post(ts.URL+"/api/v1/sandboxes/exec-sb/exec", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST exec: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, bodyBytes)
	}

	var result execResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}
	if result.Stdout != "mock output" {
		t.Errorf("expected stdout 'mock output', got %q", result.Stdout)
	}
}

func TestDeleteSandbox(t *testing.T) {
	ts, rt, store, _, _ := setupTestServerFull(t)
	defer ts.Close()

	now := time.Now()
	sb := &v1alpha1.Sandbox{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Sandbox"},
		Metadata: v1alpha1.ObjectMeta{Name: "del-sb", CreatedAt: now, UpdatedAt: now},
		Spec:     v1alpha1.SandboxSpec{BlueprintRef: "test-bp"},
		Status:   v1alpha1.SandboxStatus{State: v1alpha1.SandboxStateRunning, RuntimeID: "mock-del"},
	}
	if err := store.Save(sb); err != nil {
		t.Fatalf("save sandbox: %v", err)
	}
	rt.containers["mock-del"] = &mockContainer{id: "mock-del", state: "running"}

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/sandboxes/del-sb", http.NoBody)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE sandbox: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, bodyBytes)
	}

	// Verify it's gone.
	resp2, err := http.Get(ts.URL + "/api/v1/sandboxes/del-sb")
	if err != nil {
		t.Fatalf("GET deleted sandbox: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", resp2.StatusCode)
	}
}
