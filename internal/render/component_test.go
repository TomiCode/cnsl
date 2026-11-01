package render

import (
	"bytes"
	"context"
	"image/png"
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

// TestRenderPortrait renders a portrait screen: designed at swapped size, delivered at panel size.
func TestRenderPortrait(t *testing.T) {
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

	model, err := st.DeviceModelByFirmware(ctx, "virtual") // 800×480
	if err != nil {
		t.Fatal(err)
	}
	// A black bar across the top of the designed (portrait) screen.
	sc := store.Screen{Name: "Portrait", ModelID: model.ID, FrameworkVersion: assets.FrameworkVersion, RenderInterval: 900, Rotation: 90}
	regions := []store.Region{{Key: "main", W: 480, H: 800, Template: `<div style="position:fixed;left:0;top:0;width:100vw;height:40px;background:#000"></div>`}}
	if err := st.CreateScreen(ctx, &sc, regions); err != nil {
		t.Fatal(err)
	}
	res, err := r.Render(ctx, Request{ScreenID: sc.ID})
	if err != nil {
		if strings.Contains(err.Error(), "executable file not found") {
			t.Skip("no Chrome/Edge installed")
		}
		t.Fatal(err)
	}
	if b := res.Levels.Bounds(); b.Dx() != 800 || b.Dy() != 480 {
		t.Fatalf("device levels %v, want 800×480", b)
	}
	raw, _ := png.DecodeConfig(bytes.NewReader(res.Raw))
	if raw.Width != 480 || raw.Height != 800 {
		t.Fatalf("screenshot %dx%d, want 480×800", raw.Width, raw.Height)
	}
	// Turned clockwise, the designed top edge is the panel's right edge.
	at := func(x, y int) uint8 { return res.Levels.Pix[y*res.Levels.Stride+x] }
	if at(790, 240) != 0 || at(10, 240) == 0 {
		t.Fatalf("right edge %d, left edge %d: want the bar on the right", at(790, 240), at(10, 240))
	}
}
