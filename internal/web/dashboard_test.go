package web

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

// ---------------------------------------------------------------------------
// Mock runtime (implements runtime.Runtime)
// ---------------------------------------------------------------------------

type dashMockContainer struct {
	id    string
	name  string
	state string
}

type dashMockRuntime struct {
	mu         sync.Mutex
	containers map[string]*dashMockContainer
	nextID     int
}

func newDashMockRuntime() *dashMockRuntime {
	return &dashMockRuntime{containers: make(map[string]*dashMockContainer)}
}

func (m *dashMockRuntime) Name() string { return "mock" }

func (m *dashMockRuntime) Create(_ context.Context, cfg *runtime.CreateConfig) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	id := fmt.Sprintf("mock-%d", m.nextID)
	m.containers[id] = &dashMockContainer{id: id, name: cfg.Name, state: "created"}
	return id, nil
}

func (m *dashMockRuntime) Start(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok := m.containers[id]; ok {
		c.state = "running"
	}
	return nil
}

func (m *dashMockRuntime) Stop(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok := m.containers[id]; ok {
		c.state = "stopped"
	}
	return nil
}

func (m *dashMockRuntime) Destroy(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.containers, id)
	return nil
}

func (m *dashMockRuntime) Exec(_ context.Context, _ string, cfg *runtime.ExecConfig) (runtime.ExecResult, error) {
	if cfg.Stdout != nil {
		_, _ = cfg.Stdout.Write([]byte("ok"))
	}
	return runtime.ExecResult{ExitCode: 0}, nil
}

func (m *dashMockRuntime) Info(_ context.Context, id string) (runtime.Info, error) {
	return runtime.Info{ID: id, State: "running", IP: "172.17.0.2"}, nil
}

func (m *dashMockRuntime) Stats(_ context.Context, _ string) (runtime.Stats, error) {
	return runtime.Stats{CPUUsage: 1.0, MemoryUsage: 1024, MemoryLimit: 2048}, nil
}

func (m *dashMockRuntime) List(_ context.Context) ([]runtime.Info, error) { return nil, nil }

func (m *dashMockRuntime) Snapshot(_ context.Context, _, tag string) (string, error) {
	return "snap-" + tag, nil
}

func (m *dashMockRuntime) Restore(_ context.Context, _ string, cfg *runtime.CreateConfig) (string, error) {
	return m.Create(context.Background(), cfg)
}

func (m *dashMockRuntime) ListSnapshots(_ context.Context, _ string) ([]runtime.SnapshotInfo, error) {
	return nil, nil
}

func (m *dashMockRuntime) DeleteSnapshot(_ context.Context, _ string) error { return nil }

func (m *dashMockRuntime) CreateNetwork(_ context.Context, _ string, _ bool) error { return nil }

func (m *dashMockRuntime) DeleteNetwork(_ context.Context, _ string) error { return nil }

func (m *dashMockRuntime) CopyToContainer(_ context.Context, _ string, _ string, _ io.Reader) error {
	return nil
}

func (m *dashMockRuntime) CopyFromContainer(_ context.Context, _ string, _ string) (io.ReadCloser, error) {
	return nil, nil
}

// ---------------------------------------------------------------------------
// In-memory session store for dashboard tests.
// ---------------------------------------------------------------------------

type dashSessionStore struct {
	mu       sync.Mutex
	sessions map[string]*v1alpha1.Session
}

func newDashSessionStore() *dashSessionStore {
	return &dashSessionStore{sessions: make(map[string]*v1alpha1.Session)}
}

func (s *dashSessionStore) GetSession(id string) (*v1alpha1.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return nil, fmt.Errorf("session %q not found", id)
	}
	cp := *sess
	return &cp, nil
}

func (s *dashSessionStore) ListSessions() ([]*v1alpha1.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]*v1alpha1.Session, 0, len(s.sessions))
	for _, sess := range s.sessions {
		cp := *sess
		result = append(result, &cp)
	}
	return result, nil
}

func (s *dashSessionStore) ListSessionsBySandbox(sandbox string) ([]*v1alpha1.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []*v1alpha1.Session
	for _, sess := range s.sessions {
		if sess.Sandbox == sandbox {
			cp := *sess
			result = append(result, &cp)
		}
	}
	return result, nil
}

func (s *dashSessionStore) SaveSession(sess *v1alpha1.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *sess
	s.sessions[sess.Metadata.Name] = &cp
	return nil
}

func (s *dashSessionStore) DeleteSession(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sessions[id]; !ok {
		return fmt.Errorf("session %q not found", id)
	}
	delete(s.sessions, id)
	return nil
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// setupDashboardMux creates a Dashboard backed by a mock runtime with all
// in-memory stores and returns the HTTP mux, the sandbox store, and the
// matrix store so tests can pre-populate data.
func setupDashboardMux(t *testing.T) (*http.ServeMux, *state.MemoryStore, *state.MemoryMatrixStore, *dashMockRuntime) {
	t.Helper()
	rt := newDashMockRuntime()
	store := state.NewMemoryStore()
	sessions := newDashSessionStore()
	matrices := state.NewMemoryMatrixStore()
	ctrl := controller.New(rt, store, sessions, matrices)
	d := &Dashboard{ctrl: ctrl}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/dashboard/sandboxes", d.handleListSandboxes)
	mux.HandleFunc("GET /api/dashboard/matrices", d.handleListMatrices)
	mux.HandleFunc("GET /api/dashboard/sessions", d.handleListSessions)
	mux.HandleFunc("POST /api/dashboard/sandboxes/{name}/stop", d.handleStopSandbox)
	mux.HandleFunc("POST /api/dashboard/sandboxes/{name}/start", d.handleStartSandbox)
	mux.HandleFunc("DELETE /api/dashboard/sandboxes/{name}", d.handleDestroySandbox)
	mux.HandleFunc("POST /api/dashboard/sandboxes/{name}/exec", d.handleExecREST)

	return mux, store, matrices, rt
}

// setupDashboardMuxNilStores creates a Dashboard with nil session/matrix
// stores to test the "not configured" code paths.
func setupDashboardMuxNilStores(t *testing.T) *http.ServeMux {
	t.Helper()
	rt := newDashMockRuntime()
	store := state.NewMemoryStore()
	ctrl := controller.New(rt, store, nil, nil)
	d := &Dashboard{ctrl: ctrl}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/dashboard/sandboxes", d.handleListSandboxes)
	mux.HandleFunc("GET /api/dashboard/matrices", d.handleListMatrices)
	mux.HandleFunc("GET /api/dashboard/sessions", d.handleListSessions)

	return mux
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestTruncateID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"short", "short"},
		{"exactly12ch", "exactly12ch"},
		{"sha256:abcdef1234567890", "sha256:abcde"},
		{"123456789012", "123456789012"},
		{"1234567890123", "123456789012"},
	}
	for _, tt := range tests {
		got := truncateID(tt.input)
		if got != tt.want {
			t.Errorf("truncateID(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, map[string]string{"hello": "world"})

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["hello"] != "world" {
		t.Errorf("expected hello=world, got %v", body)
	}
}

func TestWriteJSONErrorStatus(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad"})

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestListSandboxesEmpty(t *testing.T) {
	mux, _, _, _ := setupDashboardMux(t)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/dashboard/sandboxes")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var sandboxes []sandboxJSON
	if err := json.NewDecoder(resp.Body).Decode(&sandboxes); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(sandboxes) != 0 {
		t.Errorf("expected 0 sandboxes, got %d", len(sandboxes))
	}
}

func TestListSandboxesWithData(t *testing.T) {
	mux, store, _, _ := setupDashboardMux(t)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	now := time.Now()
	startedAt := now.Add(-time.Minute)
	sb := &v1alpha1.Sandbox{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Sandbox"},
		Metadata: v1alpha1.ObjectMeta{
			Name:      "test-sb",
			CreatedAt: now,
			UpdatedAt: now,
			Labels:    map[string]string{"env": "test"},
		},
		Spec: v1alpha1.SandboxSpec{
			BlueprintRef: "my-bp",
			Resources:    v1alpha1.Resources{CPU: "2", Memory: "1Gi"},
		},
		Status: v1alpha1.SandboxStatus{
			State:     v1alpha1.SandboxStateRunning,
			RuntimeID: "sha256:abcdef1234567890deadbeef",
			IP:        "172.17.0.5",
			StartedAt: &startedAt,
		},
	}
	if err := store.Save(sb); err != nil {
		t.Fatalf("save sandbox: %v", err)
	}

	resp, err := http.Get(ts.URL + "/api/dashboard/sandboxes")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var sandboxes []sandboxJSON
	if err := json.NewDecoder(resp.Body).Decode(&sandboxes); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(sandboxes) != 1 {
		t.Fatalf("expected 1 sandbox, got %d", len(sandboxes))
	}
	got := sandboxes[0]
	if got.Name != "test-sb" {
		t.Errorf("expected name test-sb, got %q", got.Name)
	}
	if got.State != "Running" {
		t.Errorf("expected state Running, got %q", got.State)
	}
	if got.Blueprint != "my-bp" {
		t.Errorf("expected blueprint my-bp, got %q", got.Blueprint)
	}
	if got.RuntimeID != "sha256:abcde" {
		t.Errorf("expected truncated runtimeID sha256:abcde, got %q", got.RuntimeID)
	}
	if got.IP != "172.17.0.5" {
		t.Errorf("expected IP 172.17.0.5, got %q", got.IP)
	}
	if got.Labels["env"] != "test" {
		t.Errorf("expected label env=test, got %v", got.Labels)
	}
	if got.Resources.CPU != "2" {
		t.Errorf("expected CPU 2, got %q", got.Resources.CPU)
	}
}

func TestListMatricesNotConfigured(t *testing.T) {
	mux := setupDashboardMuxNilStores(t)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/dashboard/matrices")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 (empty array), got %d", resp.StatusCode)
	}

	var matrices []matrixJSON
	if err := json.NewDecoder(resp.Body).Decode(&matrices); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(matrices) != 0 {
		t.Errorf("expected 0 matrices, got %d", len(matrices))
	}
}

func TestListMatricesWithData(t *testing.T) {
	mux, _, matrixStore, _ := setupDashboardMux(t)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	now := time.Now()
	mx := &v1alpha1.Matrix{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Matrix"},
		Metadata: v1alpha1.ObjectMeta{Name: "test-mx", CreatedAt: now, UpdatedAt: now},
		Members: []v1alpha1.MatrixMember{
			{Name: "w1", Blueprint: "bp1"},
			{Name: "w2", Blueprint: "bp2"},
		},
		State: v1alpha1.MatrixStateActive,
	}
	if err := matrixStore.Save(mx); err != nil {
		t.Fatalf("save matrix: %v", err)
	}

	resp, err := http.Get(ts.URL + "/api/dashboard/matrices")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var matrices []matrixJSON
	if err := json.NewDecoder(resp.Body).Decode(&matrices); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(matrices) != 1 {
		t.Fatalf("expected 1 matrix, got %d", len(matrices))
	}
	if matrices[0].Name != "test-mx" {
		t.Errorf("expected name test-mx, got %q", matrices[0].Name)
	}
	if matrices[0].State != "Active" {
		t.Errorf("expected state Active, got %q", matrices[0].State)
	}
	if len(matrices[0].Members) != 2 {
		t.Errorf("expected 2 members, got %d", len(matrices[0].Members))
	}
}

func TestListSessionsNotConfigured(t *testing.T) {
	mux := setupDashboardMuxNilStores(t)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/dashboard/sessions")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 (empty array), got %d", resp.StatusCode)
	}

	var sessions []sessionJSON
	if err := json.NewDecoder(resp.Body).Decode(&sessions); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestListSessionsWithData(t *testing.T) {
	mux, store, _, _ := setupDashboardMux(t)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	// The controller's ListSessions requires the sandbox to exist for
	// filtered queries, but an unfiltered list just reads the session store.
	// We need a running sandbox so we can start a session through the controller.
	now := time.Now()
	sb := &v1alpha1.Sandbox{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Sandbox"},
		Metadata: v1alpha1.ObjectMeta{Name: "sess-sb", CreatedAt: now, UpdatedAt: now},
		Spec:     v1alpha1.SandboxSpec{BlueprintRef: "bp"},
		Status:   v1alpha1.SandboxStatus{State: v1alpha1.SandboxStateRunning, RuntimeID: "mock-sess"},
	}
	if err := store.Save(sb); err != nil {
		t.Fatalf("save sandbox: %v", err)
	}

	resp, err := http.Get(ts.URL + "/api/dashboard/sessions")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	// Initially empty (we didn't start a session, just have a sandbox).
	var sessions []sessionJSON
	if err := json.NewDecoder(resp.Body).Decode(&sessions); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestStopSandbox(t *testing.T) {
	mux, store, _, rt := setupDashboardMux(t)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	now := time.Now()
	sb := &v1alpha1.Sandbox{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Sandbox"},
		Metadata: v1alpha1.ObjectMeta{Name: "stop-sb", CreatedAt: now, UpdatedAt: now},
		Spec:     v1alpha1.SandboxSpec{BlueprintRef: "bp"},
		Status:   v1alpha1.SandboxStatus{State: v1alpha1.SandboxStateRunning, RuntimeID: "mock-stop-id"},
	}
	if err := store.Save(sb); err != nil {
		t.Fatalf("save sandbox: %v", err)
	}
	rt.mu.Lock()
	rt.containers["mock-stop-id"] = &dashMockContainer{id: "mock-stop-id", state: "running"}
	rt.mu.Unlock()

	resp, err := http.Post(ts.URL+"/api/dashboard/sandboxes/stop-sb/stop", "application/json", http.NoBody)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "stopped" {
		t.Errorf("expected status=stopped, got %q", body["status"])
	}
	if body["name"] != "stop-sb" {
		t.Errorf("expected name=stop-sb, got %q", body["name"])
	}
}

func TestStartSandbox(t *testing.T) {
	mux, store, _, rt := setupDashboardMux(t)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	now := time.Now()
	sb := &v1alpha1.Sandbox{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Sandbox"},
		Metadata: v1alpha1.ObjectMeta{Name: "start-sb", CreatedAt: now, UpdatedAt: now},
		Spec:     v1alpha1.SandboxSpec{BlueprintRef: "bp"},
		Status:   v1alpha1.SandboxStatus{State: v1alpha1.SandboxStateStopped, RuntimeID: "mock-start-id"},
	}
	if err := store.Save(sb); err != nil {
		t.Fatalf("save sandbox: %v", err)
	}
	rt.mu.Lock()
	rt.containers["mock-start-id"] = &dashMockContainer{id: "mock-start-id", state: "stopped"}
	rt.mu.Unlock()

	resp, err := http.Post(ts.URL+"/api/dashboard/sandboxes/start-sb/start", "application/json", http.NoBody)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "started" {
		t.Errorf("expected status=started, got %q", body["status"])
	}
}

func TestDestroySandbox(t *testing.T) {
	mux, store, _, rt := setupDashboardMux(t)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	now := time.Now()
	sb := &v1alpha1.Sandbox{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Sandbox"},
		Metadata: v1alpha1.ObjectMeta{Name: "destroy-sb", CreatedAt: now, UpdatedAt: now},
		Spec:     v1alpha1.SandboxSpec{BlueprintRef: "bp"},
		Status:   v1alpha1.SandboxStatus{State: v1alpha1.SandboxStateRunning, RuntimeID: "mock-destroy-id"},
	}
	if err := store.Save(sb); err != nil {
		t.Fatalf("save sandbox: %v", err)
	}
	rt.mu.Lock()
	rt.containers["mock-destroy-id"] = &dashMockContainer{id: "mock-destroy-id", state: "running"}
	rt.mu.Unlock()

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/dashboard/sandboxes/destroy-sb", http.NoBody)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "destroyed" {
		t.Errorf("expected status=destroyed, got %q", body["status"])
	}
}

func TestStopSandboxNotFound(t *testing.T) {
	mux, _, _, _ := setupDashboardMux(t)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/dashboard/sandboxes/nonexistent/stop", "application/json", http.NoBody)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", resp.StatusCode)
	}
}

func TestStartSandboxNotFound(t *testing.T) {
	mux, _, _, _ := setupDashboardMux(t)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/dashboard/sandboxes/nonexistent/start", "application/json", http.NoBody)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", resp.StatusCode)
	}
}

func TestDestroySandboxNotFound(t *testing.T) {
	mux, _, _, _ := setupDashboardMux(t)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/dashboard/sandboxes/nonexistent", http.NoBody)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", resp.StatusCode)
	}
}

func TestNewDashboardAndShutdown(t *testing.T) {
	rt := newDashMockRuntime()
	store := state.NewMemoryStore()
	ctrl := controller.New(rt, store, nil, nil)

	d := NewDashboard(ctrl, ":0")
	if d.ctrl != ctrl {
		t.Error("expected ctrl to be set")
	}
	if d.addr != ":0" {
		t.Errorf("expected addr :0, got %q", d.addr)
	}

	// Shutdown with nil server should be no-op.
	if err := d.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown on nil server: %v", err)
	}
}

func TestExecREST(t *testing.T) {
	mux, store, _, rt := setupDashboardMux(t)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	now := time.Now()
	sb := &v1alpha1.Sandbox{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Sandbox"},
		Metadata: v1alpha1.ObjectMeta{Name: "exec-sb", CreatedAt: now, UpdatedAt: now},
		Spec:     v1alpha1.SandboxSpec{BlueprintRef: "bp"},
		Status:   v1alpha1.SandboxStatus{State: v1alpha1.SandboxStateRunning, RuntimeID: "mock-exec-id"},
	}
	if err := store.Save(sb); err != nil {
		t.Fatalf("save sandbox: %v", err)
	}
	rt.mu.Lock()
	rt.containers["mock-exec-id"] = &dashMockContainer{id: "mock-exec-id", state: "running"}
	rt.mu.Unlock()

	body := `{"command": "echo hello"}`
	resp, err := http.Post(ts.URL+"/api/dashboard/sandboxes/exec-sb/exec", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST exec: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result["exitCode"].(float64) != 0 {
		t.Errorf("expected exitCode 0, got %v", result["exitCode"])
	}
}

func TestExecRESTMissingCommand(t *testing.T) {
	mux, store, _, rt := setupDashboardMux(t)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	now := time.Now()
	sb := &v1alpha1.Sandbox{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Sandbox"},
		Metadata: v1alpha1.ObjectMeta{Name: "exec-nocmd", CreatedAt: now, UpdatedAt: now},
		Spec:     v1alpha1.SandboxSpec{BlueprintRef: "bp"},
		Status:   v1alpha1.SandboxStatus{State: v1alpha1.SandboxStateRunning, RuntimeID: "mock-exec-nocmd"},
	}
	if err := store.Save(sb); err != nil {
		t.Fatalf("save sandbox: %v", err)
	}
	rt.mu.Lock()
	rt.containers["mock-exec-nocmd"] = &dashMockContainer{id: "mock-exec-nocmd", state: "running"}
	rt.mu.Unlock()

	body := `{"command": ""}`
	resp, err := http.Post(ts.URL+"/api/dashboard/sandboxes/exec-nocmd/exec", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST exec: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestExecRESTInvalidJSON(t *testing.T) {
	mux, _, _, _ := setupDashboardMux(t)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	body := `{invalid json`
	resp, err := http.Post(ts.URL+"/api/dashboard/sandboxes/some-sb/exec", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST exec: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestExecRESTSandboxNotFound(t *testing.T) {
	mux, _, _, _ := setupDashboardMux(t)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	body := `{"command": "echo hello"}`
	resp, err := http.Post(ts.URL+"/api/dashboard/sandboxes/nonexistent/exec", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST exec: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", resp.StatusCode)
	}
}

func TestListSandboxesMultiple(t *testing.T) {
	mux, store, _, _ := setupDashboardMux(t)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	now := time.Now()
	for i, name := range []string{"sb-a", "sb-b", "sb-c"} {
		sb := &v1alpha1.Sandbox{
			TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Sandbox"},
			Metadata: v1alpha1.ObjectMeta{Name: name, CreatedAt: now, UpdatedAt: now},
			Spec:     v1alpha1.SandboxSpec{BlueprintRef: fmt.Sprintf("bp-%d", i)},
			Status:   v1alpha1.SandboxStatus{State: v1alpha1.SandboxStateRunning, RuntimeID: fmt.Sprintf("rt-%d", i)},
		}
		if err := store.Save(sb); err != nil {
			t.Fatalf("save sandbox %s: %v", name, err)
		}
	}

	resp, err := http.Get(ts.URL + "/api/dashboard/sandboxes")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	var sandboxes []sandboxJSON
	if err := json.NewDecoder(resp.Body).Decode(&sandboxes); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(sandboxes) != 3 {
		t.Errorf("expected 3 sandboxes, got %d", len(sandboxes))
	}
}
