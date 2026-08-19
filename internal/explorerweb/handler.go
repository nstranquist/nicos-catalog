// Package explorerweb serves the embedded Explorer application with a safe SPA
// fallback for known client routes.
package explorerweb

import (
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"

	explorerassets "github.com/nstranquist/nicos-catalog/explorer"
)

type handler struct{ files fs.FS }

// Handler returns the production embedded application handler.
func Handler() http.Handler { return HandlerFromFS(explorerassets.FS()) }

// HandlerFromFS supports focused tests and static-export browser proof.
func HandlerFromFS(files fs.FS) http.Handler { return &handler{files: files} }

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if strings.Contains(r.URL.Path, "\\") || hasTraversal(r.URL.Path) {
		http.NotFound(w, r)
		return
	}
	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if name == "" || name == "." {
		name = "index.html"
	}
	payload, err := fs.ReadFile(h.files, name)
	if err != nil {
		if strings.HasPrefix(name, "assets/") || isStaticPath(name) {
			http.NotFound(w, r)
			return
		}
		payload, err = fs.ReadFile(h.files, "index.html")
		name = "index.html"
	}
	if err != nil {
		http.NotFound(w, r)
		return
	}
	contentType := mime.TypeByExtension(path.Ext(name))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if name == "index.html" {
		w.Header().Set("Cache-Control", "no-cache")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodGet {
		_, _ = w.Write(payload)
	}
}

func hasTraversal(raw string) bool {
	for _, part := range strings.Split(raw, "/") {
		if part == ".." {
			return true
		}
	}
	return false
}

func isStaticPath(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".css", ".js", ".json", ".map", ".svg", ".png", ".jpg", ".jpeg", ".webp", ".ico", ".txt", ".xml":
		return true
	default:
		return false
	}
}
