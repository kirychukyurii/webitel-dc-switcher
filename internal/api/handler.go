package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/kirychukyurii/webitel-dc-switcher/internal/service"
)

// Handler holds the HTTP handlers and dependencies
type Handler struct {
	service  service.DatacenterService
	logger   *slog.Logger
	basePath string
}

// NewHandler creates a new HTTP handler
func NewHandler(service service.DatacenterService, basePath string, logger *slog.Logger) *Handler {
	return &Handler{
		service:  service,
		logger:   logger,
		basePath: basePath,
	}
}

// Router creates and configures the HTTP router
func (h *Handler) Router() http.Handler {
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(h.loggingMiddleware)
	r.Use(middleware.Recoverer)

	// Create routes handler
	routesHandler := h.createRoutes()

	// If base path is configured, mount routes on that path
	if h.basePath != "" {
		r.Mount(h.basePath, routesHandler)
	} else {
		r.Mount("/", routesHandler)
	}

	return r
}

// createRoutes creates the API and UI routes
func (h *Handler) createRoutes() http.Handler {
	r := chi.NewRouter()

	// API routes
	r.Route("/api", func(r chi.Router) {
		// Datacenter routes
		r.Get("/datacenters", h.ListDatacenters)
		r.Get("/datacenters/{name}/nodes", h.GetNodes)
		r.Post("/datacenters/{name}/activate", h.ActivateDatacenter)

		// Region routes
		r.Get("/regions", h.ListRegions)
		r.Get("/regions/{name}/datacenters", h.GetDatacentersByRegion)
		r.Post("/regions/{name}/activate", h.ActivateRegion)
	})

	// Serve UI (must be last to act as catch-all)
	r.HandleFunc("/*", h.ServeUI())

	return r
}

// loggingMiddleware logs HTTP requests
func (h *Handler) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.logger.Info("http request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.String("remote_addr", r.RemoteAddr),
		)
		next.ServeHTTP(w, r)
	})
}

// errorResponse represents an error response
type errorResponse struct {
	Error string `json:"error"`
}

// respondJSON writes a JSON response
func (h *Handler) respondJSON(w http.ResponseWriter, statusCode int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("failed to encode response",
			slog.String("error", err.Error()),
		)
	}
}

// respondError writes an error response
func (h *Handler) respondError(w http.ResponseWriter, statusCode int, message string) {
	h.respondJSON(w, statusCode, errorResponse{Error: message})
}
