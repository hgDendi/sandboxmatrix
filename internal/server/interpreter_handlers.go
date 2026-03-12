package server

import (
	"net/http"

	"github.com/hg-dendi/sandboxmatrix/internal/controller"
	"github.com/hg-dendi/sandboxmatrix/internal/interpreter"
)

type interpretRequest struct {
	Language string `json:"language"`
	Code     string `json:"code"`
	Timeout  int    `json:"timeout,omitempty"`
}

type interpretResponse struct {
	Stdout   string                   `json:"stdout"`
	Stderr   string                   `json:"stderr"`
	ExitCode int                      `json:"exitCode"`
	Duration string                   `json:"duration"`
	Error    string                   `json:"error,omitempty"`
	Files    []interpreter.OutputFile `json:"files,omitempty"`
}

// handleInterpret returns an HTTP handler that executes code in a sandbox.
//
//	POST /api/v1/sandboxes/{name}/interpret
//	Body: {"language": "python", "code": "print('hello')", "timeout": 30}
//	Response: interpretResponse
func handleInterpret(ctrl *controller.Controller) http.HandlerFunc {
	interp := interpreter.New(ctrl)

	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if name == "" {
			errorResponse(w, http.StatusBadRequest, "sandbox name is required")
			return
		}

		var req interpretRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.Language == "" {
			errorResponse(w, http.StatusBadRequest, "language is required")
			return
		}
		if req.Code == "" {
			errorResponse(w, http.StatusBadRequest, "code is required")
			return
		}

		result, err := interp.Execute(r.Context(), &interpreter.ExecuteRequest{
			Sandbox:  name,
			Language: interpreter.Language(req.Language),
			Code:     req.Code,
			Timeout:  req.Timeout,
		})
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}

		jsonResponse(w, http.StatusOK, interpretResponse{
			Stdout:   result.Stdout,
			Stderr:   result.Stderr,
			ExitCode: result.ExitCode,
			Duration: result.Duration.String(),
			Error:    result.Error,
			Files:    result.Files,
		})
	}
}
