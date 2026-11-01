package render

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TomiCode/cnsl/internal/assets"
	"github.com/TomiCode/cnsl/internal/imaging"
	"github.com/TomiCode/cnsl/internal/library"
	"github.com/TomiCode/cnsl/internal/store"
)

// TestRenderComponents previews every built-in component in a real browser with sample data.
func TestRenderComponents(t *testing.T) {
	if testing.Short() {
		t.Skip("needs a headless browser")
	}
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(ctx, filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := library.Seed(ctx, st); err != nil {
		t.Fatal(err)
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	browser := NewBrowser("", 1, log)
	defer browser.Close()
	r := NewRenderer(Options{Store: st, Files: imaging.Files{Dir: dir}, Browser: browser, Timezone: time.UTC, Log: log})
	mux := http.NewServeMux()
	mux.Handle("/internal/render/", r.Handler())
	mux.Handle("/fonts/", http.StripPrefix("/fonts/", assets.FontsHandler()))
	srv := httptest.NewServer(mux)
	defer srv.Close()
	r.baseURL = srv.URL

	model, err := st.DeviceModelByFirmware(ctx, "x")
	if err != nil {
		t.Fatal(err)
	}
	lib, err := library.Load(ctx, st)
	if err != nil {
		t.Fatal(err)
	}
	for key, comp := range lib {
		view := "full"
		if key == "sensor" {
			view = "quadrant"
		}
		res, err := r.RenderComponent(ctx, ComponentRequest{ModelID: model.ID, Component: comp, View: view})
		if err != nil {
			if strings.Contains(err.Error(), "executable file not found") {
				t.Skip("no Chrome/Edge installed")
			}
			t.Fatalf("%s: %v", key, err)
		}
		if len(res.Errors) > 0 {
			t.Fatalf("%s: template errors: %v", key, res.Errors)
		}
		dark := 0
		for _, v := range res.Levels.Pix {
			if v < 4 {
				dark++
			}
		}
		if dark < 100 {
			t.Errorf("%s rendered (almost) nothing: %d dark pixels", key, dark)
		}
	}
}
