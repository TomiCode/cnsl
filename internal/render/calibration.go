package render

import (
	"context"
	"fmt"
	"html/template"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/TomiCode/cnsl/internal/imaging"
	"github.com/TomiCode/cnsl/internal/store"
)

// The calibration pattern shows 16 input steps, a smooth ramp and text in several grays, all run
// through the model's tone curve, so you can judge on the real panel whether the steps look evenly
// spaced and adjust the curve until they do.
var calibrationTemplate = template.Must(template.New("calibration").Parse(`<!doctype html>
<html><head><meta charset="utf-8">
<script>window.CNSL_NO_RUNTIME = true</script>
<style>
  @font-face { font-family: "Inter"; src: url("/fonts/Inter.ttf") format("truetype"); }
  html, body { margin: 0; background: #fff; }
  body { width: {{ .W }}px; height: {{ .H }}px; box-sizing: border-box; padding: {{ .Pad }}px;
         font-family: Inter, sans-serif; color: #000; display: flex; flex-direction: column; gap: {{ .Gap }}px; }
  h1 { margin: 0; font-size: {{ .Title }}px; font-weight: 700; }
  p { margin: 0; font-size: {{ .Small }}px; }
  .steps { display: flex; height: 28%; border: 2px solid #000; }
  .steps div { flex: 1; display: flex; align-items: flex-end; justify-content: center;
               padding-bottom: 6px; font-size: {{ .Small }}px; font-weight: 700; }
  .ramp { height: 14%; border: 2px solid #000; background: linear-gradient(to right, #000, #fff); }
  .samples { flex: 1; display: grid; grid-template-columns: 1fr 1fr; gap: {{ .Gap }}px; }
  .samples div { display: flex; flex-direction: column; justify-content: center; gap: 4px;
                 padding: 0 {{ .Gap }}px; font-size: {{ .Text }}px; border: 2px solid #000; }
</style></head>
<body>
  <h1>Gray calibration · {{ .Model }}</h1>
  <p>Curve: gamma {{ printf "%.2f" .Curve.Gamma }} · black {{ printf "%.2f" .Curve.Black }} · white {{ printf "%.2f" .Curve.White }}.
     Adjust in cnsl until the 16 steps look evenly spaced and the ramp is smooth.</p>
  <div class="steps">{{ range .Steps }}<div style="background: rgb({{ .V }},{{ .V }},{{ .V }}); color: {{ .Ink }}">{{ .I }}</div>{{ end }}</div>
  <div class="ramp"></div>
  <div class="samples">
    <div style="background:#fff">{{ range .Inks }}<span style="color: rgb({{ . }},{{ . }},{{ . }})">The quick brown fox · {{ . }}</span>{{ end }}</div>
    <div style="background:#888">{{ range .InksOnGray }}<span style="color: rgb({{ . }},{{ . }},{{ . }})">The quick brown fox · {{ . }}</span>{{ end }}</div>
  </div>
</body></html>`))

type calibrationCache struct {
	mu    sync.Mutex
	files map[string]string // model id + curve → image filename
}

var calibrations = calibrationCache{files: map[string]string{}}

// Calibration renders the calibration pattern for a model with its current tone curve and
// returns the stored image's filename. Results are cached per curve.
func (r *Renderer) Calibration(ctx context.Context, modelID int64) (string, error) {
	model, err := r.store.DeviceModel(ctx, modelID)
	if err != nil {
		return "", err
	}
	curve, _ := imaging.ParseToneCurve(model.ToneCurve.String)
	key := fmt.Sprintf("%d|%v", model.ID, curve)

	calibrations.mu.Lock()
	name, ok := calibrations.files[key]
	calibrations.mu.Unlock()
	if ok {
		if _, err := os.Stat(filepath.Join(r.files.Dir, name)); err == nil {
			return name, nil
		}
	}

	res, err := r.CalibrationPreview(ctx, model, curve)
	if err != nil {
		return "", err
	}
	name, err = r.files.Save(res.Levels, res.BitDepth)
	if err != nil {
		return "", err
	}
	calibrations.mu.Lock()
	calibrations.files[key] = name
	calibrations.mu.Unlock()
	return name, nil
}

// CalibrationPreview renders the pattern with the given curve without storing it.
func (r *Renderer) CalibrationPreview(ctx context.Context, model store.DeviceModel, curve imaging.ToneCurve) (Result, error) {
	// The pattern doesn't use the framework, so it is laid out directly in device pixels.
	w, h := float64(model.Width), float64(model.Height)
	unit := math.Min(w, h) / 100

	type step struct {
		I, V int
		Ink  string
	}
	steps := make([]step, 16)
	for i := range steps {
		v := int(math.Round(float64(i) * 255 / 15))
		ink := "#000"
		if v < 128 {
			ink = "#fff"
		}
		steps[i] = step{I: i, V: v, Ink: ink}
	}

	var b strings.Builder
	if err := calibrationTemplate.Execute(&b, map[string]any{
		"W": int(math.Round(w)), "H": int(math.Round(h)),
		"Pad": int(3 * unit), "Gap": int(2 * unit),
		"Title": int(6 * unit), "Small": int(3.2 * unit), "Text": int(4 * unit),
		"Model": model.Name, "Curve": curve, "Steps": steps,
		"Inks": []int{0, 68, 136, 204}, "InksOnGray": []int{0, 68, 204, 255},
	}); err != nil {
		return Result{}, err
	}

	model.ToneCurve.String = mustJSON(curve)
	model.ToneCurve.Valid = true
	return r.renderHTML(ctx, b.String(), model)
}

func mustJSON(c imaging.ToneCurve) string {
	return fmt.Sprintf(`{"gamma":%g,"black":%g,"white":%g}`, c.Gamma, c.Black, c.White)
}
