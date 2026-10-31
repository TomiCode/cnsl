// Package assets embeds the vendored trmnl-framework release (see scripts/vendor-framework.sh)
// and serves it to the headless browser that renders screens.
package assets

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// FrameworkVersion is the vendored trmnl-framework release new screens use.
const FrameworkVersion = "3.4.4"

//go:embed framework
var files embed.FS

// FrameworkHandler serves /{version}/plugins.css and /{version}/plugins.js from the gzipped
// release files, with Content-Encoding: gzip so they are never decompressed in Go.
func FrameworkHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		version, name := path.Split(strings.TrimPrefix(r.URL.Path, "/"))
		version = strings.TrimSuffix(version, "/")
		var file, contentType string
		switch name {
		case "plugins.css":
			file, contentType = "plugins.min.css.gz", "text/css; charset=utf-8"
		case "plugins.js":
			file, contentType = "plugins.min.js.gz", "text/javascript; charset=utf-8"
		default:
			http.NotFound(w, r)
			return
		}
		data, err := files.ReadFile(path.Join("framework", version, file))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		_, _ = w.Write(data)
	})
}

// FontsHandler serves the framework fonts; the CSS references them as absolute /fonts/… URLs.
func FontsHandler() http.Handler {
	fonts, _ := fs.Sub(files, "framework/fonts")
	h := http.FileServerFS(fonts)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		h.ServeHTTP(w, r)
	})
}
