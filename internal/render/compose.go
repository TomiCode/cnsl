package render

import (
	"fmt"
	"html/template"
	"strings"

	"github.com/TomiCode/cnsl/internal/store"
	"github.com/TomiCode/cnsl/internal/tmpl"
)

// DefaultTemplate is what a new screen starts with.
const DefaultTemplate = `<div class="view view--full">
  <div class="layout layout--col layout--center gap--large">
    <span class="value value--xxxlarge value--tnums">{{ date "15:04" .Now }}</span>
    <span class="label label--large">{{ date "Monday, 2 January" .Now }}</span>
  </div>
  <div class="title_bar">
    <span class="title">{{ .Screen.Name }}</span>
  </div>
</div>
`

var pageTemplate = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<link rel="stylesheet" href="/internal/render/framework/{{ .Framework }}/plugins.css">
<script src="/internal/render/framework/{{ .Framework }}/plugins.js"></script>
<style>
  html, body { margin: 0; padding: 0; background: #fff; overflow: hidden; }
  .cnsl-region { position: absolute; overflow: hidden; }
  .cnsl-error { margin: 8px; padding: 8px; border: 2px solid #000; font: 14px/1.3 monospace; white-space: pre-wrap; background: #fff; }
</style>
{{ .Head }}
</head>
<body class="environment trmnl">
<div class="{{ .ScreenClasses }}">
{{- range .Regions }}
{{- if .Bare }}
{{ .HTML }}
{{- else }}
<section class="cnsl-region {{ .Class }}" data-region="{{ .Key }}" style="{{ .Style }}">{{ .HTML }}</section>
{{- end }}
{{- end }}
</div>
</body>
</html>`))

type composedRegion struct {
	Key   string
	Class string
	Style template.CSS
	HTML  template.HTML
	Bare  bool // a single full-screen region is placed directly in .screen, as the framework expects
}

// compose builds the page for a screen. Region rectangles are in device pixels and are
// converted to CSS pixels with the model's scale. Template errors are rendered in place and
// returned.
func compose(sc store.Screen, model store.DeviceModel, regions []store.Region, ctx tmpl.Context) (string, []*tmpl.Error) {
	var (
		errs []*tmpl.Error
		out  []composedRegion
	)
	full := len(regions) == 1 && regions[0].X == 0 && regions[0].Y == 0 &&
		regions[0].W == model.Width && regions[0].H == model.Height

	for _, r := range regions {
		rctx := ctx
		rctx.Region = tmpl.RegionInfo{Key: r.Key, Width: r.W, Height: r.H}
		html, err := tmpl.Render(r.Key, r.Template, rctx)
		if err != nil {
			te, ok := err.(*tmpl.Error)
			if !ok {
				te = &tmpl.Error{Region: r.Key, Message: err.Error()}
			}
			errs = append(errs, te)
			html = template.HTML(`<div class="cnsl-error">` + template.HTMLEscapeString(te.Error()) + `</div>`)
		}
		s := model.Scale
		out = append(out, composedRegion{
			Key:   r.Key,
			Class: r.Class,
			HTML:  html,
			Bare:  full,
			Style: template.CSS(fmt.Sprintf("left:%.2fpx;top:%.2fpx;width:%.2fpx;height:%.2fpx;z-index:%d",
				float64(r.X)/s, float64(r.Y)/s, float64(r.W)/s, float64(r.H)/s, r.Z)),
		})
	}

	var b strings.Builder
	err := pageTemplate.Execute(&b, map[string]any{
		"Framework":     sc.FrameworkVersion,
		"Head":          template.HTML(sc.Head),
		"ScreenClasses": model.ScreenClasses,
		"Regions":       out,
	})
	if err != nil { // only possible with a broken page template
		panic(err)
	}
	return b.String(), errs
}
