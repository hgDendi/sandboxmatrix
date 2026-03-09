package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/hg-dendi/sandboxmatrix/internal/agent/a2a"
	"github.com/hg-dendi/sandboxmatrix/internal/controller"
	"github.com/hg-dendi/sandboxmatrix/internal/runtime"
	"github.com/hg-dendi/sandboxmatrix/internal/state"
	v1alpha1 "github.com/hg-dendi/sandboxmatrix/pkg/api/v1alpha1"
	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// ---------------------------------------------------------------------------
// Mock runtime
// ---------------------------------------------------------------------------

type mockRuntime struct {
	mu         sync.Mutex
	containers map[string]runtime.Info // id -> Info
	nextID     int
}

func newMockRuntime() *mockRuntime {
	return &mockRuntime{
		containers: make(map[string]runtime.Info),
	}
}

func (m *mockRuntime) Name() string { return "mock" }

func (m *mockRuntime) Create(_ context.Context, cfg *runtime.CreateConfig) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	id := fmt.Sprintf("mock-%d", m.nextID)
	m.containers[id] = runtime.Info{
		ID:     id,
		Name:   cfg.Name,
		Image:  cfg.Image,
		State:  "created",
		Labels: cfg.Labels,
	}
	return id, nil
}

func (m *mockRuntime) Start(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	info, ok := m.containers[id]
	if !ok {
		return fmt.Errorf("container %q not found", id)
	}
	info.State = "running"
	info.IP = "172.17.0.2"
	m.containers[id] = info
	return nil
}

func (m *mockRuntime) Stop(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	info, ok := m.containers[id]
	if !ok {
		return fmt.Errorf("container %q not found", id)
	}
	info.State = "stopped"
	m.containers[id] = info
	return nil
}

func (m *mockRuntime) Destroy(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.containers[id]; !ok {
		return fmt.Errorf("container %q not found", id)
	}
	delete(m.containers, id)
	return nil
}

func (m *mockRuntime) Exec(_ context.Context, _ string, cfg *runtime.ExecConfig) (runtime.ExecResult, error) {
	output := "hello from mock"
	if cfg.Stdout != nil {
		_, _ = io.WriteString(cfg.Stdout, output)
	}
	return runtime.ExecResult{ExitCode: 0}, nil
}

func (m *mockRuntime) Info(_ context.Context, id string) (runtime.Info, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	info, ok := m.containers[id]
	if !ok {
		return runtime.Info{}, fmt.Errorf("container %q not found", id)
	}
	return info, nil
}

func (m *mockRuntime) Stats(_ context.Context, _ string) (runtime.Stats, error) {
	return runtime.Stats{
		CPUUsage:    25.5,
		MemoryUsage: 128 * 1024 * 1024, // 128 MiB
		MemoryLimit: 512 * 1024 * 1024, // 512 MiB
		DiskUsage:   1024 * 1024 * 1024,
	}, nil
}

func (m *mockRuntime) List(_ context.Context) ([]runtime.Info, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []runtime.Info
	for _, info := range m.containers {
		result = append(result, info)
	}
	return result, nil
}

func (m *mockRuntime) Snapshot(_ context.Context, id, tag string) (string, error) {
	return "snap-" + id + "-" + tag, nil
}

func (m *mockRuntime) Restore(_ context.Context, snapshotID string, cfg *runtime.CreateConfig) (string, error) {
	return m.Create(context.Background(), cfg)
}

func (m *mockRuntime) ListSnapshots(_ context.Context, _ string) ([]runtime.SnapshotInfo, error) {
	return nil, nil
}

func (m *mockRuntime) DeleteSnapshot(_ context.Context, _ string) error {
	return nil
}

func (m *mockRuntime) CreateNetwork(_ context.Context, _ string, _ bool) error {
	return nil
}

func (m *mockRuntime) DeleteNetwork(_ context.Context, _ string) error {
	return nil
}

// ---------------------------------------------------------------------------
// In-memory session store
// ---------------------------------------------------------------------------

type memSessionStore struct {
	mu       sync.Mutex
	sessions map[string]*v1alpha1.Session
}

func newMemSessionStore() *memSessionStore {
	return &memSessionStore{sessions: make(map[string]*v1alpha1.Session)}
}

func (s *memSessionStore) GetSession(id string) (*v1alpha1.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return nil, fmt.Errorf("session %q not found", id)
	}
	cp := *sess
	return &cp, nil
}

func (s *memSessionStore) ListSessions() ([]*v1alpha1.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]*v1alpha1.Session, 0, len(s.sessions))
	for _, sess := range s.sessions {
		cp := *sess
		result = append(result, &cp)
	}
	return result, nil
}

func (s *memSessionStore) ListSessionsBySandbox(sandbox string) ([]*v1alpha1.Session, error) {
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

func (s *memSessionStore) SaveSession(sess *v1alpha1.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *sess
	s.sessions[sess.Metadata.Name] = &cp
	return nil
}

func (s *memSessionStore) DeleteSession(id string) error {
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

// writeTempBlueprint creates a temporary blueprint YAML file and returns its path.
// The caller should defer os.Remove(path) to clean up.
func writeTempBlueprint(t *testing.T) string {
	t.Helper()
	content := `apiVersion: smx/v1alpha1
kind: Blueprint
metadata:
  name: test-bp
  version: "1.0.0"
spec:
  base: alpine:latest
  runtime: docker
  resources:
    cpu: "1"
    memory: 512Mi
  workspace:
    mountPath: /workspace
`
	f, err := os.CreateTemp(t.TempDir(), "blueprint-*.yaml")
	if err != nil {
		t.Fatalf("creating temp blueprint: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("writing temp blueprint: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("closing temp blueprint: %v", err)
	}
	return f.Name()
}

// newTestServer creates a Server backed by a mock runtime and in-memory stores.
func newTestServer(t *testing.T) *Server {
	t.Helper()
	rt := newMockRuntime()
	store := state.NewMemoryStore()
	sessions := newMemSessionStore()
	matrices := state.NewMemoryMatrixStore()
	ctrl := controller.New(rt, store, sessions, matrices)
	gw := a2a.New()
	return NewServer(ctrl, gw)
}

// makeRequest builds a CallToolRequest with the given tool name and arguments.
func makeRequest(name string, args map[string]any) mcplib.CallToolRequest {
	return mcplib.CallToolRequest{
		Params: mcplib.CallToolParams{
			Name:      name,
			Arguments: args,
		},
	}
}

// resultText extracts the text from the first content element of a CallToolResult.
func resultText(t *testing.T, result *mcplib.CallToolResult) string {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("result has no content")
	}
	tc, ok := result.Content[0].(mcplib.TextContent)
	if !ok {
		t.Fatalf("result content is not TextContent, got %T", result.Content[0])
	}
	return tc.Text
}

// createSandbox is a test helper that creates a sandbox via the handler and
// returns the result text. It fatals on error.
func createSandbox(t *testing.T, s *Server, name, bpPath string) string {
	t.Helper()
	req := makeRequest("sandbox_create", map[string]any{
		"name":      name,
		"blueprint": bpPath,
	})
	result, err := s.handleSandboxCreate(context.Background(), req)
	if err != nil {
		t.Fatalf("handleSandboxCreate returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleSandboxCreate returned tool error: %s", resultText(t, result))
	}
	return resultText(t, result)
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestNewServer(t *testing.T) {
	s := newTestServer(t)

	if s == nil {
		t.Fatal("NewServer returned nil")
	}
	if s.mcpServer == nil {
		t.Fatal("mcpServer field is nil")
	}
	if s.ctrl == nil {
		t.Fatal("ctrl field is nil")
	}
	if s.gateway == nil {
		t.Fatal("gateway field is nil")
	}
}

func TestHandleSandboxCreate(t *testing.T) {
	s := newTestServer(t)
	bpPath := writeTempBlueprint(t)

	text := createSandbox(t, s, "test-sb", bpPath)

	// Result should be valid JSON with name, state, and runtimeID.
	var got map[string]string
	if err := json.Unmarshal([]byte(text), &got); err != nil {
		t.Fatalf("result is not valid JSON: %v\ntext: %s", err, text)
	}

	if got["name"] != "test-sb" {
		t.Errorf("expected name %q, got %q", "test-sb", got["name"])
	}
	if got["state"] != string(v1alpha1.SandboxStateRunning) {
		t.Errorf("expected state %q, got %q", v1alpha1.SandboxStateRunning, got["state"])
	}
	if got["runtimeID"] == "" {
		t.Error("expected non-empty runtimeID")
	}
}

func TestHandleSandboxCreate_MissingParams(t *testing.T) {
	s := newTestServer(t)
	bpPath := writeTempBlueprint(t)

	tests := []struct {
		desc string
		args map[string]any
		want string
	}{
		{
			desc: "missing name",
			args: map[string]any{"blueprint": bpPath},
			want: "parameter 'name' is required",
		},
		{
			desc: "missing blueprint",
			args: map[string]any{"name": "foo"},
			want: "parameter 'blueprint' is required",
		},
		{
			desc: "both missing",
			args: map[string]any{},
			want: "parameter 'name' is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			req := makeRequest("sandbox_create", tt.args)
			result, err := s.handleSandboxCreate(context.Background(), req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !result.IsError {
				t.Fatal("expected IsError to be true")
			}
			text := resultText(t, result)
			if !strings.Contains(text, tt.want) {
				t.Errorf("expected error to contain %q, got %q", tt.want, text)
			}
		})
	}
}

func TestHandleSandboxList(t *testing.T) {
	s := newTestServer(t)

	// List with no sandboxes.
	req := makeRequest("sandbox_list", nil)
	result, err := s.handleSandboxList(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", resultText(t, result))
	}
	text := resultText(t, result)
	if !strings.Contains(text, "No sandboxes found") {
		t.Errorf("expected 'No sandboxes found', got %q", text)
	}

	// Create a sandbox, then list again.
	bpPath := writeTempBlueprint(t)
	createSandbox(t, s, "list-test", bpPath)

	result, err = s.handleSandboxList(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", resultText(t, result))
	}
	text = resultText(t, result)
	if !strings.Contains(text, "list-test") {
		t.Errorf("expected sandbox 'list-test' in listing, got:\n%s", text)
	}
	if !strings.Contains(text, "Running") {
		t.Errorf("expected 'Running' state in listing, got:\n%s", text)
	}
}

func TestHandleSandboxExec(t *testing.T) {
	s := newTestServer(t)
	bpPath := writeTempBlueprint(t)
	createSandbox(t, s, "exec-test", bpPath)

	req := makeRequest("sandbox_exec", map[string]any{
		"name":    "exec-test",
		"command": "echo hello",
	})
	result, err := s.handleSandboxExec(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", resultText(t, result))
	}
	text := resultText(t, result)
	if !strings.Contains(text, "hello from mock") {
		t.Errorf("expected output to contain 'hello from mock', got %q", text)
	}
}

func TestHandleSandboxExec_MissingParams(t *testing.T) {
	s := newTestServer(t)

	tests := []struct {
		desc string
		args map[string]any
		want string
	}{
		{
			desc: "missing name",
			args: map[string]any{"command": "echo hi"},
			want: "parameter 'name' is required",
		},
		{
			desc: "missing command",
			args: map[string]any{"name": "test"},
			want: "parameter 'command' is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			req := makeRequest("sandbox_exec", tt.args)
			result, err := s.handleSandboxExec(context.Background(), req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !result.IsError {
				t.Fatal("expected IsError to be true")
			}
			text := resultText(t, result)
			if !strings.Contains(text, tt.want) {
				t.Errorf("expected error to contain %q, got %q", tt.want, text)
			}
		})
	}
}

func TestHandleSandboxStop(t *testing.T) {
	s := newTestServer(t)
	bpPath := writeTempBlueprint(t)
	createSandbox(t, s, "stop-test", bpPath)

	req := makeRequest("sandbox_stop", map[string]any{"name": "stop-test"})
	result, err := s.handleSandboxStop(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", resultText(t, result))
	}
	text := resultText(t, result)
	if !strings.Contains(text, "stopped") {
		t.Errorf("expected 'stopped' in result, got %q", text)
	}
}

func TestHandleSandboxStart(t *testing.T) {
	s := newTestServer(t)
	bpPath := writeTempBlueprint(t)
	createSandbox(t, s, "start-test", bpPath)

	// Stop the sandbox first.
	stopReq := makeRequest("sandbox_stop", map[string]any{"name": "start-test"})
	result, err := s.handleSandboxStop(context.Background(), stopReq)
	if err != nil {
		t.Fatalf("stop: unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("stop: unexpected tool error: %s", resultText(t, result))
	}

	// Now start it.
	startReq := makeRequest("sandbox_start", map[string]any{"name": "start-test"})
	result, err = s.handleSandboxStart(context.Background(), startReq)
	if err != nil {
		t.Fatalf("start: unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("start: unexpected tool error: %s", resultText(t, result))
	}
	text := resultText(t, result)
	if !strings.Contains(text, "started") {
		t.Errorf("expected 'started' in result, got %q", text)
	}
}

func TestHandleSandboxDestroy(t *testing.T) {
	s := newTestServer(t)
	bpPath := writeTempBlueprint(t)
	createSandbox(t, s, "destroy-test", bpPath)

	req := makeRequest("sandbox_destroy", map[string]any{"name": "destroy-test"})
	result, err := s.handleSandboxDestroy(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", resultText(t, result))
	}
	text := resultText(t, result)
	if !strings.Contains(text, "destroyed") {
		t.Errorf("expected 'destroyed' in result, got %q", text)
	}

	// Verify sandbox is gone by listing.
	listReq := makeRequest("sandbox_list", nil)
	result, err = s.handleSandboxList(context.Background(), listReq)
	if err != nil {
		t.Fatalf("list after destroy: unexpected error: %v", err)
	}
	listText := resultText(t, result)
	if strings.Contains(listText, "destroy-test") {
		t.Errorf("sandbox 'destroy-test' should not appear after destroy, got:\n%s", listText)
	}
}

func TestHandleSandboxStats(t *testing.T) {
	s := newTestServer(t)
	bpPath := writeTempBlueprint(t)
	createSandbox(t, s, "stats-test", bpPath)

	req := makeRequest("sandbox_stats", map[string]any{"name": "stats-test"})
	result, err := s.handleSandboxStats(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", resultText(t, result))
	}
	text := resultText(t, result)

	// The mock returns CPU 25.5%, memory 128 MiB / 512 MiB (25.0%).
	if !strings.Contains(text, "CPU") {
		t.Errorf("expected 'CPU' in stats output, got %q", text)
	}
	if !strings.Contains(text, "25.5%") {
		t.Errorf("expected '25.5%%' CPU usage in stats, got %q", text)
	}
	if !strings.Contains(text, "Memory") {
		t.Errorf("expected 'Memory' in stats output, got %q", text)
	}
	if !strings.Contains(text, "128.0 MiB") {
		t.Errorf("expected '128.0 MiB' in stats output, got %q", text)
	}
	if !strings.Contains(text, "512.0 MiB") {
		t.Errorf("expected '512.0 MiB' in stats output, got %q", text)
	}
}

func TestHandleA2ASend(t *testing.T) {
	s := newTestServer(t)

	req := makeRequest("a2a_send", map[string]any{
		"from":    "sender-sb",
		"to":      "receiver-sb",
		"type":    "request",
		"payload": `{"action":"ping"}`,
	})
	result, err := s.handleA2ASend(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", resultText(t, result))
	}
	text := resultText(t, result)
	if !strings.Contains(text, "Message sent") {
		t.Errorf("expected 'Message sent' in result, got %q", text)
	}
	if !strings.Contains(text, "sender-sb") {
		t.Errorf("expected 'sender-sb' in result, got %q", text)
	}
	if !strings.Contains(text, "receiver-sb") {
		t.Errorf("expected 'receiver-sb' in result, got %q", text)
	}
}

func TestHandleA2AReceive(t *testing.T) {
	s := newTestServer(t)

	// Receive with no messages.
	req := makeRequest("a2a_receive", map[string]any{
		"sandbox_name": "my-sb",
	})
	result, err := s.handleA2AReceive(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", resultText(t, result))
	}
	text := resultText(t, result)
	if !strings.Contains(text, "No pending messages") {
		t.Errorf("expected 'No pending messages', got %q", text)
	}

	// Send a message, then receive it.
	sendReq := makeRequest("a2a_send", map[string]any{
		"from":    "alice",
		"to":      "my-sb",
		"type":    "event",
		"payload": `{"data":"test"}`,
	})
	sendResult, err := s.handleA2ASend(context.Background(), sendReq)
	if err != nil {
		t.Fatalf("send: unexpected error: %v", err)
	}
	if sendResult.IsError {
		t.Fatalf("send: unexpected tool error: %s", resultText(t, sendResult))
	}

	// Now receive.
	result, err = s.handleA2AReceive(context.Background(), req)
	if err != nil {
		t.Fatalf("receive: unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("receive: unexpected tool error: %s", resultText(t, result))
	}
	text = resultText(t, result)

	// Result should be JSON array of messages.
	var msgs []a2a.Message
	if err := json.Unmarshal([]byte(text), &msgs); err != nil {
		t.Fatalf("result is not valid JSON array of messages: %v\ntext: %s", err, text)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].From != "alice" {
		t.Errorf("expected From %q, got %q", "alice", msgs[0].From)
	}
	if msgs[0].To != "my-sb" {
		t.Errorf("expected To %q, got %q", "my-sb", msgs[0].To)
	}
	if msgs[0].Type != "event" {
		t.Errorf("expected Type %q, got %q", "event", msgs[0].Type)
	}

	// Receive again -- inbox should be cleared.
	result, err = s.handleA2AReceive(context.Background(), req)
	if err != nil {
		t.Fatalf("receive again: unexpected error: %v", err)
	}
	text = resultText(t, result)
	if !strings.Contains(text, "No pending messages") {
		t.Errorf("expected inbox to be cleared, got %q", text)
	}
}

func TestHandleA2ABroadcast(t *testing.T) {
	s := newTestServer(t)

	req := makeRequest("a2a_broadcast", map[string]any{
		"from":    "coordinator",
		"targets": "worker-1, worker-2, worker-3",
		"type":    "task",
		"payload": `{"job":"build"}`,
	})
	result, err := s.handleA2ABroadcast(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", resultText(t, result))
	}
	text := resultText(t, result)
	if !strings.Contains(text, "Broadcast") {
		t.Errorf("expected 'Broadcast' in result, got %q", text)
	}
	if !strings.Contains(text, "3 targets") {
		t.Errorf("expected '3 targets' in result, got %q", text)
	}

	// Verify each worker received the message.
	for _, worker := range []string{"worker-1", "worker-2", "worker-3"} {
		recvReq := makeRequest("a2a_receive", map[string]any{
			"sandbox_name": worker,
		})
		recvResult, err := s.handleA2AReceive(context.Background(), recvReq)
		if err != nil {
			t.Fatalf("receive %s: unexpected error: %v", worker, err)
		}
		if recvResult.IsError {
			t.Fatalf("receive %s: unexpected tool error: %s", worker, resultText(t, recvResult))
		}
		recvText := resultText(t, recvResult)

		var msgs []a2a.Message
		if err := json.Unmarshal([]byte(recvText), &msgs); err != nil {
			t.Fatalf("receive %s: not valid JSON: %v\ntext: %s", worker, err, recvText)
		}
		if len(msgs) != 1 {
			t.Errorf("receive %s: expected 1 message, got %d", worker, len(msgs))
		}
		if len(msgs) > 0 && msgs[0].From != "coordinator" {
			t.Errorf("receive %s: expected From 'coordinator', got %q", worker, msgs[0].From)
		}
	}
}

func TestHandleSandboxReadyWait(t *testing.T) {
	// Since the mock runtime has no readiness probe configured in the blueprint,
	// the sandbox will be in Running state (not Ready). The handler polls until
	// Ready or timeout. We test by:
	// 1. Creating a sandbox (ends up in Running state).
	// 2. Manually setting its state to Ready in the store.
	// 3. Calling the handler with a short timeout.
	//
	// We do this by accessing the controller's store directly. Since the
	// controller is created in newTestServer and we need access to the store,
	// we build the server manually here.

	rt := newMockRuntime()
	store := state.NewMemoryStore()
	sessions := newMemSessionStore()
	matrices := state.NewMemoryMatrixStore()
	ctrl := controller.New(rt, store, sessions, matrices)
	gw := a2a.New()
	srv := NewServer(ctrl, gw)

	bpPath := writeTempBlueprint(t)
	createSandbox(t, srv, "ready-test", bpPath)

	// Manually set the sandbox state to Ready so the handler finds it.
	sb, err := store.Get("ready-test")
	if err != nil {
		t.Fatalf("failed to get sandbox from store: %v", err)
	}
	sb.Status.State = v1alpha1.SandboxStateReady
	if err := store.Save(sb); err != nil {
		t.Fatalf("failed to save sandbox: %v", err)
	}

	req := makeRequest("sandbox_ready_wait", map[string]any{
		"name":    "ready-test",
		"timeout": "5",
	})
	result, err := srv.handleSandboxReadyWait(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", resultText(t, result))
	}
	text := resultText(t, result)
	if !strings.Contains(text, "is ready") {
		t.Errorf("expected 'is ready' in result, got %q", text)
	}
}

func TestHandleSandboxReadyWait_Timeout(t *testing.T) {
	// Verify that the handler returns a timeout error when the sandbox
	// stays in Running state (never becomes Ready).
	s := newTestServer(t)
	bpPath := writeTempBlueprint(t)
	createSandbox(t, s, "timeout-test", bpPath)

	req := makeRequest("sandbox_ready_wait", map[string]any{
		"name":    "timeout-test",
		"timeout": "2",
	})
	result, err := s.handleSandboxReadyWait(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError to be true for timeout")
	}
	text := resultText(t, result)
	if !strings.Contains(text, "did not become ready") {
		t.Errorf("expected timeout message, got %q", text)
	}
}

func TestHandleSandboxReadyWait_ErrorState(t *testing.T) {
	// Verify that the handler returns immediately when the sandbox is in Error state.
	rt := newMockRuntime()
	store := state.NewMemoryStore()
	sessions := newMemSessionStore()
	matrices := state.NewMemoryMatrixStore()
	ctrl := controller.New(rt, store, sessions, matrices)
	gw := a2a.New()
	srv := NewServer(ctrl, gw)

	bpPath := writeTempBlueprint(t)
	createSandbox(t, srv, "error-test", bpPath)

	// Set sandbox to Error state.
	sb, err := store.Get("error-test")
	if err != nil {
		t.Fatalf("failed to get sandbox: %v", err)
	}
	sb.Status.State = v1alpha1.SandboxStateError
	sb.Status.Message = "something went wrong"
	if err := store.Save(sb); err != nil {
		t.Fatalf("failed to save sandbox: %v", err)
	}

	req := makeRequest("sandbox_ready_wait", map[string]any{
		"name":    "error-test",
		"timeout": "10",
	})
	result, err := srv.handleSandboxReadyWait(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError to be true for error state")
	}
	text := resultText(t, result)
	if !strings.Contains(text, "error state") {
		t.Errorf("expected 'error state' in result, got %q", text)
	}
}

func TestHandleA2ASend_MissingParams(t *testing.T) {
	s := newTestServer(t)

	tests := []struct {
		desc string
		args map[string]any
		want string
	}{
		{
			desc: "missing from",
			args: map[string]any{"to": "b", "type": "req", "payload": "{}"},
			want: "parameter 'from' is required",
		},
		{
			desc: "missing to",
			args: map[string]any{"from": "a", "type": "req", "payload": "{}"},
			want: "parameter 'to' is required",
		},
		{
			desc: "missing type",
			args: map[string]any{"from": "a", "to": "b", "payload": "{}"},
			want: "parameter 'type' is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			req := makeRequest("a2a_send", tt.args)
			result, err := s.handleA2ASend(context.Background(), req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !result.IsError {
				t.Fatal("expected IsError to be true")
			}
			text := resultText(t, result)
			if !strings.Contains(text, tt.want) {
				t.Errorf("expected error to contain %q, got %q", tt.want, text)
			}
		})
	}
}

func TestHandleSandboxStop_MissingName(t *testing.T) {
	s := newTestServer(t)

	req := makeRequest("sandbox_stop", map[string]any{})
	result, err := s.handleSandboxStop(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError to be true")
	}
	text := resultText(t, result)
	if !strings.Contains(text, "parameter 'name' is required") {
		t.Errorf("expected 'parameter name is required', got %q", text)
	}
}

func TestHandleSandboxStart_MissingName(t *testing.T) {
	s := newTestServer(t)

	req := makeRequest("sandbox_start", map[string]any{})
	result, err := s.handleSandboxStart(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError to be true")
	}
	text := resultText(t, result)
	if !strings.Contains(text, "parameter 'name' is required") {
		t.Errorf("expected 'parameter name is required', got %q", text)
	}
}

func TestHandleSandboxDestroy_MissingName(t *testing.T) {
	s := newTestServer(t)

	req := makeRequest("sandbox_destroy", map[string]any{})
	result, err := s.handleSandboxDestroy(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError to be true")
	}
	text := resultText(t, result)
	if !strings.Contains(text, "parameter 'name' is required") {
		t.Errorf("expected 'parameter name is required', got %q", text)
	}
}

func TestHandleSandboxStats_MissingName(t *testing.T) {
	s := newTestServer(t)

	req := makeRequest("sandbox_stats", map[string]any{})
	result, err := s.handleSandboxStats(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError to be true")
	}
	text := resultText(t, result)
	if !strings.Contains(text, "parameter 'name' is required") {
		t.Errorf("expected 'parameter name is required', got %q", text)
	}
}

func TestHandleSandboxCreate_DuplicateName(t *testing.T) {
	s := newTestServer(t)
	bpPath := writeTempBlueprint(t)

	createSandbox(t, s, "dup-test", bpPath)

	// Attempt to create again with the same name.
	req := makeRequest("sandbox_create", map[string]any{
		"name":      "dup-test",
		"blueprint": bpPath,
	})
	result, err := s.handleSandboxCreate(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError to be true for duplicate name")
	}
	text := resultText(t, result)
	if !strings.Contains(text, "already exists") {
		t.Errorf("expected 'already exists' in error, got %q", text)
	}
}

func TestHandleSandboxExec_NonexistentSandbox(t *testing.T) {
	s := newTestServer(t)

	req := makeRequest("sandbox_exec", map[string]any{
		"name":    "nonexistent",
		"command": "echo hi",
	})
	result, err := s.handleSandboxExec(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError to be true for nonexistent sandbox")
	}
	text := resultText(t, result)
	if !strings.Contains(text, "exec failed") {
		t.Errorf("expected 'exec failed' in error, got %q", text)
	}
}

func TestHandleA2AReceive_MissingParam(t *testing.T) {
	s := newTestServer(t)

	req := makeRequest("a2a_receive", map[string]any{})
	result, err := s.handleA2AReceive(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError to be true")
	}
	text := resultText(t, result)
	if !strings.Contains(text, "parameter 'sandbox_name' is required") {
		t.Errorf("expected 'parameter sandbox_name is required', got %q", text)
	}
}

func TestHandleA2ABroadcast_MissingParams(t *testing.T) {
	s := newTestServer(t)

	tests := []struct {
		desc string
		args map[string]any
		want string
	}{
		{
			desc: "missing from",
			args: map[string]any{"targets": "a,b", "type": "req", "payload": "{}"},
			want: "parameter 'from' is required",
		},
		{
			desc: "missing targets",
			args: map[string]any{"from": "a", "type": "req", "payload": "{}"},
			want: "parameter 'targets' is required",
		},
		{
			desc: "missing type",
			args: map[string]any{"from": "a", "targets": "b,c", "payload": "{}"},
			want: "parameter 'type' is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			req := makeRequest("a2a_broadcast", tt.args)
			result, err := s.handleA2ABroadcast(context.Background(), req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !result.IsError {
				t.Fatal("expected IsError to be true")
			}
			text := resultText(t, result)
			if !strings.Contains(text, tt.want) {
				t.Errorf("expected error to contain %q, got %q", tt.want, text)
			}
		})
	}
}

func TestHandleSandboxReadyWait_MissingName(t *testing.T) {
	s := newTestServer(t)

	req := makeRequest("sandbox_ready_wait", map[string]any{})
	result, err := s.handleSandboxReadyWait(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError to be true")
	}
	text := resultText(t, result)
	if !strings.Contains(text, "parameter 'name' is required") {
		t.Errorf("expected 'parameter name is required', got %q", text)
	}
}
