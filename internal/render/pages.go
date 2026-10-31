package render

import (
	"crypto/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/TomiCode/cnsl/internal/assets"
)

// pages holds composed HTML documents for the browser to load, under one-shot random tokens.
type pages struct {
	mu    sync.Mutex
	items map[string]pageEntry
}

type pageEntry struct {
	data        []byte
	contentType string
	expires     time.Time
}

func newPages() *pages {
	return &pages{items: make(map[string]pageEntry)}
}

func (p *pages) put(html string) string {
	return p.putBytes([]byte(html), "text/html; charset=utf-8")
}

// putBytes stores any resource the page needs (e.g. its background image) under a one-shot token.
func (p *pages) putBytes(data []byte, contentType string) string {
	token := rand.Text()
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	for k, v := range p.items {
		if now.After(v.expires) {
			delete(p.items, k)
		}
	}
	p.items[token] = pageEntry{data: data, contentType: contentType, expires: now.Add(time.Minute)}
	return token
}

func (p *pages) take(token string) (pageEntry, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	pg, ok := p.items[token]
	delete(p.items, token)
	return pg, ok && time.Now().Before(pg.expires)
}

// Handler serves what the headless browser loads, mounted at /internal/render/ on the
// dashboard listener: /internal/render/page/{token} and /internal/render/framework/{version}/….
// Fonts live at /fonts/ (the framework CSS hard-codes that path) and are mounted separately.
func (r *Renderer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /internal/render/page/{token}", func(w http.ResponseWriter, req *http.Request) {
		pg, ok := r.pages.take(req.PathValue("token"))
		if !ok {
			http.NotFound(w, req)
			return
		}
		w.Header().Set("Content-Type", pg.contentType)
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(pg.data)
	})
	mux.Handle("GET /internal/render/framework/", http.StripPrefix("/internal/render/framework", assets.FrameworkHandler()))
	mux.Handle("GET /internal/render/images/", http.StripPrefix("/internal/render/images/", http.FileServer(http.Dir(r.files.Dir))))
	return mux
}

func pageURL(base, token string) string {
	return strings.TrimRight(base, "/") + "/internal/render/page/" + token
}
