package admin

import (
	"embed"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var frontendAssets embed.FS

// serveSPA serves the frontend SPA with fallback to index.html for client-side routing.
func (s *AdminServer) serveSPA() http.Handler {
	// Get the dist subdirectory from embedded files
	distFS, err := fs.Sub(frontendAssets, "dist")
	if err != nil {
		s.logger.Error("failed to access frontend assets", "error", err)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Admin UI not available", http.StatusNotFound)
		})
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get the URL path
		urlPath := r.URL.Path
		if urlPath == "" || urlPath == "/" {
			urlPath = "index.html"
		} else {
			// Remove leading slash
			urlPath = strings.TrimPrefix(urlPath, "/")
		}

		// Clean the path
		cleanPath := path.Clean(urlPath)

		// Try to open the file
		file, err := distFS.Open(cleanPath)
		if err != nil {
			// File not found, serve index.html for SPA routing
			cleanPath = "index.html"
			file, err = distFS.Open(cleanPath)
			if err != nil {
				http.Error(w, "Not found", http.StatusNotFound)
				return
			}
		}
		defer file.Close()

		// Get file info for content type detection
		info, err := file.Stat()
		if err != nil {
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}

		// Serve the file
		http.ServeContent(w, r, cleanPath, info.ModTime(), file.(io.ReadSeeker))
	})
}
