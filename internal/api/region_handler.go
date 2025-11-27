package api

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// ListRegions handles GET /api/regions
func (h *Handler) ListRegions(w http.ResponseWriter, r *http.Request) {
	regions, err := h.service.ListRegions(r.Context())
	if err != nil {
		h.logger.Warn("failed to list regions",
			slog.String("error", err.Error()),
		)
		// Return empty list instead of error to allow UI to continue
		h.respondJSON(w, http.StatusOK, []interface{}{})
		return
	}

	h.respondJSON(w, http.StatusOK, regions)
}

// GetDatacentersByRegion handles GET /api/regions/{name}/datacenters
func (h *Handler) GetDatacentersByRegion(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		h.respondError(w, http.StatusBadRequest, "region name is required")
		return
	}

	datacenters, err := h.service.GetDatacentersByRegion(r.Context(), name)
	if err != nil {
		h.logger.Warn("region not found or unavailable",
			slog.String("region", name),
			slog.String("error", err.Error()),
		)
		// Return 404 for not found region
		h.respondError(w, http.StatusNotFound, err.Error())
		return
	}

	h.respondJSON(w, http.StatusOK, datacenters)
}

// ActivateRegion handles POST /api/regions/{name}/activate
func (h *Handler) ActivateRegion(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		h.respondError(w, http.StatusBadRequest, "region name is required")
		return
	}

	result, err := h.service.ActivateRegion(r.Context(), name)
	if err != nil {
		h.logger.Error("failed to activate region",
			slog.String("region", name),
			slog.String("error", err.Error()),
		)

		// If we have a result with rollback info, return it with the error
		if result != nil && len(result.Errors) > 0 {
			h.respondJSON(w, http.StatusInternalServerError, result)
			return
		}

		h.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.respondJSON(w, http.StatusOK, result)
}
