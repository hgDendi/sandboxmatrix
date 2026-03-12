package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/hg-dendi/sandboxmatrix/internal/agent/a2a"
	"github.com/hg-dendi/sandboxmatrix/internal/auth"
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

func (m *mockRuntime) CopyToContainer(_ context.Context, _ string, _ string, _ io.Reader) error {
	return nil
}

func (m *mockRuntime) CopyFromContainer(_ context.Context, _ string, _ string) (io.ReadCloser, error) {
	return nil, nil
}

func (m *mockRuntime) Pause(_ context.Context, _ string) error   { return nil }
func (m *mockRuntime) Unpause(_ context.Context, _ string) error { return nil }
func (m *mockRuntime) UpdateResources(_ context.Context, _ string, _ runtime.ResourceUpdate) error {
	return nil
}
func (m *mockRuntime) HostInfo(_ context.Context) (runtime.HostResources, error) {
	return runtime.HostResources{TotalCPUs: 4, TotalMemory: 16 * 1024 * 1024 * 1024}, nil
}

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

// --------------------------------------------------------------------
// Additional tests
// --------------------------------------------------------------------

func TestStartSandbox(t *testing.T) {
	ts, rt, store, _, _ := setupTestServerFull(t)
	defer ts.Close()

	now := time.Now()
	sb := &v1alpha1.Sandbox{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Sandbox"},
		Metadata: v1alpha1.ObjectMeta{Name: "start-sb", CreatedAt: now, UpdatedAt: now},
		Spec:     v1alpha1.SandboxSpec{BlueprintRef: "test-bp"},
		Status:   v1alpha1.SandboxStatus{State: v1alpha1.SandboxStateStopped, RuntimeID: "mock-start"},
	}
	if err := store.Save(sb); err != nil {
		t.Fatalf("save sandbox: %v", err)
	}
	rt.containers["mock-start"] = &mockContainer{id: "mock-start", state: "stopped"}

	resp, err := http.Post(ts.URL+"/api/v1/sandboxes/start-sb/start", "application/json", http.NoBody)
	if err != nil {
		t.Fatalf("POST start: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, bodyBytes)
	}

	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result["status"] != "started" {
		t.Errorf("expected status=started, got %q", result["status"])
	}
}

func TestStopSandbox(t *testing.T) {
	ts, rt, store, _, _ := setupTestServerFull(t)
	defer ts.Close()

	now := time.Now()
	sb := &v1alpha1.Sandbox{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Sandbox"},
		Metadata: v1alpha1.ObjectMeta{Name: "stop-sb", CreatedAt: now, UpdatedAt: now},
		Spec:     v1alpha1.SandboxSpec{BlueprintRef: "test-bp"},
		Status:   v1alpha1.SandboxStatus{State: v1alpha1.SandboxStateRunning, RuntimeID: "mock-stop"},
	}
	if err := store.Save(sb); err != nil {
		t.Fatalf("save sandbox: %v", err)
	}
	rt.containers["mock-stop"] = &mockContainer{id: "mock-stop", state: "running"}

	resp, err := http.Post(ts.URL+"/api/v1/sandboxes/stop-sb/stop", "application/json", http.NoBody)
	if err != nil {
		t.Fatalf("POST stop: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, bodyBytes)
	}

	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result["status"] != "stopped" {
		t.Errorf("expected status=stopped, got %q", result["status"])
	}
}

func TestStatsSandbox(t *testing.T) {
	ts, _, store, _, _ := setupTestServerFull(t)
	defer ts.Close()

	now := time.Now()
	sb := &v1alpha1.Sandbox{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Sandbox"},
		Metadata: v1alpha1.ObjectMeta{Name: "stats-sb", CreatedAt: now, UpdatedAt: now},
		Spec:     v1alpha1.SandboxSpec{BlueprintRef: "test-bp"},
		Status:   v1alpha1.SandboxStatus{State: v1alpha1.SandboxStateRunning, RuntimeID: "mock-stats"},
	}
	if err := store.Save(sb); err != nil {
		t.Fatalf("save sandbox: %v", err)
	}

	resp, err := http.Get(ts.URL + "/api/v1/sandboxes/stats-sb/stats")
	if err != nil {
		t.Fatalf("GET stats: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, bodyBytes)
	}

	var stats map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if stats["CPUUsage"] == nil {
		t.Error("expected CPUUsage in response")
	}
	if stats["MemoryUsage"] == nil {
		t.Error("expected MemoryUsage in response")
	}
}

func TestCreateSnapshot(t *testing.T) {
	ts, _, store, _, _ := setupTestServerFull(t)
	defer ts.Close()

	now := time.Now()
	sb := &v1alpha1.Sandbox{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Sandbox"},
		Metadata: v1alpha1.ObjectMeta{Name: "snap-sb", CreatedAt: now, UpdatedAt: now},
		Spec:     v1alpha1.SandboxSpec{BlueprintRef: "test-bp"},
		Status:   v1alpha1.SandboxStatus{State: v1alpha1.SandboxStateRunning, RuntimeID: "mock-snap"},
	}
	if err := store.Save(sb); err != nil {
		t.Fatalf("save sandbox: %v", err)
	}

	body := `{"tag": "v1"}`
	resp, err := http.Post(ts.URL+"/api/v1/sandboxes/snap-sb/snapshots", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST snapshot: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, bodyBytes)
	}

	var result createSnapshotResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.SnapshotID == "" {
		t.Error("expected non-empty snapshotId")
	}
	if result.Tag != "v1" {
		t.Errorf("expected tag=v1, got %q", result.Tag)
	}
}

func TestListSnapshotsEmpty(t *testing.T) {
	ts, _, store, _, _ := setupTestServerFull(t)
	defer ts.Close()

	now := time.Now()
	sb := &v1alpha1.Sandbox{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Sandbox"},
		Metadata: v1alpha1.ObjectMeta{Name: "snaplist-sb", CreatedAt: now, UpdatedAt: now},
		Spec:     v1alpha1.SandboxSpec{BlueprintRef: "test-bp"},
		Status:   v1alpha1.SandboxStatus{State: v1alpha1.SandboxStateRunning, RuntimeID: "mock-snaplist"},
	}
	if err := store.Save(sb); err != nil {
		t.Fatalf("save sandbox: %v", err)
	}

	resp, err := http.Get(ts.URL + "/api/v1/sandboxes/snaplist-sb/snapshots")
	if err != nil {
		t.Fatalf("GET snapshots: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, bodyBytes)
	}

	var snapshots []interface{}
	if err := json.NewDecoder(resp.Body).Decode(&snapshots); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(snapshots) != 0 {
		t.Errorf("expected 0 snapshots, got %d", len(snapshots))
	}
}

// writeTempBlueprint creates a minimal valid blueprint file in a temp directory
// and returns its path.
func writeTempBlueprint(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()
	content := fmt.Sprintf(`apiVersion: smx/v1alpha1
kind: Blueprint
metadata:
  name: %s
spec:
  base: alpine:latest
  runtime: docker
  resources:
    cpu: "1"
    memory: 512Mi
`, name)
	path := dir + "/" + name + ".yaml"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp blueprint: %v", err)
	}
	return path
}

func TestCreateMatrix(t *testing.T) {
	ts, _, _, _, _ := setupTestServerFull(t)
	defer ts.Close()

	bpPath := writeTempBlueprint(t, "test-member-bp")

	body := fmt.Sprintf(`{"name":"test-mx","members":[{"name":"worker1","blueprint":%q},{"name":"worker2","blueprint":%q}]}`, bpPath, bpPath)
	resp, err := http.Post(ts.URL+"/api/v1/matrices", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST matrix: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, bodyBytes)
	}

	var mx v1alpha1.Matrix
	if err := json.NewDecoder(resp.Body).Decode(&mx); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if mx.Metadata.Name != "test-mx" {
		t.Errorf("expected name=test-mx, got %q", mx.Metadata.Name)
	}
	if len(mx.Members) != 2 {
		t.Errorf("expected 2 members, got %d", len(mx.Members))
	}
}

func TestGetMatrixNotFound(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/matrices/nonexistent")
	if err != nil {
		t.Fatalf("GET matrix: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", resp.StatusCode)
	}
}

func TestStartStopMatrix(t *testing.T) {
	ts, rt, store, _, matrices := setupTestServerFull(t)
	defer ts.Close()

	now := time.Now()
	members := []v1alpha1.MatrixMember{
		{Name: "a", Blueprint: "bp-a"},
		{Name: "b", Blueprint: "bp-b"},
	}

	// Create the member sandboxes in the store as Running (simulating a
	// previously created matrix).
	for _, m := range members {
		sbName := "mxss-" + m.Name
		sb := &v1alpha1.Sandbox{
			TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Sandbox"},
			Metadata: v1alpha1.ObjectMeta{Name: sbName, CreatedAt: now, UpdatedAt: now},
			Spec:     v1alpha1.SandboxSpec{BlueprintRef: m.Blueprint},
			Status:   v1alpha1.SandboxStatus{State: v1alpha1.SandboxStateRunning, RuntimeID: "mock-" + sbName},
		}
		if err := store.Save(sb); err != nil {
			t.Fatalf("save sandbox %s: %v", sbName, err)
		}
		rt.containers["mock-"+sbName] = &mockContainer{id: "mock-" + sbName, state: "running"}
	}

	// Save the matrix as Active.
	mx := &v1alpha1.Matrix{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Matrix"},
		Metadata: v1alpha1.ObjectMeta{Name: "mxss", CreatedAt: now, UpdatedAt: now},
		Members:  members,
		State:    v1alpha1.MatrixStateActive,
	}
	if err := matrices.Save(mx); err != nil {
		t.Fatalf("save matrix: %v", err)
	}

	// Stop the matrix.
	resp, err := http.Post(ts.URL+"/api/v1/matrices/mxss/stop", "application/json", http.NoBody)
	if err != nil {
		t.Fatalf("POST stop matrix: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 on stop, got %d: %s", resp.StatusCode, bodyBytes)
	}

	// Start the matrix.
	resp2, err := http.Post(ts.URL+"/api/v1/matrices/mxss/start", "application/json", http.NoBody)
	if err != nil {
		t.Fatalf("POST start matrix: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp2.Body)
		t.Fatalf("expected 200 on start, got %d: %s", resp2.StatusCode, bodyBytes)
	}
}

func TestDestroyMatrix(t *testing.T) {
	ts, rt, store, _, matrices := setupTestServerFull(t)
	defer ts.Close()

	now := time.Now()
	members := []v1alpha1.MatrixMember{
		{Name: "x", Blueprint: "bp-x"},
	}

	sbName := "mxdel-x"
	sb := &v1alpha1.Sandbox{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Sandbox"},
		Metadata: v1alpha1.ObjectMeta{Name: sbName, CreatedAt: now, UpdatedAt: now},
		Spec:     v1alpha1.SandboxSpec{BlueprintRef: "bp-x"},
		Status:   v1alpha1.SandboxStatus{State: v1alpha1.SandboxStateRunning, RuntimeID: "mock-" + sbName},
	}
	if err := store.Save(sb); err != nil {
		t.Fatalf("save sandbox: %v", err)
	}
	rt.containers["mock-"+sbName] = &mockContainer{id: "mock-" + sbName, state: "running"}

	mx := &v1alpha1.Matrix{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Matrix"},
		Metadata: v1alpha1.ObjectMeta{Name: "mxdel", CreatedAt: now, UpdatedAt: now},
		Members:  members,
		State:    v1alpha1.MatrixStateActive,
	}
	if err := matrices.Save(mx); err != nil {
		t.Fatalf("save matrix: %v", err)
	}

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/matrices/mxdel", http.NoBody)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE matrix: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, bodyBytes)
	}

	// Verify it's gone.
	resp2, err := http.Get(ts.URL + "/api/v1/matrices/mxdel")
	if err != nil {
		t.Fatalf("GET destroyed matrix: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 after destroy, got %d", resp2.StatusCode)
	}
}

func TestStartSession(t *testing.T) {
	ts, _, store, _, _ := setupTestServerFull(t)
	defer ts.Close()

	now := time.Now()
	sb := &v1alpha1.Sandbox{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Sandbox"},
		Metadata: v1alpha1.ObjectMeta{Name: "sess-sb", CreatedAt: now, UpdatedAt: now},
		Spec:     v1alpha1.SandboxSpec{BlueprintRef: "test-bp"},
		Status:   v1alpha1.SandboxStatus{State: v1alpha1.SandboxStateRunning, RuntimeID: "mock-sess"},
	}
	if err := store.Save(sb); err != nil {
		t.Fatalf("save sandbox: %v", err)
	}

	body := `{"sandbox": "sess-sb"}`
	resp, err := http.Post(ts.URL+"/api/v1/sessions", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST session: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, bodyBytes)
	}

	var session v1alpha1.Session
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if session.Sandbox != "sess-sb" {
		t.Errorf("expected sandbox=sess-sb, got %q", session.Sandbox)
	}
	if session.State != v1alpha1.SessionStateActive {
		t.Errorf("expected state=Active, got %q", session.State)
	}
}

func TestEndSession(t *testing.T) {
	ts, _, store, _, _ := setupTestServerFull(t)
	defer ts.Close()

	now := time.Now()
	sb := &v1alpha1.Sandbox{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Sandbox"},
		Metadata: v1alpha1.ObjectMeta{Name: "endsess-sb", CreatedAt: now, UpdatedAt: now},
		Spec:     v1alpha1.SandboxSpec{BlueprintRef: "test-bp"},
		Status:   v1alpha1.SandboxStatus{State: v1alpha1.SandboxStateRunning, RuntimeID: "mock-endsess"},
	}
	if err := store.Save(sb); err != nil {
		t.Fatalf("save sandbox: %v", err)
	}

	// Start a session first.
	body := `{"sandbox": "endsess-sb"}`
	resp, err := http.Post(ts.URL+"/api/v1/sessions", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST session: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, bodyBytes)
	}

	var session v1alpha1.Session
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		t.Fatalf("decode session: %v", err)
	}
	sessionID := session.Metadata.Name

	// End the session.
	resp2, err := http.Post(ts.URL+"/api/v1/sessions/"+sessionID+"/end", "application/json", http.NoBody)
	if err != nil {
		t.Fatalf("POST end session: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp2.Body)
		t.Fatalf("expected 200, got %d: %s", resp2.StatusCode, bodyBytes)
	}

	var result map[string]string
	if err := json.NewDecoder(resp2.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result["status"] != "ended" {
		t.Errorf("expected status=ended, got %q", result["status"])
	}
}

func TestExecInSession(t *testing.T) {
	ts, _, store, _, _ := setupTestServerFull(t)
	defer ts.Close()

	now := time.Now()
	sb := &v1alpha1.Sandbox{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Sandbox"},
		Metadata: v1alpha1.ObjectMeta{Name: "execsess-sb", CreatedAt: now, UpdatedAt: now},
		Spec:     v1alpha1.SandboxSpec{BlueprintRef: "test-bp"},
		Status:   v1alpha1.SandboxStatus{State: v1alpha1.SandboxStateRunning, RuntimeID: "mock-execsess"},
	}
	if err := store.Save(sb); err != nil {
		t.Fatalf("save sandbox: %v", err)
	}

	// Start a session.
	body := `{"sandbox": "execsess-sb"}`
	resp, err := http.Post(ts.URL+"/api/v1/sessions", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST session: %v", err)
	}
	defer resp.Body.Close()

	var session v1alpha1.Session
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		t.Fatalf("decode session: %v", err)
	}
	sessionID := session.Metadata.Name

	// Exec in session.
	execBody := `{"command": ["echo", "hello"]}`
	resp2, err := http.Post(ts.URL+"/api/v1/sessions/"+sessionID+"/exec", "application/json", bytes.NewBufferString(execBody))
	if err != nil {
		t.Fatalf("POST exec in session: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp2.Body)
		t.Fatalf("expected 200, got %d: %s", resp2.StatusCode, bodyBytes)
	}

	var result execResponse
	if err := json.NewDecoder(resp2.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}
	if result.Stdout != "mock output" {
		t.Errorf("expected stdout 'mock output', got %q", result.Stdout)
	}
}

func TestExecSandboxMissingCommand(t *testing.T) {
	ts, _, store, _, _ := setupTestServerFull(t)
	defer ts.Close()

	now := time.Now()
	sb := &v1alpha1.Sandbox{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Sandbox"},
		Metadata: v1alpha1.ObjectMeta{Name: "execmiss-sb", CreatedAt: now, UpdatedAt: now},
		Spec:     v1alpha1.SandboxSpec{BlueprintRef: "test-bp"},
		Status:   v1alpha1.SandboxStatus{State: v1alpha1.SandboxStateRunning, RuntimeID: "mock-execmiss"},
	}
	if err := store.Save(sb); err != nil {
		t.Fatalf("save sandbox: %v", err)
	}

	body := `{"command": []}`
	resp, err := http.Post(ts.URL+"/api/v1/sandboxes/execmiss-sb/exec", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST exec: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestCreateSandboxInvalidJSON(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	body := `{not valid json`
	resp, err := http.Post(ts.URL+"/api/v1/sandboxes", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST sandbox: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestAuthMiddlewareNoRBAC(t *testing.T) {
	// Without RBAC configured, all requests should pass through.
	ts := setupTestServer(t)
	defer ts.Close()

	// Access a protected endpoint without any auth header.
	resp, err := http.Get(ts.URL + "/api/v1/sandboxes")
	if err != nil {
		t.Fatalf("GET sandboxes: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 without RBAC, got %d", resp.StatusCode)
	}
}

func TestAuthMiddlewareWithRBAC(t *testing.T) {
	rt := newMockRuntime()
	store := state.NewMemoryStore()
	sessions := newMemorySessionStore()
	matrices := state.NewMemoryMatrixStore()

	ctrl := controller.New(rt, store, sessions, matrices)

	rbac := auth.New()
	audit, err := auth.NewAuditLog(100, "")
	if err != nil {
		t.Fatalf("create audit log: %v", err)
	}

	// Add an admin user with a known token.
	adminToken := "test-admin-token-12345678"
	if err := rbac.AddUser(&v1alpha1.User{
		Name:  "admin-user",
		Role:  v1alpha1.RoleAdmin,
		Token: adminToken,
	}); err != nil {
		t.Fatalf("add admin user: %v", err)
	}

	// Add a viewer user with a known token.
	viewerToken := "test-viewer-token-12345678"
	if err := rbac.AddUser(&v1alpha1.User{
		Name:  "viewer-user",
		Role:  v1alpha1.RoleViewer,
		Token: viewerToken,
	}); err != nil {
		t.Fatalf("add viewer user: %v", err)
	}

	srv := New(ctrl, ":0", WithRBAC(rbac, audit))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Test 1: Request without auth header should return 401.
	resp, err := http.Get(ts.URL + "/api/v1/sandboxes")
	if err != nil {
		t.Fatalf("GET sandboxes no auth: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 without token, got %d", resp.StatusCode)
	}

	// Test 2: Request with invalid token should return 401.
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/sandboxes", http.NoBody)
	req.Header.Set("Authorization", "Bearer invalid-token")
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET sandboxes invalid token: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 with invalid token, got %d", resp2.StatusCode)
	}

	// Test 3: Admin user with valid token should get 200.
	req3, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/sandboxes", http.NoBody)
	req3.Header.Set("Authorization", "Bearer "+adminToken)
	resp3, err := http.DefaultClient.Do(req3)
	if err != nil {
		t.Fatalf("GET sandboxes admin: %v", err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for admin, got %d", resp3.StatusCode)
	}

	// Test 4: Health endpoint should work without auth even with RBAC.
	resp4, err := http.Get(ts.URL + "/api/v1/health")
	if err != nil {
		t.Fatalf("GET health: %v", err)
	}
	defer resp4.Body.Close()
	if resp4.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for health without auth, got %d", resp4.StatusCode)
	}

	// Test 5: Viewer trying to create (POST) should get 403.
	req5, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/sandboxes", bytes.NewBufferString(`{"name":"x","blueprint":"y"}`))
	req5.Header.Set("Authorization", "Bearer "+viewerToken)
	req5.Header.Set("Content-Type", "application/json")
	resp5, err := http.DefaultClient.Do(req5)
	if err != nil {
		t.Fatalf("POST sandboxes viewer: %v", err)
	}
	defer resp5.Body.Close()
	if resp5.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403 for viewer creating sandbox, got %d", resp5.StatusCode)
	}

	// Test 6: Viewer reading (GET) should get 200.
	req6, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/sandboxes", http.NoBody)
	req6.Header.Set("Authorization", "Bearer "+viewerToken)
	resp6, err := http.DefaultClient.Do(req6)
	if err != nil {
		t.Fatalf("GET sandboxes viewer: %v", err)
	}
	defer resp6.Body.Close()
	if resp6.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for viewer reading sandboxes, got %d", resp6.StatusCode)
	}
}

// --------------------------------------------------------------------
// Shard / Collect handler tests
// --------------------------------------------------------------------

// setupTestServerWithGateway creates a test server with an A2A gateway.
func setupTestServerWithGateway(t *testing.T) (*httptest.Server, *mockRuntime, *state.MemoryStore, *state.MemoryMatrixStore, *a2a.Gateway) {
	t.Helper()

	rt := newMockRuntime()
	store := state.NewMemoryStore()
	sessions := newMemorySessionStore()
	matrices := state.NewMemoryMatrixStore()
	gw := a2a.New()

	ctrl := controller.New(rt, store, sessions, matrices)
	srv := New(ctrl, ":0", WithGateway(gw))
	ts := httptest.NewServer(srv.Handler())
	return ts, rt, store, matrices, gw
}

func TestShardTask(t *testing.T) {
	ts, _, _, matrices, _ := setupTestServerWithGateway(t)
	defer ts.Close()

	now := time.Now()
	mx := &v1alpha1.Matrix{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Matrix"},
		Metadata: v1alpha1.ObjectMeta{Name: "shard-mx", CreatedAt: now, UpdatedAt: now},
		Members: []v1alpha1.MatrixMember{
			{Name: "worker1", Blueprint: "bp1"},
			{Name: "worker2", Blueprint: "bp2"},
		},
		State: v1alpha1.MatrixStateActive,
	}
	if err := matrices.Save(mx); err != nil {
		t.Fatalf("save matrix: %v", err)
	}

	body := `{"tasks":[{"id":"t1","payload":"do-something"},{"id":"t2","payload":"do-other"},{"id":"t3","payload":"do-third"}],"strategy":"round-robin"}`
	resp, err := http.Post(ts.URL+"/api/v1/matrices/shard-mx/shard", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST shard: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, bodyBytes)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}

	totalTasks := result["totalTasks"].(float64)
	if totalTasks != 3 {
		t.Errorf("expected totalTasks=3, got %v", totalTasks)
	}

	assignments := result["assignments"].([]interface{})
	if len(assignments) != 3 {
		t.Errorf("expected 3 assignments, got %d", len(assignments))
	}
}

func TestShardTaskMissingTasks(t *testing.T) {
	ts, _, _, matrices, _ := setupTestServerWithGateway(t)
	defer ts.Close()

	now := time.Now()
	mx := &v1alpha1.Matrix{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Matrix"},
		Metadata: v1alpha1.ObjectMeta{Name: "shard-empty", CreatedAt: now, UpdatedAt: now},
		Members: []v1alpha1.MatrixMember{
			{Name: "w1", Blueprint: "bp1"},
		},
		State: v1alpha1.MatrixStateActive,
	}
	if err := matrices.Save(mx); err != nil {
		t.Fatalf("save matrix: %v", err)
	}

	body := `{"tasks":[]}`
	resp, err := http.Post(ts.URL+"/api/v1/matrices/shard-empty/shard", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST shard: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestShardTaskMatrixNotFound(t *testing.T) {
	ts, _, _, _, _ := setupTestServerWithGateway(t)
	defer ts.Close()

	body := `{"tasks":[{"id":"t1","payload":"x"}]}`
	resp, err := http.Post(ts.URL+"/api/v1/matrices/nonexistent/shard", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST shard: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestShardTaskInvalidJSON(t *testing.T) {
	ts, _, _, _, _ := setupTestServerWithGateway(t)
	defer ts.Close()

	body := `{not valid json`
	resp, err := http.Post(ts.URL+"/api/v1/matrices/some-mx/shard", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST shard: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestShardTaskWithHashStrategy(t *testing.T) {
	ts, _, _, matrices, _ := setupTestServerWithGateway(t)
	defer ts.Close()

	now := time.Now()
	mx := &v1alpha1.Matrix{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Matrix"},
		Metadata: v1alpha1.ObjectMeta{Name: "hash-mx", CreatedAt: now, UpdatedAt: now},
		Members: []v1alpha1.MatrixMember{
			{Name: "w1", Blueprint: "bp1"},
			{Name: "w2", Blueprint: "bp2"},
		},
		State: v1alpha1.MatrixStateActive,
	}
	if err := matrices.Save(mx); err != nil {
		t.Fatalf("save matrix: %v", err)
	}

	body := `{"tasks":[{"id":"t1","payload":"a","key":"key1"},{"id":"t2","payload":"b","key":"key2"}],"strategy":"hash"}`
	resp, err := http.Post(ts.URL+"/api/v1/matrices/hash-mx/shard", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST shard: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, bodyBytes)
	}
}

func TestCollectResultsNoGateway(t *testing.T) {
	// Create a server WITHOUT a gateway.
	ts, _, _, _, matrices := setupTestServerFull(t)
	defer ts.Close()

	now := time.Now()
	mx := &v1alpha1.Matrix{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Matrix"},
		Metadata: v1alpha1.ObjectMeta{Name: "no-gw-mx", CreatedAt: now, UpdatedAt: now},
		Members: []v1alpha1.MatrixMember{
			{Name: "w1", Blueprint: "bp1"},
		},
		State: v1alpha1.MatrixStateActive,
	}
	if err := matrices.Save(mx); err != nil {
		t.Fatalf("save matrix: %v", err)
	}

	body := `{"taskID":"task-123"}`
	resp, err := http.Post(ts.URL+"/api/v1/matrices/no-gw-mx/collect", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST collect: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 503, got %d: %s", resp.StatusCode, bodyBytes)
	}
}

func TestCollectResultsMissingTaskID(t *testing.T) {
	ts, _, _, matrices, _ := setupTestServerWithGateway(t)
	defer ts.Close()

	now := time.Now()
	mx := &v1alpha1.Matrix{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Matrix"},
		Metadata: v1alpha1.ObjectMeta{Name: "collect-mx", CreatedAt: now, UpdatedAt: now},
		Members: []v1alpha1.MatrixMember{
			{Name: "w1", Blueprint: "bp1"},
		},
		State: v1alpha1.MatrixStateActive,
	}
	if err := matrices.Save(mx); err != nil {
		t.Fatalf("save matrix: %v", err)
	}

	body := `{"taskID":""}`
	resp, err := http.Post(ts.URL+"/api/v1/matrices/collect-mx/collect", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST collect: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestCollectResultsMatrixNotFound(t *testing.T) {
	ts, _, _, _, _ := setupTestServerWithGateway(t)
	defer ts.Close()

	body := `{"taskID":"task-123"}`
	resp, err := http.Post(ts.URL+"/api/v1/matrices/nonexistent/collect", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST collect: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestShardTaskNoGateway(t *testing.T) {
	// Even without a gateway, shard should still work (just no A2A dispatch).
	ts, _, _, _, matrices := setupTestServerFull(t)
	defer ts.Close()

	now := time.Now()
	mx := &v1alpha1.Matrix{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Matrix"},
		Metadata: v1alpha1.ObjectMeta{Name: "no-gw-shard", CreatedAt: now, UpdatedAt: now},
		Members: []v1alpha1.MatrixMember{
			{Name: "w1", Blueprint: "bp1"},
		},
		State: v1alpha1.MatrixStateActive,
	}
	if err := matrices.Save(mx); err != nil {
		t.Fatalf("save matrix: %v", err)
	}

	body := `{"tasks":[{"id":"t1","payload":"test"}]}`
	resp, err := http.Post(ts.URL+"/api/v1/matrices/no-gw-shard/shard", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST shard: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, bodyBytes)
	}
}

func TestMapRequestToPermission(t *testing.T) {
	tests := []struct {
		method  string
		path    string
		wantRes string
		wantAct string
	}{
		{http.MethodGet, "/api/v1/sandboxes", "sandbox", "read"},
		{http.MethodGet, "/api/v1/sandboxes/my-sb", "sandbox", "read"},
		{http.MethodPost, "/api/v1/sandboxes", "sandbox", "create"},
		{http.MethodDelete, "/api/v1/sandboxes/my-sb", "sandbox", "delete"},
		{http.MethodPost, "/api/v1/sandboxes/my-sb/exec", "sandbox", "exec"},
		{http.MethodPost, "/api/v1/sandboxes/my-sb/start", "sandbox", "update"},
		{http.MethodPost, "/api/v1/sandboxes/my-sb/stop", "sandbox", "update"},
		{http.MethodPost, "/api/v1/sandboxes/my-sb/snapshots", "sandbox", "create"},
		{http.MethodGet, "/api/v1/sandboxes/my-sb/snapshots", "sandbox", "read"},
		{http.MethodGet, "/api/v1/matrices", "matrix", "read"},
		{http.MethodPost, "/api/v1/matrices", "matrix", "create"},
		{http.MethodDelete, "/api/v1/matrices/my-mx", "matrix", "delete"},
		{http.MethodPost, "/api/v1/matrices/my-mx/start", "matrix", "update"},
		{http.MethodPost, "/api/v1/matrices/my-mx/stop", "matrix", "update"},
		{http.MethodGet, "/api/v1/sessions", "session", "read"},
		{http.MethodPost, "/api/v1/sessions", "session", "create"},
		{http.MethodPost, "/api/v1/sessions/my-sess/end", "session", "update"},
		{http.MethodPost, "/api/v1/sessions/my-sess/exec", "session", "exec"},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			gotRes, gotAct := mapRequestToPermission(tt.method, tt.path)
			if gotRes != tt.wantRes {
				t.Errorf("resource: got %q, want %q", gotRes, tt.wantRes)
			}
			if gotAct != tt.wantAct {
				t.Errorf("action: got %q, want %q", gotAct, tt.wantAct)
			}
		})
	}
}
