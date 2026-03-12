package server

import (
	"encoding/json"
	"net/http"

	"github.com/hg-dendi/sandboxmatrix/internal/autoscale"
	"github.com/hg-dendi/sandboxmatrix/internal/runtime"
)

// handleAutoscaleStatus returns the current autoscaler status.
func handleAutoscaleStatus(as *autoscale.Autoscaler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if as == nil {
			errorResponse(w, http.StatusServiceUnavailable, "autoscaler not configured")
			return
		}
		status, err := as.Status(r.Context())
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonResponse(w, http.StatusOK, status)
	}
}

// handleAutoscaleEnable starts the autoscaler.
func handleAutoscaleEnable(as *autoscale.Autoscaler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if as == nil {
			errorResponse(w, http.StatusServiceUnavailable, "autoscaler not configured")
			return
		}
		as.Start(r.Context())
		jsonResponse(w, http.StatusOK, map[string]string{"status": "enabled"})
	}
}

// handleAutoscaleDisable stops the autoscaler and restores all sandboxes.
func handleAutoscaleDisable(as *autoscale.Autoscaler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if as == nil {
			errorResponse(w, http.StatusServiceUnavailable, "autoscaler not configured")
			return
		}
		as.Stop(r.Context())
		jsonResponse(w, http.StatusOK, map[string]string{"status": "disabled"})
	}
}

type setPriorityRequest struct {
	Priority int `json:"priority"` // 0=low, 1=normal, 2=high, 3=critical
}

// handleSetPriority sets the autoscaler priority for a sandbox.
func handleSetPriority(as *autoscale.Autoscaler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if as == nil {
			errorResponse(w, http.StatusServiceUnavailable, "autoscaler not configured")
			return
		}

		name := r.PathValue("name")
		if name == "" {
			errorResponse(w, http.StatusBadRequest, "sandbox name is required")
			return
		}

		var req setPriorityRequest
		if r.Body == nil {
			errorResponse(w, http.StatusBadRequest, "request body is required")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			errorResponse(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}

		if req.Priority < 0 || req.Priority > 3 {
			errorResponse(w, http.StatusBadRequest, "priority must be 0-3")
			return
		}

		as.SetPriority(name, runtime.SandboxPriority(req.Priority))
		jsonResponse(w, http.StatusOK, map[string]string{
			"status": "updated",
			"name":   name,
		})
	}
}
