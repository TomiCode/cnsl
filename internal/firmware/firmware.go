// Package firmware serves the device listener: the endpoints TRMNL firmware calls and the
// rendered images it downloads. Paths and headers are dictated by the firmware, so these
// handlers are hand-written rather than generated from the OpenAPI spec.
package firmware

import (
	"net/http"
)

type Options struct {
	ImagesDir string
	// AllowOrigin is the dashboard origin allowed to call these endpoints from the browser
	// (the virtual device). Empty disables CORS.
	AllowOrigin string
}

func Handler(opts Options) http.Handler {
	mux := http.NewServeMux()

	// /api/setup, /api/display and /api/log arrive in phase 1.

	images := http.StripPrefix("/images/", http.FileServer(http.Dir(opts.ImagesDir)))
	mux.Handle("GET /images/", immutable(images))

	return cors(opts.AllowOrigin, mux)
}

// immutable marks responses as cacheable forever: image filenames are content hashes.
func immutable(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		next.ServeHTTP(w, r)
	})
}

func cors(origin string, next http.Handler) http.Handler {
	if origin == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") == origin {
			h := w.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Access-Control-Allow-Headers", "*")
			h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			h.Add("Vary", "Origin")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
