package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/hg-dendi/sandboxmatrix/internal/controller"
	"github.com/hg-dendi/sandboxmatrix/internal/observability"
	"github.com/hg-dendi/sandboxmatrix/internal/runtime"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(_ *http.Request) bool { return true },
}

// wsExecRequest is sent by the client to start an exec session.
type wsExecRequest struct {
	Command []string `json:"command"`
}

// wsExecEvent is streamed back to the client.
type wsExecEvent struct {
	Type     string `json:"type"`               // "stdout", "stderr", "exit", "error"
	Data     string `json:"data,omitempty"`     // output data
	ExitCode int    `json:"exitCode,omitempty"` // only for type "exit"
}

// wsWriter is an io.Writer that sends each Write as a WebSocket message.
type wsWriter struct {
	mu         *sync.Mutex
	conn       *websocket.Conn
	streamType string // "stdout" or "stderr"
}

func (w *wsWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	evt := wsExecEvent{Type: w.streamType, Data: string(p)}
	if err := w.conn.WriteJSON(evt); err != nil {
		return 0, err
	}
	return len(p), nil
}

// handleExecStream upgrades to WebSocket and streams exec output in real time.
//
// Protocol:
//  1. Client connects via WebSocket to /api/v1/sandboxes/{name}/exec/stream
//  2. Client sends JSON: {"command": ["sh", "-c", "..."]}
//  3. Server streams events: {"type":"stdout","data":"..."}, {"type":"stderr","data":"..."}
//  4. On completion: {"type":"exit","exitCode":0}
//  5. On error: {"type":"error","data":"error message"}
func handleExecStream(ctrl *controller.Controller) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if name == "" {
			http.Error(w, "sandbox name is required", http.StatusBadRequest)
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return // Upgrade already wrote the error response
		}
		defer conn.Close()

		observability.Metrics.WebSocketConnections.Inc()
		defer observability.Metrics.WebSocketConnections.Dec()

		// Read the command from the first message.
		var req wsExecRequest
		if err := conn.ReadJSON(&req); err != nil {
			_ = conn.WriteJSON(wsExecEvent{Type: "error", Data: "invalid request: " + err.Error()})
			return
		}
		if len(req.Command) == 0 {
			_ = conn.WriteJSON(wsExecEvent{Type: "error", Data: "command is required"})
			return
		}

		// Set up streaming writers.
		var mu sync.Mutex
		stdoutW := &wsWriter{mu: &mu, conn: conn, streamType: "stdout"}
		stderrW := &wsWriter{mu: &mu, conn: conn, streamType: "stderr"}

		// Set up stdin: read from WebSocket in background.
		stdinR, stdinW := io.Pipe()
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()

		// Read stdin from client in background.
		go func() {
			defer stdinW.Close()
			for {
				_, msg, err := conn.ReadMessage()
				if err != nil {
					return
				}
				// Try to parse as a stdin event.
				var evt struct {
					Type string `json:"type"`
					Data string `json:"data"`
				}
				if json.Unmarshal(msg, &evt) == nil && evt.Type == "stdin" {
					if _, err := stdinW.Write([]byte(evt.Data)); err != nil {
						return
					}
				}
			}
		}()

		// Execute command with streaming I/O.
		result, execErr := ctrl.Exec(ctx, name, &runtime.ExecConfig{
			Cmd:    req.Command,
			Stdout: stdoutW,
			Stderr: stderrW,
			Stdin:  stdinR,
		})

		mu.Lock()
		defer mu.Unlock()

		if execErr != nil {
			_ = conn.WriteJSON(wsExecEvent{Type: "error", Data: execErr.Error()})
			return
		}

		_ = conn.WriteJSON(wsExecEvent{Type: "exit", ExitCode: result.ExitCode})
	}
}
