package render

import (
	"fmt"
	"html/template"
	"strings"

	"github.com/TomiCode/cnsl/internal/store"
	"github.com/TomiCode/cnsl/internal/tmpl"
)

var pageTemplate = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<link rel="stylesheet" href="/internal/render/framework/{{ .Framework }}/plugins.css">
<script src="/internal/render/framework/{{ .Framework }}/plugins.js"></script>
<style>
  html, body { margin: 0; padding: 0; background: #fff; overflow: hidden; }
  .screen { position: relative; }
  .cnsl-free { position: absolute; overflow: hidden; }
  /* Transparent regions let the background show through. The framework paints view surfaces on
     ::before (and on a runtime .mashup-cell wrapper in some modes), so those are cleared too. */
  .cnsl-transparent, .cnsl-transparent::before, .cnsl-transparent::after, .cnsl-transparent .layout,
  .mashup-cell:has(> .cnsl-transparent) {
    background: transparent !important; background-image: none !important;
  }
  .cnsl-icon { flex-shrink: 0; vertical-align: middle; }
  .cnsl-block { display: contents; }
  [data-cnsl-arrange="grid"] { align-items: center; }
  .cnsl-flow { width: 100%; height: 100%; column-fill: auto; column-gap: var(--cnsl-gap); }
  .cnsl-flow > * { break-inside: avoid; margin-bottom: var(--cnsl-gap); }
  .cnsl-error { margin: 8px; padding: 8px; border: 2px solid #000; font: 14px/1.3 monospace; white-space: pre-wrap; background: #fff; }
{{- if .Background }}
  .cnsl-bg { position: fixed; left: 0; top: 0; z-index: 0; image-rendering: pixelated; }
  .screen { z-index: 1; background: transparent !important; background-image: none !important; }
{{- end }}
</style>
{{ .Head }}
</head>
<body class="environment trmnl">
{{- if .Background }}
<img class="cnsl-bg" src="{{ .Background }}" width="{{ .Width }}" height="{{ .Height }}" alt="">
{{- end }}
<div class="{{ .ScreenClasses }}">
{{- if .Mashup }}
<div class="mashup mashup--{{ .Mashup }}">
{{- end }}
{{- range .Regions }}
{{ .HTML }}
{{- end }}
{{- if .Mashup }}
</div>
{{- end }}
</div>
<script>
// Region arrangements that depend on the content (runs before the framework runtime measures).
(function () {
  // Automatic grids: columns from the item count and the space's shape (6 items, landscape → 3×2).
  document.querySelectorAll('[data-cnsl-grid="auto"]').forEach(function (g) {
    var n = g.children.length, box = (g.parentElement || g).getBoundingClientRect();
    if (!n || !box.height) return;
    var cols = Math.max(1, Math.min(n, Math.round(Math.sqrt(n * box.width / box.height))));
    if (n <= 3 && box.width >= box.height) cols = n;
    g.style.gridTemplateColumns = 'repeat(' + cols + ', minmax(0, 1fr))';
  });
  // Flowing columns: as many as the content needs to fit the height.
  document.querySelectorAll('[data-cnsl-flow]').forEach(function (f) {
    var h = f.clientHeight, total = 0;
    if (!h) return;
    Array.prototype.forEach.call(f.children, function (c) { total += c.getBoundingClientRect().height + parseFloat(getComputedStyle(c).marginBottom || 0); });
    f.style.columnCount = String(Math.max(1, Math.min(6, Math.ceil(total / h))));
  });
})();
</script>
</body>
</html>`))

// page is everything compose needs besides the regions.
type page struct {
	screen     store.Screen
	model      store.DeviceModel
	background string // URL of the prepared background image, or ""
	// data returns the connector data visible to a region (screen-wide plus its own bindings).
	data func(regionKey string) (map[string]any, map[string]bool)
	lib  tmpl.Library // components ({{ component … }}), may be nil
}

// compose builds the page for a screen. Region rectangles are in device pixels and are converted
// to CSS pixels inside .screen with the model's scale. Template errors are rendered in place and
// returned.
func compose(p page, regions []store.Region, ctx tmpl.Context) (string, []*tmpl.Error) {
	layout, ok := LayoutByKey(p.screen.Layout)
	if !ok {
		layout, _ = LayoutByKey(LayoutFull)
	}
	views := map[string]string{}
	for _, s := range layout.Slots {
		views[s.Key] = s.View
	}
	scale := p.model.Scale
	if scale <= 0 {
		scale = 1
	}

	var (
		errs []*tmpl.Error
		out  []struct{ HTML template.HTML }
	)
	for _, r := range ordered(layout, regions) {
		rctx := ctx
		rctx.Region = tmpl.RegionInfo{Key: r.Key, Width: r.W, Height: r.H}
		if p.data != nil {
			rctx.Data, rctx.Stale = p.data(r.Key)
		}
		body, err := tmpl.Render(r.Key, r.Template, rctx, p.lib)
		if err != nil {
			te, ok := err.(*tmpl.Error)
			if !ok {
				te = &tmpl.Error{Region: r.Key, Message: err.Error()}
			}
			errs = append(errs, te)
			body = template.HTML(`<div class="cnsl-error">` + template.HTMLEscapeString(te.Error()) + `</div>`)
		}

		// The region's arrangement wraps the content in the framework layout (and title bar).
		if arr, err := ParseArrange(r.Arrange); err == nil && arr.Mode != "" && !legacyView(r.Template) {
			free := layout.Key == LayoutFree
			body = template.HTML(arr.wrap(body, free))
			if arr.TitleBar != "" && !free {
				title, err := tmpl.Render(r.Key, arr.TitleBar, rctx, p.lib)
				if err != nil {
					te, ok := err.(*tmpl.Error)
					if !ok {
						te = &tmpl.Error{Region: r.Key, Message: err.Error()}
					}
					te.Message, te.Line, te.Column = "title bar: "+te.Message, 0, 0
					errs = append(errs, te)
				}
				body += template.HTML(`<div class="title_bar"><span class="title">`) + title + template.HTML(`</span></div>`)
			}
		}

		classes := strings.TrimSpace(r.Class)
		if r.Transparent {
			classes = strings.TrimSpace(classes + " cnsl-transparent")
		}
		var html string
		switch {
		case layout.Key == LayoutFree:
			html = fmt.Sprintf(`<div class="cnsl-free %s" data-region="%s" style="left:%.2fpx;top:%.2fpx;width:%.2fpx;height:%.2fpx;z-index:%d">%s</div>`,
				template.HTMLEscapeString(classes), template.HTMLEscapeString(r.Key),
				float64(r.X)/scale, float64(r.Y)/scale, float64(r.W)/scale, float64(r.H)/scale, r.Z, body)
		case legacyView(r.Template):
			html = string(body) // the template brings its own view element
		default:
			html = fmt.Sprintf(`<div class="view view--%s %s" data-region="%s">%s</div>`,
				views[r.Key], template.HTMLEscapeString(classes), template.HTMLEscapeString(r.Key), body)
		}
		out = append(out, struct{ HTML template.HTML }{template.HTML(html)})
	}

	mashup := ""
	if layout.Key != LayoutFull && layout.Key != LayoutFree {
		mashup = layout.Key
	}
	var b strings.Builder
	err := pageTemplate.Execute(&b, map[string]any{
		"Framework":     p.screen.FrameworkVersion,
		"Head":          template.HTML(p.screen.Head),
		"ScreenClasses": p.model.ScreenClasses,
		"Mashup":        mashup,
		"Regions":       out,
		"Background":    p.background,
		"Width":         p.model.Width,
		"Height":        p.model.Height,
	})
	if err != nil { // only possible with a broken page template
		panic(err)
	}
	return b.String(), errs
}

// ordered returns regions in the DOM order the layout needs: slot order for presets (the
// framework positions mashup views by order), z then id for free regions.
func ordered(layout Layout, regions []store.Region) []store.Region {
	if len(layout.Slots) == 0 {
		return regions // already sorted by z, id from the store
	}
	byKey := map[string]store.Region{}
	for _, r := range regions {
		byKey[r.Key] = r
	}
	out := make([]store.Region, 0, len(layout.Slots))
	for _, s := range layout.Slots {
		if r, ok := byKey[s.Key]; ok {
			out = append(out, r)
		}
	}
	return out
}
