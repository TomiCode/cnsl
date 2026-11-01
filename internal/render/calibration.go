package render

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"image"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/TomiCode/cnsl/internal/imaging"
	"github.com/TomiCode/cnsl/internal/store"
)

// The calibration pattern has two jobs:
//
//   - Measuring: 16 numbered bands drawn with the raw device levels (no curve, no palette), so
//     you can judge how bright each level really looks on the panel.
//   - Checking: two ramps prepared in Go with the calibration under test — one dithered, one with
//     the nearest level only (which shows how many levels are really distinct).
//
// The page is laid out in device pixels with absolute positions so the ramps can be pasted into
// the screenshot at known rectangles.
var calibrationTemplate = template.Must(template.New("calibration").Parse(`<!doctype html>
<html><head><meta charset="utf-8">
<script>window.CNSL_NO_RUNTIME = true</script>
<style>
  @font-face { font-family: "Inter"; src: url("/fonts/Inter.ttf") format("truetype"); }
  html, body { margin: 0; background: #fff; }
  body { position: relative; width: {{ .W }}px; height: {{ .H }}px; font-family: Inter, sans-serif; color: #000; overflow: hidden; }
  .abs { position: absolute; }
  .bands { display: flex; border: {{ .Border }}px solid #000; box-sizing: border-box; }
  .bands div { flex: 1; display: flex; align-items: flex-end; justify-content: center; padding-bottom: {{ .Pad2 }}px;
               font-size: {{ .Small }}px; font-weight: 700; }
  .frame { border: {{ .Border }}px solid #000; box-sizing: border-box; }
</style></head>
<body>
  <div class="abs" style="left:{{ .Pad }}px; top:{{ .Pad }}px; font-size:{{ .Title }}px; font-weight:700">Gray calibration · {{ .Model }}</div>
  <div class="abs" style="left:{{ .Pad }}px; top:{{ .SubY }}px; font-size:{{ .Small }}px">
    The numbered bands are the panel's raw gray levels. Judge how bright each looks and enter it in cnsl.
  </div>
  <div class="abs bands" style="left:{{ .Bands.Min.X }}px; top:{{ .Bands.Min.Y }}px; width:{{ .Bands.Dx }}px; height:{{ .Bands.Dy }}px">
    {{ range .Steps }}<div style="background: rgb({{ .V }},{{ .V }},{{ .V }}); color: {{ .Ink }}">{{ .I }}</div>{{ end }}
  </div>
  <div class="abs" style="left:{{ .Pad }}px; top:{{ .LabelA }}px; font-size:{{ .Small }}px">Ramp dithered with this calibration — should look smooth</div>
  <div class="abs frame" style="left:{{ .FrameA.Min.X }}px; top:{{ .FrameA.Min.Y }}px; width:{{ .FrameA.Dx }}px; height:{{ .FrameA.Dy }}px"></div>
  <div class="abs" style="left:{{ .Pad }}px; top:{{ .LabelB }}px; font-size:{{ .Small }}px">Same ramp, nearest level only — each visible step is a distinct level</div>
  <div class="abs frame" style="left:{{ .FrameB.Min.X }}px; top:{{ .FrameB.Min.Y }}px; width:{{ .FrameB.Dx }}px; height:{{ .FrameB.Dy }}px"></div>
  <div class="abs" style="left:{{ .Pad }}px; top:{{ .FootY }}px; font-size:{{ .Small }}px">{{ .Footer }}</div>
</body></html>`))

type calibrationLayout struct {
	W, H, Pad, Pad2, Border, Title, Small int
	SubY, LabelA, LabelB, FootY           int
	Bands, FrameA, FrameB                 image.Rectangle
}

func newCalibrationLayout(w, h int) calibrationLayout {
	u := float64(min(w, h)) / 100
	px := func(f float64) int { return int(math.Round(f * u)) }
	l := calibrationLayout{W: w, H: h, Pad: px(3), Pad2: px(1), Border: max(2, px(0.2)), Title: px(5), Small: px(2.6)}
	l.SubY = px(9.5)
	l.Bands = image.Rect(l.Pad, px(15), w-l.Pad, px(47))
	l.LabelA = px(50)
	l.FrameA = image.Rect(l.Pad, px(54), w-l.Pad, px(66))
	l.LabelB = px(69)
	l.FrameB = image.Rect(l.Pad, px(73), w-l.Pad, px(85))
	l.FootY = px(90)
	return l
}

type calibrationCache struct {
	mu    sync.Mutex
	files map[string]string // model id + curve → image filename
}

var calibrations = calibrationCache{files: map[string]string{}}

// Calibration renders (or reuses) the calibration pattern for a model with its current
// calibration and returns the stored image's filename.
func (r *Renderer) Calibration(ctx context.Context, modelID int64) (string, error) {
	model, err := r.store.DeviceModel(ctx, modelID)
	if err != nil {
		return "", err
	}
	curve, _ := imaging.ParseToneCurve(model.ToneCurve.String)
	key := fmt.Sprintf("%d|%s", model.ID, curveJSON(curve))

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

// CalibrationPreview renders the pattern with the given calibration without storing it.
func (r *Renderer) CalibrationPreview(ctx context.Context, model store.DeviceModel, curve imaging.ToneCurve) (Result, error) {
	l := newCalibrationLayout(model.Width, model.Height)
	levels := 1 << model.BitDepth

	type step struct {
		I, V int
		Ink  string
	}
	steps := make([]step, levels)
	for i := range steps {
		v := int(math.Round(float64(i) * 255 / float64(levels-1)))
		ink := "#000"
		if i < levels/2 {
			ink = "#fff"
		}
		steps[i] = step{I: i, V: v, Ink: ink}
	}
	measured := "even (not measured)"
	if len(curve.Levels) == levels {
		measured = "measured"
	}

	var b strings.Builder
	if err := calibrationTemplate.Execute(&b, map[string]any{
		"W": l.W, "H": l.H, "Pad": l.Pad, "Pad2": l.Pad2, "Border": l.Border, "Title": l.Title, "Small": l.Small,
		"SubY": l.SubY, "LabelA": l.LabelA, "LabelB": l.LabelB, "FootY": l.FootY,
		"Bands": l.Bands, "FrameA": l.FrameA, "FrameB": l.FrameB,
		"Model": model.Name, "Steps": steps,
		"Footer": fmt.Sprintf("Calibration: gamma %.2f · black %.2f · white %.2f · levels %s", curve.Gamma, curve.Black, curve.White, measured),
	}); err != nil {
		return Result{}, err
	}

	// Render with an identity pipeline (linear curve, even levels) so band i is exactly level i.
	raw := model
	raw.ToneCurve = sql.NullString{}
	res, err := r.renderHTML(ctx, b.String(), raw)
	if err != nil {
		return Result{}, err
	}

	// Paste the ramps prepared with the calibration under test.
	pal := curve.Palette(model.BitDepth)
	inset := func(rc image.Rectangle) image.Rectangle { return rc.Inset(l.Border) }
	pasteRamp(res.Levels, inset(l.FrameA), curve, pal, imaging.DitherFloydSteinberg)
	pasteRamp(res.Levels, inset(l.FrameB), curve, pal, imaging.DitherNone)

	out, err := finish(res.Levels, model.BitDepth, pal)
	out.Raw = res.Raw
	return out, err
}

// pasteRamp draws a black-to-white ramp through the calibration into a rectangle of dst.
func pasteRamp(dst *image.Gray, rc image.Rectangle, curve imaging.ToneCurve, pal imaging.Palette, dither imaging.Dither) {
	w, h := rc.Dx(), rc.Dy()
	if w <= 1 || h <= 0 {
		return
	}
	ramp := image.NewGray(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			ramp.Pix[y*ramp.Stride+x] = uint8(math.Round(float64(x) * 255 / float64(w-1)))
		}
	}
	levels := imaging.QuantizeTo(curve.Apply(ramp), pal, dither)
	for y := range h {
		copy(dst.Pix[(rc.Min.Y+y)*dst.Stride+rc.Min.X:], levels.Pix[y*levels.Stride:y*levels.Stride+w])
	}
}

func curveJSON(c imaging.ToneCurve) string {
	b, _ := json.Marshal(c)
	return string(b)
}
