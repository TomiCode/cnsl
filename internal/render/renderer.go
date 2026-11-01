// Package render turns screens into device images: templates → HTML page → headless browser
// screenshot → tone curve → quantized low-bit PNG.
package render

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"image"
	"image/png"
	"log/slog"
	"strings"
	"time"

	"github.com/TomiCode/cnsl/internal/connector"
	"github.com/TomiCode/cnsl/internal/imaging"
	"github.com/TomiCode/cnsl/internal/store"
	"github.com/TomiCode/cnsl/internal/tmpl"
)

type Renderer struct {
	store   *store.Store
	files   imaging.Files
	browser *Browser
	pages   *pages
	baseURL string // where the browser reaches the dashboard listener
	tz      *time.Location
	log     *slog.Logger

	uploadsDir string // original asset uploads
	cacheDir   string // processed asset variants

	data DataSource
}

// DataSource provides connector data for a screen's region (see connector.Service).
type DataSource interface {
	Resolve(ctx context.Context, screenID int64, regionKey string, sample bool) (map[string]connector.Result, error)
}

type Options struct {
	Store    *store.Store
	Files    imaging.Files
	Browser  *Browser
	BaseURL  string
	Timezone *time.Location
	Log      *slog.Logger

	UploadsDir string
	CacheDir   string
	Data       DataSource
}

func NewRenderer(o Options) *Renderer {
	return &Renderer{store: o.Store, files: o.Files, browser: o.Browser, pages: newPages(),
		baseURL: o.BaseURL, tz: o.Timezone, log: o.Log, uploadsDir: o.UploadsDir, cacheDir: o.CacheDir, data: o.Data}
}

// Request describes one render. Overrides replace region templates by key without saving
// them (editor preview).
type Request struct {
	ScreenID  int64
	Overrides map[string]string
	Head      *string
	Trigger   string
	Sample    bool // use connector sample data instead of real data
}

type Result struct {
	Raw      []byte // browser screenshot (8-bit, before quantization)
	Device   []byte // PNG exactly as a device would receive it
	Panel    []byte // how the panel shows it, using the model's measured gray levels
	Levels   *image.Gray
	BitDepth int
	Errors   []*tmpl.Error
	// DataErrors are connector problems by binding alias (the render still happens, with
	// stale or missing data).
	DataErrors map[string]string
	Duration   time.Duration
}

// Render runs the pipeline without storing anything.
func (r *Renderer) Render(ctx context.Context, req Request) (Result, error) {
	start := time.Now()
	sc, err := r.store.Screen(ctx, req.ScreenID)
	if err != nil {
		return Result{}, err
	}
	model, err := r.store.DeviceModel(ctx, sc.ModelID)
	if err != nil {
		return Result{}, err
	}
	regions, err := r.store.Regions(ctx, sc.ID)
	if err != nil {
		return Result{}, err
	}
	for i := range regions {
		if src, ok := req.Overrides[regions[i].Key]; ok {
			regions[i].Template = src
		}
	}
	if req.Head != nil {
		sc.Head = *req.Head
	}

	// Background layer: one asset, or the slideshow's current one.
	bg, err := ParseBackground(sc.Background)
	if err != nil {
		return Result{}, fmt.Errorf("invalid background: %w", err)
	}
	var bgLevels *image.Gray
	if id, ok := bg.Pick(sc.BackgroundIndex); ok {
		if bgLevels, err = r.assetLevels(ctx, id, model, imaging.Fit(bg.Fit), imaging.Dither(bg.Dither)); err != nil {
			return Result{}, err
		}
	}

	// Image screens are just their background: no browser needed.
	if sc.Kind == "image" {
		if bgLevels == nil {
			return Result{}, errors.New("this image screen has no image yet")
		}
		curve, _ := imaging.ParseToneCurve(model.ToneCurve.String)
		res, err := levelsResult(bgLevels, model.BitDepth, curve.Palette(model.BitDepth))
		res.Duration = time.Since(start)
		return res, err
	}

	bgURL := ""
	if bgLevels != nil {
		curve, _ := imaging.ParseToneCurve(model.ToneCurve.String)
		var buf bytes.Buffer
		if err := png.Encode(&buf, imaging.ForPage(bgLevels, curve, model.BitDepth)); err != nil {
			return Result{}, err
		}
		bgURL = "/internal/render/page/" + r.pages.putBytes(buf.Bytes(), "image/png")
	}

	dataErrors := map[string]string{}
	dataFor := func(regionKey string) (map[string]any, map[string]bool) {
		data, stale := map[string]any{}, map[string]bool{}
		if r.data == nil {
			return data, stale
		}
		results, err := r.data.Resolve(ctx, sc.ID, regionKey, req.Sample)
		if err != nil {
			dataErrors["*"] = err.Error()
			return data, stale
		}
		for alias, res := range results {
			data[alias], stale[alias] = res.Data, res.Stale
			if res.Error != "" {
				dataErrors[alias] = res.Error
			}
		}
		return data, stale
	}

	html, errs := compose(page{screen: sc, model: model, background: bgURL, data: dataFor}, regions, tmpl.Context{
		Now:    time.Now().In(r.tz),
		Screen: tmpl.ScreenInfo{Name: sc.Name, Width: model.Width, Height: model.Height, BitDepth: model.BitDepth},
		Data:   map[string]any{},
		Stale:  map[string]bool{},
	})
	res, err := r.renderHTML(ctx, html, model)
	if err != nil {
		return Result{}, err
	}
	res.Errors = errs
	res.DataErrors = dataErrors
	res.Duration = time.Since(start)
	return res, nil
}

// RenderAndStore renders a screen and records the result. A render with template errors is
// recorded as failed and its image is not stored, so devices keep the last good image.
func (r *Renderer) RenderAndStore(ctx context.Context, screenID int64, trigger string) (store.Render, error) {
	res, err := r.Render(ctx, Request{ScreenID: screenID, Trigger: trigger})
	rec := store.Render{ScreenID: screenID, Trigger: trigger, DurationMS: res.Duration.Milliseconds()}
	switch {
	case errors.Is(err, store.ErrNotFound):
		return rec, err
	case err != nil:
		rec.Status, rec.Error = "error", sql.NullString{String: err.Error(), Valid: true}
	case len(res.Errors) > 0:
		msgs := make([]string, len(res.Errors))
		for i, e := range res.Errors {
			msgs[i] = e.Error()
		}
		rec.Status, rec.Error = "error", sql.NullString{String: strings.Join(msgs, "\n"), Valid: true}
	default:
		name, err := r.files.Save(res.Levels, res.BitDepth)
		if err != nil {
			return rec, err
		}
		rec.Status, rec.Filename = "ok", sql.NullString{String: name, Valid: true}
	}
	if err := r.store.InsertRender(ctx, &rec); err != nil {
		return rec, err
	}
	if rec.Status == "error" {
		r.log.WarnContext(ctx, "screen render failed", "screen", screenID, "trigger", trigger, "err", rec.Error.String)
	}
	if rec.Status == "ok" {
		r.advanceSlideshow(ctx, screenID)
	}
	r.prune(ctx, screenID)
	return rec, nil
}

// advanceSlideshow moves a slideshow background to its next image for the next render.
func (r *Renderer) advanceSlideshow(ctx context.Context, screenID int64) {
	sc, err := r.store.Screen(ctx, screenID)
	if err != nil {
		return
	}
	if bg, err := ParseBackground(sc.Background); err == nil && bg.Kind == "slideshow" && len(bg.AssetIDs) > 1 {
		if err := r.store.SetBackgroundIndex(ctx, screenID, sc.BackgroundIndex+1); err != nil {
			r.log.WarnContext(ctx, "advance slideshow", "screen", screenID, "err", err)
		}
	}
}

// levelsResult packages prepared level indexes as a render result (image screens).
func levelsResult(levels *image.Gray, bitDepth int, pal imaging.Palette) (Result, error) {
	var raw bytes.Buffer
	if err := png.Encode(&raw, imaging.Expand(levels, bitDepth)); err != nil {
		return Result{}, err
	}
	res, err := finish(levels, bitDepth, pal)
	res.Raw = raw.Bytes()
	return res, err
}

// finish encodes level indexes as the device PNG plus the panel emulation for previews.
func finish(levels *image.Gray, bitDepth int, pal imaging.Palette) (Result, error) {
	var device, panel bytes.Buffer
	if err := imaging.EncodePNG(&device, levels, bitDepth); err != nil {
		return Result{}, err
	}
	if err := png.Encode(&panel, pal.Emulate(levels)); err != nil {
		return Result{}, err
	}
	return Result{Device: device.Bytes(), Panel: panel.Bytes(), Levels: levels, BitDepth: bitDepth}, nil
}

func (r *Renderer) prune(ctx context.Context, screenID int64) {
	unused, err := r.store.PruneRenders(ctx, screenID, 20)
	if err != nil {
		r.log.WarnContext(ctx, "prune renders", "screen", screenID, "err", err)
		return
	}
	for _, name := range unused {
		if err := r.files.Remove(name); err != nil {
			r.log.WarnContext(ctx, "remove render image", "file", name, "err", err)
		}
	}
}

// renderHTML screenshots a page for a model and converts it to the model's gray levels.
func (r *Renderer) renderHTML(ctx context.Context, html string, model store.DeviceModel) (Result, error) {
	// The viewport is the panel in device pixels at a device pixel ratio of 1: the framework
	// scales .screen up itself (transform: scale(var(--pixel-ratio)), 1.8 on TRMNL X).
	token := r.pages.put(html)
	raw, err := r.browser.Screenshot(ctx, pageURL(r.baseURL, token), model.Width, model.Height, 1)
	if err != nil {
		return Result{}, err
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		return Result{}, fmt.Errorf("decode screenshot: %w", err)
	}

	gray := imaging.Gray(img)
	if b := gray.Bounds(); b.Dx() != model.Width || b.Dy() != model.Height {
		// Rounding of width/scale can be off by a pixel.
		gray = imaging.Resize(gray, model.Width, model.Height, imaging.FitCover)
	}
	curve, err := imaging.ParseToneCurve(model.ToneCurve.String)
	if err != nil {
		r.log.WarnContext(ctx, "invalid tone curve, using linear", "model", model.Key, "err", err)
	}
	pal := curve.Palette(model.BitDepth)
	levels := imaging.QuantizeTo(curve.Apply(gray), pal, imaging.DitherNone)
	res, err := finish(levels, model.BitDepth, pal)
	res.Raw = raw
	return res, err
}
