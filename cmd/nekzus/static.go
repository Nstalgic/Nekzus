package main

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed webdist
var webUI embed.FS

// serveWebUI returns a handler that serves the embedded web UI
func (app *Application) serveWebUI() http.Handler {
	// Get the webdist subfolder
	distFS, err := fs.Sub(webUI, "webdist")
	if err != nil {
		log.Error("failed to load embedded web ui", "error", err)
		log.Info("web ui may not be available, run 'make build-web' to build it")
		return http.NotFoundHandler()
	}

	// Create file server
	fileServer := http.FileServer(http.FS(distFS))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip API routes - let the actual API handlers handle these
		// If we're seeing an API path here, it means the route didn't match any handler
		if strings.HasPrefix(r.URL.Path, "/api/") ||
			strings.HasPrefix(r.URL.Path, "/admin/api/") ||
			strings.HasPrefix(r.URL.Path, "/metrics") ||
			strings.HasPrefix(r.URL.Path, "/healthz") ||
			strings.HasPrefix(r.URL.Path, "/livez") ||
			strings.HasPrefix(r.URL.Path, "/readyz") {
			// Don't serve web UI for API routes - return early
			// This lets Go's ServeMux return a 404 for unmatched API routes
			return
		}

		path := strings.TrimPrefix(r.URL.Path, "/")

		// Try to open the file
		if path == "" {
			path = "index.html"
		}

		file, err := distFS.Open(path)
		if err != nil {
			// File doesn't exist, serve index.html for SPA routing
			// This allows React Router to handle the route
			r.URL.Path = "/"
		} else {
			file.Close()
		}

		// Serve the file
		fileServer.ServeHTTP(w, r)
	})
}

// isWebUIAvailable checks if the web UI is embedded and available
func (app *Application) isWebUIAvailable() bool {
	distFS, err := fs.Sub(webUI, "webdist")
	if err != nil {
		return false
	}

	// Check if index.html exists
	_, err = distFS.Open("index.html")
	return err == nil
}
