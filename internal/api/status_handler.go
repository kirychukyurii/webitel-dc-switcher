package api

import (
	"log/slog"
	"net/http"
)

// GetStatus handles GET /api/status
func (h *Handler) GetStatus(w http.ResponseWriter, r *http.Request) {
	status, err := h.service.GetStatus(r.Context())
	if err != nil {
		h.logger.Error("failed to get service status",
			slog.String("error", err.Error()),
		)
		h.respondError(w, http.StatusInternalServerError, "failed to get service status")
		return
	}

	h.respondJSON(w, http.StatusOK, status)
}
