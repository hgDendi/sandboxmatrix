package server

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/hg-dendi/sandboxmatrix/internal/controller"
)

// maxFileUploadSize is the maximum allowed file upload size (10 MB).
const maxFileUploadSize = 10 << 20

// uploadFileJSONRequest is the JSON request body for file uploads.
type uploadFileJSONRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"` // base64-encoded content
}

// handleUploadFile handles PUT /api/v1/sandboxes/{name}/files?path=/workspace/main.py
// Accepts raw file content (application/octet-stream) or JSON with base64 content.
func handleUploadFile(ctrl *controller.Controller) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if name == "" {
			errorResponse(w, http.StatusBadRequest, "sandbox name is required")
			return
		}

		var (
			filePath string
			content  io.Reader
		)

		ct := r.Header.Get("Content-Type")
		if strings.HasPrefix(ct, "application/json") {
			r.Body = http.MaxBytesReader(w, r.Body, maxFileUploadSize)
			var req uploadFileJSONRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				errorResponse(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
				return
			}
			filePath = req.Path
			if filePath == "" {
				errorResponse(w, http.StatusBadRequest, "path is required")
				return
			}
			decoded, err := base64.StdEncoding.DecodeString(req.Content)
			if err != nil {
				errorResponse(w, http.StatusBadRequest, "invalid base64 content: "+err.Error())
				return
			}
			content = strings.NewReader(string(decoded))
		} else {
			// Raw body upload (application/octet-stream or other).
			filePath = r.URL.Query().Get("path")
			if filePath == "" {
				errorResponse(w, http.StatusBadRequest, "path query parameter is required")
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxFileUploadSize)
			content = r.Body
		}

		if err := ctrl.WriteFile(r.Context(), name, filePath, content); err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}

		jsonResponse(w, http.StatusOK, map[string]string{
			"status": "uploaded",
			"path":   filePath,
		})
	}
}

// handleDownloadFile handles GET /api/v1/sandboxes/{name}/files?path=/workspace/main.py
// Returns raw file content.
func handleDownloadFile(ctrl *controller.Controller) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if name == "" {
			errorResponse(w, http.StatusBadRequest, "sandbox name is required")
			return
		}

		filePath := r.URL.Query().Get("path")
		if filePath == "" {
			errorResponse(w, http.StatusBadRequest, "path query parameter is required")
			return
		}

		rc, err := ctrl.ReadFile(r.Context(), name, filePath)
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rc.Close()

		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(w, rc)
	}
}

// handleListFiles handles GET /api/v1/sandboxes/{name}/files/list?path=/workspace
// Returns JSON array of FileInfo.
func handleListFiles(ctrl *controller.Controller) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if name == "" {
			errorResponse(w, http.StatusBadRequest, "sandbox name is required")
			return
		}

		dirPath := r.URL.Query().Get("path")
		if dirPath == "" {
			dirPath = "/"
		}

		files, err := ctrl.ListFiles(r.Context(), name, dirPath)
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		if files == nil {
			files = []controller.FileInfo{}
		}
		jsonResponse(w, http.StatusOK, files)
	}
}
