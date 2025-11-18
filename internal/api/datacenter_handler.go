package api

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// ListDatacenters handles GET /api/datacenters
func (h *Handler) ListDatacenters(w http.ResponseWriter, r *http.Request) {
	datacenters, err := h.service.ListDatacenters(r.Context())
	if err != nil {
		h.logger.Error("failed to list datacenters",
			slog.String("error", err.Error()),
		)
		h.respondError(w, http.StatusInternalServerError, "failed to list datacenters")
		return
	}

	h.respondJSON(w, http.StatusOK, datacenters)
}

// GetNodes handles GET /api/datacenters/{name}/nodes
func (h *Handler) GetNodes(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		h.respondError(w, http.StatusBadRequest, "datacenter name is required")
		return
	}

	nodes, err := h.service.GetNodes(r.Context(), name)
	if err != nil {
		h.logger.Error("failed to get nodes",
			slog.String("datacenter", name),
			slog.String("error", err.Error()),
		)
		h.respondError(w, http.StatusInternalServerError, "failed to get nodes")
		return
	}

	h.respondJSON(w, http.StatusOK, nodes)
}

// ActivateDatacenter handles POST /api/datacenters/{name}/activate
func (h *Handler) ActivateDatacenter(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		h.respondError(w, http.StatusBadRequest, "datacenter name is required")
		return
	}

	result, err := h.service.ActivateDatacenter(r.Context(), name)
	if err != nil {
		h.logger.Error("failed to activate datacenter",
			slog.String("datacenter", name),
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

// GetJobs handles GET /api/datacenters/{name}/jobs
func (h *Handler) GetJobs(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		h.respondError(w, http.StatusBadRequest, "datacenter name is required")
		return
	}

	jobs, err := h.service.GetJobs(r.Context(), name)
	if err != nil {
		h.logger.Error("failed to get jobs",
			slog.String("datacenter", name),
			slog.String("error", err.Error()),
		)
		h.respondError(w, http.StatusInternalServerError, "failed to get jobs")
		return
	}

	h.respondJSON(w, http.StatusOK, jobs)
}

// StartJob handles POST /api/datacenters/{name}/jobs/{job_id}/start
func (h *Handler) StartJob(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	jobID := chi.URLParam(r, "job_id")

	if name == "" {
		h.respondError(w, http.StatusBadRequest, "datacenter name is required")
		return
	}
	if jobID == "" {
		h.respondError(w, http.StatusBadRequest, "job ID is required")
		return
	}

	result, err := h.service.StartJob(r.Context(), name, jobID)
	if err != nil {
		h.logger.Error("failed to start job",
			slog.String("datacenter", name),
			slog.String("job_id", jobID),
			slog.String("error", err.Error()),
		)

		// Return result with error details
		if result != nil {
			h.respondJSON(w, http.StatusInternalServerError, result)
			return
		}

		h.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.respondJSON(w, http.StatusOK, result)
}

// StopJob handles POST /api/datacenters/{name}/jobs/{job_id}/stop
func (h *Handler) StopJob(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	jobID := chi.URLParam(r, "job_id")

	if name == "" {
		h.respondError(w, http.StatusBadRequest, "datacenter name is required")
		return
	}
	if jobID == "" {
		h.respondError(w, http.StatusBadRequest, "job ID is required")
		return
	}

	result, err := h.service.StopJob(r.Context(), name, jobID)
	if err != nil {
		h.logger.Error("failed to stop job",
			slog.String("datacenter", name),
			slog.String("job_id", jobID),
			slog.String("error", err.Error()),
		)

		// Return result with error details
		if result != nil {
			h.respondJSON(w, http.StatusInternalServerError, result)
			return
		}

		h.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.respondJSON(w, http.StatusOK, result)
}
