package render

import (
	"bytes"
	"context"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TomiCode/cnsl/internal/assets"
	"github.com/TomiCode/cnsl/internal/imaging"
	"github.com/TomiCode/cnsl/internal/store"
)

// TestRenderScreen renders the default template in a real headless browser. It needs Chrome or
// Edge installed (or CNSL_CHROME_URL-style remote use is not covered here) and is skipped with -short.
func TestRenderScreen(t *testing.T) {
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

	model, err := st.DeviceModelByFirmware(ctx, "x")
	if err != nil {
		t.Fatal(err)
	}
	sc := store.Screen{Name: "Clock", ModelID: model.ID, FrameworkVersion: assets.FrameworkVersion, RenderInterval: 900}
	regions := []store.Region{{Key: "main", W: model.Width, H: model.Height, Template: DefaultTemplate}}
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
	if len(res.Errors) > 0 {
		t.Fatalf("template errors: %v", res.Errors)
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(res.Device))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Width != 1872 || cfg.Height != 1404 {
		t.Fatalf("device image %dx%d, want 1872x1404", cfg.Width, cfg.Height)
	}
	// The clock text must have produced dark pixels on the white page.
	dark := 0
	for _, v := range res.Levels.Pix {
		if v < 4 {
			dark++
		}
	}
	if dark < 1000 {
		t.Fatalf("only %d dark pixels; did the page render?", dark)
	}
	t.Logf("rendered in %v", res.Duration)
	if out := os.Getenv("CNSL_RENDER_OUT"); out != "" { // dump images to look at them
		_ = os.WriteFile(filepath.Join(out, "render-raw.png"), res.Raw, 0o644)
		_ = os.WriteFile(filepath.Join(out, "render-device.png"), res.Device, 0o644)
		cal, err := r.CalibrationPreview(ctx, model, imaging.LinearCurve)
		if err != nil {
			t.Fatal(err)
		}
		_ = os.WriteFile(filepath.Join(out, "calibration-device.png"), cal.Device, 0o644)
	}

	// A broken template is reported, not fatal.
	bad := "{{ .Nope.Deeper }}"
	res, err = r.Render(ctx, Request{ScreenID: sc.ID, Overrides: map[string]string{"main": bad}})
	if err != nil || len(res.Errors) != 1 || res.Errors[0].Line != 1 {
		t.Fatalf("broken template: err=%v errors=%v", err, res.Errors)
	}
}
