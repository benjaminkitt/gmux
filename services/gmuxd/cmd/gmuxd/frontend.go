package main

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed web/*
var webFS embed.FS

// spaHandler serves the embedded frontend as a single-page application.
// Static files are served directly; all other paths fall back to index.html
// so client-side routing works.
func spaHandler() http.Handler {
	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		panic("embedded web directory missing: " + err.Error())
	}
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Don't serve the SPA for API or WebSocket routes.
		path := r.URL.Path
		if strings.HasPrefix(path, "/v1/") || strings.HasPrefix(path, "/ws/") {
			http.NotFound(w, r)
			return
		}

		// Try to serve the file directly.
		// Strip leading slash to get the fs path.
		fsPath := strings.TrimPrefix(path, "/")
		if fsPath == "" {
			fsPath = "index.html"
		}

		if _, err := fs.Stat(sub, fsPath); err == nil {
			// Cache static assets aggressively (they have content hashes).
			if strings.HasPrefix(fsPath, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			fileServer.ServeHTTP(w, r)
			return
		}

		// File not found — serve index.html for SPA routing.
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}
