// Package web embeds the built dashboard (yarn build → web/dist) and serves it as a SPA.
package web

import (
	"embed"
	"errors"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var dist embed.FS

// Handler serves the embedded SPA. Unknown paths fall back to index.html so client-side
// routes work on reload; hashed assets under /assets/ are cached forever.
func Handler() http.Handler {
	root, _ := fs.Sub(dist, "dist")
	files := http.FileServerFS(root)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "api" || strings.HasPrefix(name, "api/") {
			http.NotFound(w, r) // never answer API paths with the SPA
			return
		}
		if name == "" {
			name = "index.html"
		}
		if _, err := fs.Stat(root, name); errors.Is(err, fs.ErrNotExist) {
			if path.Ext(name) != "" {
				http.NotFound(w, r)
				return
			}
			name = "index.html"
		}

		if name == "index.html" {
			index, err := fs.ReadFile(root, "index.html")
			if err != nil {
				http.Error(w, "dashboard not built: run `yarn build` in web/", http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache")
			_, _ = w.Write(index)
			return
		}
		if strings.HasPrefix(name, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		files.ServeHTTP(w, r)
	})
}
