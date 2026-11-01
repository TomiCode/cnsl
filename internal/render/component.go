package render

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/TomiCode/cnsl/internal/assets"
	"github.com/TomiCode/cnsl/internal/connector"
	"github.com/TomiCode/cnsl/internal/library"
	"github.com/TomiCode/cnsl/internal/store"
	"github.com/TomiCode/cnsl/internal/tmpl"
)

// ComponentRequest previews a component on its own, without a screen. Component may be unsaved
// (the library editor sends the source being edited).
type ComponentRequest struct {
	ModelID   int64
	Component tmpl.Component
	// View is the framework view size to show it in: full, half_vertical, half_horizontal or quadrant.
	View string
	// Values override input values; inputs without one use their sample, then their default.
	Values map[string]any
	Lang   string // "" = the global setting
}

var previewLayouts = map[string]string{"full": LayoutFull, "half_vertical": "1Lx1R", "half_horizontal": "1Tx1B", "quadrant": "2x2"}

func (r *Renderer) RenderComponent(ctx context.Context, req ComponentRequest) (Result, error) {
	start := time.Now()
	model, err := r.store.DeviceModel(ctx, req.ModelID)
	if err != nil {
		return Result{}, err
	}
	layoutKey, ok := previewLayouts[req.View]
	if !ok {
		layoutKey = LayoutFull
	}
	layout, _ := LayoutByKey(layoutKey)

	lib, err := library.Load(ctx, r.store)
	if err != nil {
		return Result{}, fmt.Errorf("load components: %w", err)
	}
	comp := req.Component
	lib[comp.Key] = comp

	// The call passes every input from the region's data, filled with preview values.
	values := map[string]any{}
	var call strings.Builder
	call.WriteString("{{ component " + strconv.Quote(comp.Key))
	for _, in := range comp.Inputs {
		v, ok := req.Values[in.Name]
		if !ok {
			v, ok = sampleValue(in)
		}
		if !ok {
			continue
		}
		values[in.Name] = v
		call.WriteString(" " + strconv.Quote(in.Name) + " (index .Data " + strconv.Quote(in.Name) + ")")
	}
	call.WriteString(" }}")

	slot := layout.Slots[0]
	regions := make([]store.Region, len(layout.Slots))
	for i, s := range layout.Slots {
		regions[i] = store.Region{Key: s.Key, W: int(s.W * float64(model.Width)), H: int(s.H * float64(model.Height))}
	}
	regions[0].Template = call.String()

	sc := store.Screen{Name: "Component preview", ModelID: model.ID, Layout: layoutKey, FrameworkVersion: assets.FrameworkVersion}
	data := func(key string) (map[string]any, map[string]bool) {
		if key == slot.Key {
			return values, map[string]bool{}
		}
		return map[string]any{}, map[string]bool{}
	}
	lang, err := r.language(ctx, req.Lang)
	if err != nil {
		return Result{}, err
	}
	html, errs := compose(page{screen: sc, model: model, data: data, lib: lib}, regions, tmpl.Context{
		Lang:   lang,
		Now:    time.Now().In(r.tz),
		Screen: tmpl.ScreenInfo{Name: sc.Name, Width: model.Width, Height: model.Height, BitDepth: model.BitDepth},
	})
	res, err := r.renderHTML(ctx, html, model)
	if err != nil {
		return Result{}, err
	}
	res.Errors = errs
	res.Duration = time.Since(start)
	return res, nil
}

// sampleValue is an input's preview value: its own sample, the connector's sample for data
// inputs, then its default.
func sampleValue(in tmpl.Input) (any, bool) {
	if in.Sample != nil {
		return in.Sample, true
	}
	if in.Kind == "data" && in.Connector != "" {
		if typ, ok := connector.Lookup(in.Connector); ok {
			return typ.Sample(connector.Values{}), true
		}
	}
	if in.Default != nil {
		return in.Default, true
	}
	return nil, false
}
