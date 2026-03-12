package server

import (
	"net/http"

	"github.com/hg-dendi/sandboxmatrix/internal/controller"
	"github.com/hg-dendi/sandboxmatrix/internal/image"
)

// --------------------------------------------------------------------
// Image build handlers
// --------------------------------------------------------------------

type buildImageRequest struct {
	Blueprint string `json:"blueprint"`
}

func handleBuildImage(ctrl *controller.Controller) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req buildImageRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.Blueprint == "" {
			errorResponse(w, http.StatusBadRequest, "blueprint is required")
			return
		}

		builder := image.NewBuilder(ctrl.Runtime())
		result, err := builder.Build(r.Context(), req.Blueprint)
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonResponse(w, http.StatusCreated, result)
	}
}

func handleListImages(ctrl *controller.Controller) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		builder := image.NewBuilder(ctrl.Runtime())
		images, err := builder.ListBuiltImages(r.Context())
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		if images == nil {
			images = []image.Info{}
		}
		jsonResponse(w, http.StatusOK, images)
	}
}
