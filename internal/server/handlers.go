package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	goruntime "runtime"

	"github.com/hg-dendi/sandboxmatrix/internal/controller"
	"github.com/hg-dendi/sandboxmatrix/internal/runtime"
	v1alpha1 "github.com/hg-dendi/sandboxmatrix/pkg/api/v1alpha1"
)

// --------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------

// errorResponse writes a JSON error response with the given status code.
func errorResponse(w http.ResponseWriter, status int, msg string) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// jsonResponse writes a JSON response with the given status code and payload.
func jsonResponse(w http.ResponseWriter, status int, payload any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// decodeJSON decodes a JSON request body into dst.
// Returns false and writes an error response if decoding fails.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if r.Body == nil {
		errorResponse(w, http.StatusBadRequest, "request body is required")
		return false
	}
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		errorResponse(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
		return false
	}
	return true
}

// --------------------------------------------------------------------
// Health / Version
// --------------------------------------------------------------------

// Version variables — set from the cli package at server startup.
var (
	ServerVersion   = "dev"
	ServerCommit    = "unknown"
	ServerBuildDate = "unknown"
)

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	jsonResponse(w, http.StatusOK, map[string]string{"status": "ok"})
}

func handleVersion(w http.ResponseWriter, _ *http.Request) {
	jsonResponse(w, http.StatusOK, map[string]string{
		"version":   ServerVersion,
		"commit":    ServerCommit,
		"buildDate": ServerBuildDate,
		"goVersion": goruntime.Version(),
		"os":        goruntime.GOOS,
		"arch":      goruntime.GOARCH,
	})
}

// --------------------------------------------------------------------
// Sandbox handlers
// --------------------------------------------------------------------

type createSandboxRequest struct {
	Name      string `json:"name"`
	Blueprint string `json:"blueprint"`
	Workspace string `json:"workspace,omitempty"`
}

func handleCreateSandbox(ctrl *controller.Controller) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createSandboxRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.Name == "" {
			errorResponse(w, http.StatusBadRequest, "name is required")
			return
		}
		if req.Blueprint == "" {
			errorResponse(w, http.StatusBadRequest, "blueprint is required")
			return
		}

		sb, err := ctrl.Create(r.Context(), controller.CreateOptions{
			Name:          req.Name,
			BlueprintPath: req.Blueprint,
			WorkspaceDir:  req.Workspace,
		})
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonResponse(w, http.StatusCreated, sb)
	}
}

func handleListSandboxes(ctrl *controller.Controller) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		sandboxes, err := ctrl.List()
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		if sandboxes == nil {
			sandboxes = []*v1alpha1.Sandbox{}
		}
		jsonResponse(w, http.StatusOK, sandboxes)
	}
}

func handleGetSandbox(ctrl *controller.Controller) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if name == "" {
			errorResponse(w, http.StatusBadRequest, "sandbox name is required")
			return
		}
		sb, err := ctrl.Get(name)
		if err != nil {
			errorResponse(w, http.StatusNotFound, err.Error())
			return
		}
		jsonResponse(w, http.StatusOK, sb)
	}
}

func handleStartSandbox(ctrl *controller.Controller) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if name == "" {
			errorResponse(w, http.StatusBadRequest, "sandbox name is required")
			return
		}
		if err := ctrl.Start(r.Context(), name); err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonResponse(w, http.StatusOK, map[string]string{"status": "started", "name": name})
	}
}

func handleStopSandbox(ctrl *controller.Controller) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if name == "" {
			errorResponse(w, http.StatusBadRequest, "sandbox name is required")
			return
		}
		if err := ctrl.Stop(r.Context(), name); err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonResponse(w, http.StatusOK, map[string]string{"status": "stopped", "name": name})
	}
}

func handleDestroySandbox(ctrl *controller.Controller) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if name == "" {
			errorResponse(w, http.StatusBadRequest, "sandbox name is required")
			return
		}
		if err := ctrl.Destroy(r.Context(), name); err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonResponse(w, http.StatusOK, map[string]string{"status": "destroyed", "name": name})
	}
}

type execRequest struct {
	Command []string `json:"command"`
}

type execResponse struct {
	ExitCode int    `json:"exitCode"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
}

func handleExecSandbox(ctrl *controller.Controller) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if name == "" {
			errorResponse(w, http.StatusBadRequest, "sandbox name is required")
			return
		}

		var req execRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if len(req.Command) == 0 {
			errorResponse(w, http.StatusBadRequest, "command is required")
			return
		}

		var stdout, stderr bytes.Buffer
		result, err := ctrl.Exec(r.Context(), name, &runtime.ExecConfig{
			Cmd:    req.Command,
			Stdout: &stdout,
			Stderr: &stderr,
		})
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}

		jsonResponse(w, http.StatusOK, execResponse{
			ExitCode: result.ExitCode,
			Stdout:   stdout.String(),
			Stderr:   stderr.String(),
		})
	}
}

func handleStatsSandbox(ctrl *controller.Controller) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if name == "" {
			errorResponse(w, http.StatusBadRequest, "sandbox name is required")
			return
		}

		stats, err := ctrl.Stats(r.Context(), name)
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonResponse(w, http.StatusOK, stats)
	}
}

type createSnapshotRequest struct {
	Tag string `json:"tag,omitempty"`
}

type createSnapshotResponse struct {
	SnapshotID string `json:"snapshotId"`
	Tag        string `json:"tag"`
}

func handleCreateSnapshot(ctrl *controller.Controller) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if name == "" {
			errorResponse(w, http.StatusBadRequest, "sandbox name is required")
			return
		}

		var req createSnapshotRequest
		// Body is optional for snapshot creation.
		if r.Body != nil && r.ContentLength > 0 {
			if !decodeJSON(w, r, &req) {
				return
			}
		}

		snapshotID, err := ctrl.Snapshot(r.Context(), name, req.Tag)
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}

		jsonResponse(w, http.StatusCreated, createSnapshotResponse{
			SnapshotID: snapshotID,
			Tag:        req.Tag,
		})
	}
}

func handleListSnapshots(ctrl *controller.Controller) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if name == "" {
			errorResponse(w, http.StatusBadRequest, "sandbox name is required")
			return
		}

		snapshots, err := ctrl.ListSnapshots(r.Context(), name)
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		if snapshots == nil {
			snapshots = []runtime.SnapshotInfo{}
		}
		jsonResponse(w, http.StatusOK, snapshots)
	}
}

// --------------------------------------------------------------------
// Matrix handlers
// --------------------------------------------------------------------

type createMatrixRequest struct {
	Name    string                  `json:"name"`
	Members []v1alpha1.MatrixMember `json:"members"`
}

func handleCreateMatrix(ctrl *controller.Controller) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createMatrixRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.Name == "" {
			errorResponse(w, http.StatusBadRequest, "name is required")
			return
		}
		if len(req.Members) == 0 {
			errorResponse(w, http.StatusBadRequest, "members is required")
			return
		}

		mx, err := ctrl.CreateMatrix(r.Context(), req.Name, req.Members)
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonResponse(w, http.StatusCreated, mx)
	}
}

func handleListMatrices(ctrl *controller.Controller) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		matrices, err := ctrl.ListMatrices()
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		if matrices == nil {
			matrices = []*v1alpha1.Matrix{}
		}
		jsonResponse(w, http.StatusOK, matrices)
	}
}

func handleGetMatrix(ctrl *controller.Controller) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if name == "" {
			errorResponse(w, http.StatusBadRequest, "matrix name is required")
			return
		}
		mx, err := ctrl.GetMatrix(name)
		if err != nil {
			errorResponse(w, http.StatusNotFound, err.Error())
			return
		}
		jsonResponse(w, http.StatusOK, mx)
	}
}

func handleStartMatrix(ctrl *controller.Controller) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if name == "" {
			errorResponse(w, http.StatusBadRequest, "matrix name is required")
			return
		}
		if err := ctrl.StartMatrix(r.Context(), name); err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonResponse(w, http.StatusOK, map[string]string{"status": "started", "name": name})
	}
}

func handleStopMatrix(ctrl *controller.Controller) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if name == "" {
			errorResponse(w, http.StatusBadRequest, "matrix name is required")
			return
		}
		if err := ctrl.StopMatrix(r.Context(), name); err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonResponse(w, http.StatusOK, map[string]string{"status": "stopped", "name": name})
	}
}

func handleDestroyMatrix(ctrl *controller.Controller) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if name == "" {
			errorResponse(w, http.StatusBadRequest, "matrix name is required")
			return
		}
		if err := ctrl.DestroyMatrix(r.Context(), name); err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonResponse(w, http.StatusOK, map[string]string{"status": "destroyed", "name": name})
	}
}

// --------------------------------------------------------------------
// Session handlers
// --------------------------------------------------------------------

type startSessionRequest struct {
	Sandbox string `json:"sandbox"`
}

func handleStartSession(ctrl *controller.Controller) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req startSessionRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.Sandbox == "" {
			errorResponse(w, http.StatusBadRequest, "sandbox is required")
			return
		}

		session, err := ctrl.StartSession(r.Context(), req.Sandbox)
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonResponse(w, http.StatusCreated, session)
	}
}

func handleListSessions(ctrl *controller.Controller) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sandbox := r.URL.Query().Get("sandbox")
		sessions, err := ctrl.ListSessions(sandbox)
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		if sessions == nil {
			sessions = []*v1alpha1.Session{}
		}
		jsonResponse(w, http.StatusOK, sessions)
	}
}

func handleEndSession(ctrl *controller.Controller) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			errorResponse(w, http.StatusBadRequest, "session id is required")
			return
		}
		if err := ctrl.EndSession(r.Context(), id); err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonResponse(w, http.StatusOK, map[string]string{"status": "ended", "id": id})
	}
}

func handleExecInSession(ctrl *controller.Controller) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			errorResponse(w, http.StatusBadRequest, "session id is required")
			return
		}

		var req execRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if len(req.Command) == 0 {
			errorResponse(w, http.StatusBadRequest, "command is required")
			return
		}

		var stdout, stderr bytes.Buffer
		result, err := ctrl.ExecInSession(r.Context(), id, &runtime.ExecConfig{
			Cmd:    req.Command,
			Stdout: &stdout,
			Stderr: &stderr,
		})
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}

		jsonResponse(w, http.StatusOK, execResponse{
			ExitCode: result.ExitCode,
			Stdout:   stdout.String(),
			Stderr:   stderr.String(),
		})
	}
}
