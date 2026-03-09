package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hg-dendi/sandboxmatrix/internal/controller"
	"github.com/hg-dendi/sandboxmatrix/internal/state"
	v1alpha1 "github.com/hg-dendi/sandboxmatrix/pkg/api/v1alpha1"
)

func TestExecStreamWebSocket(t *testing.T) {
	rt := newMockRuntime()
	store := state.NewMemoryStore()
	ctrl := controller.New(rt, store, nil, nil)

	// Insert a running sandbox directly.
	now := time.Now()
	sb := &v1alpha1.Sandbox{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Sandbox"},
		Metadata: v1alpha1.ObjectMeta{Name: "ws-sb", CreatedAt: now, UpdatedAt: now},
		Spec:     v1alpha1.SandboxSpec{BlueprintRef: "test-bp"},
		Status:   v1alpha1.SandboxStatus{State: v1alpha1.SandboxStateRunning, RuntimeID: "mock-ws"},
	}
	if err := store.Save(sb); err != nil {
		t.Fatal(err)
	}
	rt.containers["mock-ws"] = &mockContainer{id: "mock-ws", state: "running"}

	// Create a test HTTP server with the WebSocket handler.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/sandboxes/{name}/exec/stream", handleExecStream(ctrl))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	// Connect via WebSocket.
	wsURL := "ws" + ts.URL[4:] + "/api/v1/sandboxes/ws-sb/exec/stream"
	ws, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer ws.Close()
	defer resp.Body.Close()

	// Send command.
	cmd := wsExecRequest{Command: []string{"echo", "hello"}}
	if err := ws.WriteJSON(cmd); err != nil {
		t.Fatalf("write command: %v", err)
	}

	// Read events until exit.
	var events []wsExecEvent
	for {
		_, msg, err := ws.ReadMessage()
		if err != nil {
			break
		}
		var evt wsExecEvent
		if err := json.Unmarshal(msg, &evt); err != nil {
			t.Fatalf("unmarshal event: %v", err)
		}
		events = append(events, evt)
		if evt.Type == "exit" || evt.Type == "error" {
			break
		}
	}

	if len(events) == 0 {
		t.Fatal("expected at least one event")
	}

	// Last event should be "exit" with code 0.
	last := events[len(events)-1]
	if last.Type != "exit" {
		t.Fatalf("expected last event type 'exit', got %q", last.Type)
	}
	if last.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", last.ExitCode)
	}
}

func TestExecStreamWebSocket_InvalidCommand(t *testing.T) {
	rt := newMockRuntime()
	store := state.NewMemoryStore()
	ctrl := controller.New(rt, store, nil, nil)

	now := time.Now()
	sb := &v1alpha1.Sandbox{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Sandbox"},
		Metadata: v1alpha1.ObjectMeta{Name: "ws-sb2", CreatedAt: now, UpdatedAt: now},
		Spec:     v1alpha1.SandboxSpec{BlueprintRef: "test-bp"},
		Status:   v1alpha1.SandboxStatus{State: v1alpha1.SandboxStateRunning, RuntimeID: "mock-ws2"},
	}
	if err := store.Save(sb); err != nil {
		t.Fatal(err)
	}
	rt.containers["mock-ws2"] = &mockContainer{id: "mock-ws2", state: "running"}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/sandboxes/{name}/exec/stream", handleExecStream(ctrl))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	wsURL := "ws" + ts.URL[4:] + "/api/v1/sandboxes/ws-sb2/exec/stream"
	ws, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer ws.Close()
	defer resp.Body.Close()

	// Send empty command.
	cmd := wsExecRequest{Command: []string{}}
	if err := ws.WriteJSON(cmd); err != nil {
		t.Fatalf("write command: %v", err)
	}

	// Should get an error event.
	_, msg, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("read message: %v", err)
	}
	var evt wsExecEvent
	if err := json.Unmarshal(msg, &evt); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if evt.Type != "error" {
		t.Fatalf("expected error event, got %q", evt.Type)
	}
}
