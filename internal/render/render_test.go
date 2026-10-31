package render

import (
	"bytes"
	"context"
	"image"
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

// TestRenderLayoutAndBackground renders a 1Lx2R mashup over a background image (one region
// transparent) and an image screen, in a real headless browser.
func TestRenderLayoutAndBackground(t *testing.T) {
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

	// A synthetic "photo": diagonal gradient with a dark disc.
	uploads := filepath.Join(dir, "uploads")
	_ = os.MkdirAll(uploads, 0o755)
	img := image.NewGray(image.Rect(0, 0, 1200, 900))
	for y := range 900 {
		for x := range 1200 {
			v := uint8((x + y) * 255 / 2100)
			if dx, dy := x-800, y-450; dx*dx+dy*dy < 200*200 {
				v = 30
			}
			img.Pix[y*img.Stride+x] = v
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	_ = os.WriteFile(filepath.Join(uploads, "photo.png"), buf.Bytes(), 0o644)
	asset := store.Asset{Name: "photo", Filename: "photo.png", Mime: "image/png", Width: 1200, Height: 900, Size: int64(buf.Len())}
	if err := st.CreateAsset(ctx, &asset); err != nil {
		t.Fatal(err)
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	browser := NewBrowser("", 1, log)
	defer browser.Close()
	r := NewRenderer(Options{Store: st, Files: imaging.Files{Dir: dir}, Browser: browser, Timezone: time.UTC, Log: log,
		UploadsDir: uploads, CacheDir: filepath.Join(dir, "cache")})
	mux := http.NewServeMux()
	mux.Handle("/internal/render/", r.Handler())
	mux.Handle("/fonts/", http.StripPrefix("/fonts/", assets.FontsHandler()))
	srv := httptest.NewServer(mux)
	defer srv.Close()
	r.baseURL = srv.URL

	model, _ := st.DeviceModelByFirmware(ctx, "x")
	layout, _ := LayoutByKey("1Lx2R")
	bg := Background{Kind: "image", AssetIDs: []int64{asset.ID}, Fit: "cover", Dither: "floyd-steinberg"}
	sc := store.Screen{Name: "Split", ModelID: model.ID, FrameworkVersion: assets.FrameworkVersion, RenderInterval: 900,
		Layout: layout.Key, Background: bg.JSON()}
	regions := ApplyLayout(layout, model, nil)
	regions[0].Transparent = true // the photo shows through the left half
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
	if out := os.Getenv("CNSL_RENDER_OUT"); out != "" {
		_ = os.WriteFile(filepath.Join(out, "layout-device.png"), res.Device, 0o644)
		_ = os.WriteFile(filepath.Join(out, "layout-raw.png"), res.Raw, 0o644)
	}
	if len(res.Errors) > 0 {
		t.Fatalf("template errors: %v", res.Errors)
	}
	// The dark disc (centered at 2/3 width) sits under the opaque right column, the left half shows
	// the gradient; a mid-left sample must be neither paper white nor the framework's surface.
	if v := res.Levels.GrayAt(300, 700).Y; v == 15 {
		t.Fatalf("left region is white at (300,700): background not visible")
	}

	img2 := store.Screen{Name: "Photo", ModelID: model.ID, FrameworkVersion: assets.FrameworkVersion, RenderInterval: 900,
		Kind: "image", Background: Background{Kind: "slideshow", AssetIDs: []int64{asset.ID}, Dither: "ordered"}.JSON()}
	if err := st.CreateScreen(ctx, &img2, nil); err != nil {
		t.Fatal(err)
	}
	ires, err := r.Render(ctx, Request{ScreenID: img2.ID})
	if err != nil {
		t.Fatal(err)
	}
	if b := ires.Levels.Bounds(); b.Dx() != 1872 || b.Dy() != 1404 {
		t.Fatalf("image screen size %v", b)
	}

	if out := os.Getenv("CNSL_RENDER_OUT"); out != "" {
		_ = os.WriteFile(filepath.Join(out, "image-screen-device.png"), ires.Device, 0o644)
	}
}
