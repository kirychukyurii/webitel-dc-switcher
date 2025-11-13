package api

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/kirychukyurii/webitel-dc-switcher/ui"
)

// ServeUI returns a handler that serves the embedded UI files
func (h *Handler) ServeUI() http.HandlerFunc {
	// Get embedded filesystem
	fsys, err := ui.GetFileSystem()
	if err != nil {
		h.logger.Error("failed to get UI filesystem", "error", err.Error())
		return func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "UI not available", http.StatusNotFound)
		}
	}

	// Create file server
	fileServer := http.FileServer(fsys)

	// Pre-generate index.html with basePath injected
	var indexHTML []byte
	if file, err := fsys.Open("index.html"); err == nil {
		defer file.Close()
		if content, err := io.ReadAll(file); err == nil {
			// If base path is set, we need to:
			// 1. Fix asset paths (from /assets/ to {basePath}/assets/)
			// 2. Inject base path as global variable for Vue Router and Axios

			if h.basePath != "" {
				// Fix all asset paths: /assets/ -> {basePath}/assets/
				content = bytes.ReplaceAll(content, []byte(`"/assets/`), []byte(fmt.Sprintf(`"%s/assets/`, h.basePath)))
				content = bytes.ReplaceAll(content, []byte(`'/assets/`), []byte(fmt.Sprintf(`'%s/assets/`, h.basePath)))

				// Also fix favicon if present
				content = bytes.ReplaceAll(content, []byte(`href="/favicon.`), []byte(fmt.Sprintf(`href="%s/favicon.`, h.basePath)))
			}

			// Inject base path as a global variable before any script tags
			basePathScript := fmt.Sprintf("<script>window._BASE_PATH='%s';</script>", h.basePath)
			// Insert before closing </head> tag
			indexHTML = bytes.Replace(content, []byte("</head>"), []byte(basePathScript+"</head>"), 1)
		}
	}

	return func(w http.ResponseWriter, r *http.Request) {
		// Get the requested path
		path := r.URL.Path

		h.logger.Info("UI handler request",
			"path", path,
			"basePath", h.basePath,
		)

		// Don't serve API routes through UI handler
		if strings.HasPrefix(path, "/api/") {
			http.NotFound(w, r)
			return
		}

		// Strip base path if present (chi.Mount doesn't strip it automatically)
		if h.basePath != "" && strings.HasPrefix(path, h.basePath) {
			path = strings.TrimPrefix(path, h.basePath)
			if path == "" {
				path = "/"
			}
			h.logger.Info("stripped base path", "newPath", path)
		}

		// Clean the path for filesystem lookup
		cleanPath := strings.TrimPrefix(path, "/")
		if cleanPath == "" {
			cleanPath = "."
		}

		// Check if requesting index.html or SPA route (file doesn't exist)
		isIndexRequest := path == "/" || path == ""
		if !isIndexRequest {
			file, err := fsys.Open(cleanPath)
			if err != nil {
				h.logger.Info("file not found in embedded fs, serving index",
					"path", path,
					"cleanPath", cleanPath,
					"error", err.Error(),
				)
				isIndexRequest = true // File not found - serve index for SPA routing
			} else {
				file.Close()
				h.logger.Info("file found in embedded fs",
					"path", path,
					"cleanPath", cleanPath,
				)
			}
		}

		// Serve modified index.html for SPA routes
		if isIndexRequest && len(indexHTML) > 0 {
			h.logger.Info("serving modified index.html")
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			w.Write(indexHTML)
			return
		}

		// Serve static files via file server
		// Update request path to stripped path for fileServer
		r.URL.Path = path
		h.logger.Info("serving static file via fileServer", "path", path)
		fileServer.ServeHTTP(w, r)
	}
}
