// Package web provides an embedded web dashboard for sandboxMatrix.
package web

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/hg-dendi/sandboxmatrix/internal/controller"
	v1alpha1 "github.com/hg-dendi/sandboxmatrix/pkg/api/v1alpha1"
)

//go:embed static/*
var staticFS embed.FS

// Dashboard serves the embedded web UI and dashboard API endpoints.
type Dashboard struct {
	ctrl   *controller.Controller
	addr   string
	server *http.Server
}

// NewDashboard creates a new Dashboard bound to the given address.
func NewDashboard(ctrl *controller.Controller, addr string) *Dashboard {
	return &Dashboard{ctrl: ctrl, addr: addr}
}

// sandboxJSON is the JSON representation of a sandbox for the dashboard API.
type sandboxJSON struct {
	Name        string            `json:"name"`
	State       string            `json:"state"`
	Blueprint   string            `json:"blueprint"`
	RuntimeID   string            `json:"runtimeID"`
	IP          string            `json:"ip,omitempty"`
	CreatedAt   time.Time         `json:"createdAt"`
	StartedAt   *time.Time        `json:"startedAt,omitempty"`
	StoppedAt   *time.Time        `json:"stoppedAt,omitempty"`
	Message     string            `json:"message,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Resources   v1alpha1.Resources `json:"resources,omitempty"`
}

// matrixJSON is the JSON representation of a matrix for the dashboard API.
type matrixJSON struct {
	Name      string                  `json:"name"`
	State     string                  `json:"state"`
	Members   []v1alpha1.MatrixMember `json:"members"`
	CreatedAt time.Time               `json:"createdAt"`
}

// sessionJSON is the JSON representation of a session for the dashboard API.
type sessionJSON struct {
	ID        string     `json:"id"`
	Sandbox   string     `json:"sandbox"`
	State     string     `json:"state"`
	StartedAt *time.Time `json:"startedAt,omitempty"`
	EndedAt   *time.Time `json:"endedAt,omitempty"`
	ExecCount int        `json:"execCount"`
}

// Start starts the HTTP server in a goroutine and returns immediately.
func (d *Dashboard) Start() error {
	mux := http.NewServeMux()

	// Serve static files from the embedded filesystem.
	staticContent, err := fs.Sub(staticFS, "static")
	if err != nil {
		return fmt.Errorf("create static sub-filesystem: %w", err)
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticContent))))

	// Serve the index page at root.
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		data, err := staticFS.ReadFile("static/index.html")
		if err != nil {
			http.Error(w, "dashboard not found", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
	})

	// API endpoints.
	mux.HandleFunc("GET /api/dashboard/sandboxes", d.handleListSandboxes)
	mux.HandleFunc("GET /api/dashboard/matrices", d.handleListMatrices)
	mux.HandleFunc("GET /api/dashboard/sessions", d.handleListSessions)
	mux.HandleFunc("POST /api/dashboard/sandboxes/{name}/stop", d.handleStopSandbox)
	mux.HandleFunc("POST /api/dashboard/sandboxes/{name}/start", d.handleStartSandbox)
	mux.HandleFunc("DELETE /api/dashboard/sandboxes/{name}", d.handleDestroySandbox)

	d.server = &http.Server{
		Addr:    d.addr,
		Handler: mux,
	}

	go func() {
		if err := d.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("dashboard server error: %v\n", err)
		}
	}()

	return nil
}

// Shutdown gracefully shuts down the dashboard server.
func (d *Dashboard) Shutdown(ctx context.Context) error {
	if d.server == nil {
		return nil
	}
	return d.server.Shutdown(ctx)
}

func (d *Dashboard) handleListSandboxes(w http.ResponseWriter, r *http.Request) {
	sandboxes, err := d.ctrl.List()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	result := make([]sandboxJSON, 0, len(sandboxes))
	for _, sb := range sandboxes {
		result = append(result, sandboxJSON{
			Name:      sb.Metadata.Name,
			State:     string(sb.Status.State),
			Blueprint: sb.Spec.BlueprintRef,
			RuntimeID: truncateID(sb.Status.RuntimeID),
			IP:        sb.Status.IP,
			CreatedAt: sb.Metadata.CreatedAt,
			StartedAt: sb.Status.StartedAt,
			StoppedAt: sb.Status.StoppedAt,
			Message:   sb.Status.Message,
			Labels:    sb.Metadata.Labels,
			Resources: sb.Spec.Resources,
		})
	}

	writeJSON(w, http.StatusOK, result)
}

func (d *Dashboard) handleListMatrices(w http.ResponseWriter, r *http.Request) {
	matrices, err := d.ctrl.ListMatrices()
	if err != nil {
		// If matrices are not configured, return empty list.
		if strings.Contains(err.Error(), "not configured") {
			writeJSON(w, http.StatusOK, []matrixJSON{})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	result := make([]matrixJSON, 0, len(matrices))
	for _, mx := range matrices {
		result = append(result, matrixJSON{
			Name:      mx.Metadata.Name,
			State:     string(mx.State),
			Members:   mx.Members,
			CreatedAt: mx.Metadata.CreatedAt,
		})
	}

	writeJSON(w, http.StatusOK, result)
}

func (d *Dashboard) handleListSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := d.ctrl.ListSessions("")
	if err != nil {
		// If sessions are not configured, return empty list.
		if strings.Contains(err.Error(), "not configured") {
			writeJSON(w, http.StatusOK, []sessionJSON{})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	result := make([]sessionJSON, 0, len(sessions))
	for _, s := range sessions {
		result = append(result, sessionJSON{
			ID:        s.Metadata.Name,
			Sandbox:   s.Sandbox,
			State:     string(s.State),
			StartedAt: s.StartedAt,
			EndedAt:   s.EndedAt,
			ExecCount: s.ExecCount,
		})
	}

	writeJSON(w, http.StatusOK, result)
}

func (d *Dashboard) handleStopSandbox(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "sandbox name required"})
		return
	}

	if err := d.ctrl.Stop(r.Context(), name); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped", "name": name})
}

func (d *Dashboard) handleStartSandbox(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "sandbox name required"})
		return
	}

	if err := d.ctrl.Start(r.Context(), name); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "started", "name": name})
}

func (d *Dashboard) handleDestroySandbox(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "sandbox name required"})
		return
	}

	if err := d.ctrl.Destroy(r.Context(), name); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "destroyed", "name": name})
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// truncateID shortens a runtime ID to 12 characters for display.
func truncateID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
