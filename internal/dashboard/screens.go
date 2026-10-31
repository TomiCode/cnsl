package dashboard

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/TomiCode/cnsl/internal/assets"
	"github.com/TomiCode/cnsl/internal/dashboard/gen"
	"github.com/TomiCode/cnsl/internal/events"
	"github.com/TomiCode/cnsl/internal/render"
	"github.com/TomiCode/cnsl/internal/store"
	"github.com/TomiCode/cnsl/internal/tmpl"
)

const previewTimeout = 45 * time.Second

func (s *Server) ListScreens(ctx context.Context, _ gen.ListScreensRequestObject) (gen.ListScreensResponseObject, error) {
	screens, err := s.store.Screens(ctx)
	if err != nil {
		return nil, err
	}
	out := make(gen.ListScreens200JSONResponse, 0, len(screens))
	for _, sc := range screens {
		sum, err := s.screenSummary(ctx, sc)
		if err != nil {
			return nil, err
		}
		out = append(out, sum)
	}
	return out, nil
}

func (s *Server) screenSummary(ctx context.Context, sc store.Screen) (gen.ScreenSummary, error) {
	sum := gen.ScreenSummary{
		Id: sc.ID, Name: sc.Name, Kind: gen.ScreenKind(sc.Kind), Layout: sc.Layout, ModelId: sc.ModelID,
		RenderInterval: sc.RenderInterval, UpdatedAt: sc.UpdatedAt.Time,
	}
	if r, err := s.store.LatestRender(ctx, sc.ID, false); err == nil {
		gr := toRender(r)
		sum.LatestRender = &gr
	} else if !errors.Is(err, store.ErrNotFound) {
		return sum, err
	}
	if r, err := s.store.LatestRender(ctx, sc.ID, true); err == nil {
		sum.LatestImage = nullString(r.Filename)
	} else if !errors.Is(err, store.ErrNotFound) {
		return sum, err
	}
	return sum, nil
}

func (s *Server) screenResponse(ctx context.Context, sc store.Screen) (gen.ScreenJSONResponse, error) {
	sum, err := s.screenSummary(ctx, sc)
	if err != nil {
		return gen.ScreenJSONResponse{}, err
	}
	regions, err := s.store.Regions(ctx, sc.ID)
	if err != nil {
		return gen.ScreenJSONResponse{}, err
	}
	bg, _ := render.ParseBackground(sc.Background)
	out := gen.ScreenJSONResponse{
		Id: sum.Id, Name: sum.Name, Kind: sum.Kind, Layout: sum.Layout, ModelId: sum.ModelId,
		RenderInterval: sum.RenderInterval, UpdatedAt: sum.UpdatedAt,
		LatestRender: sum.LatestRender, LatestImage: sum.LatestImage,
		FrameworkVersion: sc.FrameworkVersion, Head: sc.Head, Background: toBackground(bg),
		Regions: make([]gen.Region, 0, len(regions)),
	}
	for _, r := range regions {
		out.Regions = append(out.Regions, gen.Region{Id: r.ID, Key: r.Key, X: r.X, Y: r.Y, W: r.W, H: r.H, Z: r.Z,
			Transparent: r.Transparent, Template: r.Template})
	}
	return out, nil
}

func (s *Server) GetScreen(ctx context.Context, req gen.GetScreenRequestObject) (gen.GetScreenResponseObject, error) {
	sc, err := s.store.Screen(ctx, req.Id)
	if errors.Is(err, store.ErrNotFound) {
		return gen.GetScreen404ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: notFoundProblem("screen")}, nil
	}
	if err != nil {
		return nil, err
	}
	resp, err := s.screenResponse(ctx, sc)
	return gen.GetScreen200JSONResponse{ScreenJSONResponse: resp}, err
}

func (s *Server) CreateScreen(ctx context.Context, req gen.CreateScreenRequestObject) (gen.CreateScreenResponseObject, error) {
	invalid := func(err error) gen.CreateScreenResponseObject {
		return gen.CreateScreen422ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: invalidProblem(err)}
	}
	model, err := s.store.DeviceModel(ctx, req.Body.ModelId)
	if errors.Is(err, store.ErrNotFound) {
		return invalid(fmt.Errorf("unknown device model %d", req.Body.ModelId)), nil
	}
	if err != nil {
		return nil, err
	}
	kind := gen.ScreenKindTemplate
	if req.Body.Kind != nil {
		kind = *req.Body.Kind
	}
	sc := store.Screen{
		Name:             strings.TrimSpace(req.Body.Name),
		Kind:             string(kind),
		Layout:           render.LayoutFull,
		ModelID:          model.ID,
		FrameworkVersion: assets.FrameworkVersion,
		RenderInterval:   900,
	}
	var regions []store.Region
	switch kind {
	case gen.ScreenKindTemplate:
		regions = []store.Region{{Key: "main", W: model.Width, H: model.Height, Template: render.DefaultTemplate}}
	case gen.ScreenKindImage:
		sc.RenderInterval = 3600 // only matters for slideshows
	default:
		return invalid(fmt.Errorf("unknown screen kind %q", kind)), nil
	}
	if err := s.store.CreateScreen(ctx, &sc, regions); err != nil {
		return nil, err
	}
	s.events.Publish(events.Event{Type: events.ScreenUpdated, ScreenID: sc.ID})
	if kind == gen.ScreenKindTemplate {
		s.scheduler.Request(sc.ID, "edit")
	}
	resp, err := s.screenResponse(ctx, sc)
	return gen.CreateScreen201JSONResponse{ScreenJSONResponse: resp}, err
}

func (s *Server) UpdateScreen(ctx context.Context, req gen.UpdateScreenRequestObject) (gen.UpdateScreenResponseObject, error) {
	sc, err := s.store.Screen(ctx, req.Id)
	if errors.Is(err, store.ErrNotFound) {
		return gen.UpdateScreen404ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: notFoundProblem("screen")}, nil
	}
	if err != nil {
		return nil, err
	}
	b := req.Body
	invalid := func(detail string, templateErrs []gen.TemplateError) gen.UpdateScreenResponseObject {
		p := gen.UpdateScreen422ApplicationProblemPlusJSONResponse{
			Title: http.StatusText(http.StatusUnprocessableEntity), Status: http.StatusUnprocessableEntity, Detail: &detail,
		}
		if len(templateErrs) > 0 {
			p.TemplateErrors = &templateErrs
		}
		return p
	}

	model, err := s.store.DeviceModel(ctx, b.ModelId)
	if errors.Is(err, store.ErrNotFound) {
		return invalid(fmt.Sprintf("unknown device model %d", b.ModelId), nil), nil
	}
	if err != nil {
		return nil, err
	}
	bg := fromBackground(b.Background)
	if err := bg.Validate(); err != nil {
		return invalid(err.Error(), nil), nil
	}
	for _, id := range bg.AssetIDs {
		if _, err := s.store.Asset(ctx, id); errors.Is(err, store.ErrNotFound) {
			return invalid(fmt.Sprintf("unknown image %d", id), nil), nil
		} else if err != nil {
			return nil, err
		}
	}
	if sc.Kind == "image" && bg.Kind == "none" {
		return invalid("an image screen needs an image or a slideshow", nil), nil
	}

	var regions []store.Region
	if sc.Kind == "template" {
		layout, ok := render.LayoutByKey(b.Layout)
		if !ok {
			return invalid(fmt.Sprintf("unknown layout %q", b.Layout), nil), nil
		}
		regions, err = s.mergeRegions(ctx, sc, model, layout, b.Regions)
		if err != nil {
			return invalid(err.Error(), nil), nil
		}
		var templateErrs []gen.TemplateError
		for _, r := range regions {
			if err := tmpl.Check(r.Key, r.Template); err != nil {
				templateErrs = append(templateErrs, toTemplateError(err))
			}
		}
		if len(templateErrs) > 0 {
			return invalid("the template has syntax errors", templateErrs), nil
		}
		sc.Layout = layout.Key
	}

	if bg.JSON() != sc.Background {
		_ = s.store.SetBackgroundIndex(ctx, sc.ID, 0) // a new slideshow starts from its first image
	}
	sc.Name = strings.TrimSpace(b.Name)
	sc.ModelID = model.ID
	sc.Head = b.Head
	sc.RenderInterval = b.RenderInterval
	sc.Background = bg.JSON()
	if err := s.store.UpdateScreen(ctx, &sc, regions); err != nil {
		return nil, err
	}
	s.events.Publish(events.Event{Type: events.ScreenUpdated, ScreenID: sc.ID})
	s.scheduler.Request(sc.ID, "edit")
	resp, err := s.screenResponse(ctx, sc)
	return gen.UpdateScreen200JSONResponse{ScreenJSONResponse: resp}, err
}

// mergeRegions turns the submitted regions into the screen's new region list. For preset
// layouts the stored regions are updated from the input (matched by id, then key) and reshaped
// to the layout's slots; for the free layout the input defines the regions and their rectangles.
func (s *Server) mergeRegions(ctx context.Context, sc store.Screen, model store.DeviceModel, layout render.Layout, in []regionInput) ([]store.Region, error) {
	existing, err := s.store.Regions(ctx, sc.ID)
	if err != nil {
		return nil, err
	}
	byID := map[int64]int{}
	for i, r := range existing {
		byID[r.ID] = i
	}
	seen := map[string]bool{}
	for _, r := range in {
		if seen[r.Key] {
			return nil, fmt.Errorf("region key %q is used twice", r.Key)
		}
		seen[r.Key] = true
		if r.Id != nil {
			if _, ok := byID[*r.Id]; !ok {
				return nil, fmt.Errorf("region %d does not belong to this screen", *r.Id)
			}
		}
	}

	if layout.Key == render.LayoutFree {
		out := make([]store.Region, 0, len(in))
		for _, r := range in {
			reg := store.Region{Key: r.Key, Template: r.Template}
			if r.Id != nil {
				reg = existing[byID[*r.Id]]
				reg.Key, reg.Template = r.Key, r.Template
			}
			if r.Transparent != nil {
				reg.Transparent = *r.Transparent
			}
			if r.X != nil && r.Y != nil && r.W != nil && r.H != nil {
				reg.X, reg.Y, reg.W, reg.H = *r.X, *r.Y, *r.W, *r.H
			}
			if r.Z != nil {
				reg.Z = *r.Z
			}
			if reg.W <= 0 || reg.H <= 0 || reg.X < 0 || reg.Y < 0 || reg.X+reg.W > model.Width || reg.Y+reg.H > model.Height {
				return nil, fmt.Errorf("region %q must lie within the %d×%d screen and have a size", reg.Key, model.Width, model.Height)
			}
			out = append(out, reg)
		}
		return out, nil
	}

	// Preset: apply submitted contents to the stored regions, then reshape to the layout.
	merged := append([]store.Region(nil), existing...)
	for _, r := range in {
		idx := -1
		if r.Id != nil {
			idx = byID[*r.Id]
		} else {
			for i, e := range merged {
				if e.Key == r.Key {
					idx = i
				}
			}
		}
		if idx < 0 {
			continue // new regions come from the layout's slots
		}
		merged[idx].Template = r.Template
		if r.Transparent != nil {
			merged[idx].Transparent = *r.Transparent
		}
	}
	return render.ApplyLayout(layout, model, merged), nil
}

func toBackground(b render.Background) gen.Background {
	fit, dither, order := gen.ImageFit(b.Fit), gen.ImageDither(b.Dither), gen.BackgroundOrder(b.Order)
	ids := append([]int64{}, b.AssetIDs...)
	return gen.Background{Kind: gen.BackgroundKind(b.Kind), AssetIds: &ids, Fit: &fit, Dither: &dither, Order: &order}
}

func fromBackground(g gen.Background) render.Background {
	b := render.Background{Kind: string(g.Kind)}
	if g.AssetIds != nil {
		b.AssetIDs = *g.AssetIds
	}
	if g.Fit != nil {
		b.Fit = string(*g.Fit)
	}
	if g.Dither != nil {
		b.Dither = string(*g.Dither)
	}
	if g.Order != nil {
		b.Order = string(*g.Order)
	}
	if b.Kind == "none" {
		b.AssetIDs = nil
	}
	parsed, _ := render.ParseBackground(b.JSON()) // fill defaults
	return parsed
}

func (s *Server) DeleteScreen(ctx context.Context, req gen.DeleteScreenRequestObject) (gen.DeleteScreenResponseObject, error) {
	err := s.store.DeleteScreen(ctx, req.Id)
	if errors.Is(err, store.ErrNotFound) {
		return gen.DeleteScreen404ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: notFoundProblem("screen")}, nil
	}
	if err != nil {
		return nil, err
	}
	s.events.Publish(events.Event{Type: events.ScreenUpdated, ScreenID: req.Id})
	return gen.DeleteScreen204Response{}, nil
}

func (s *Server) PreviewScreen(ctx context.Context, req gen.PreviewScreenRequestObject) (gen.PreviewScreenResponseObject, error) {
	rreq := render.Request{ScreenID: req.Id, Trigger: "preview"}
	if req.Body != nil {
		if req.Body.Templates != nil {
			rreq.Overrides = *req.Body.Templates
		}
		rreq.Head = req.Body.Head
	}
	ctx, cancel := context.WithTimeout(ctx, previewTimeout)
	defer cancel()
	res, err := s.renderer.Render(ctx, rreq)
	switch {
	case errors.Is(err, store.ErrNotFound):
		return gen.PreviewScreen404ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: notFoundProblem("screen")}, nil
	case err != nil:
		return gen.PreviewScreen503ApplicationProblemPlusJSONResponse(rendererProblem(err)), nil
	}
	return gen.PreviewScreen200JSONResponse(toPreview(res)), nil
}

func (s *Server) ListScreenRenders(ctx context.Context, req gen.ListScreenRendersRequestObject) (gen.ListScreenRendersResponseObject, error) {
	if _, err := s.store.Screen(ctx, req.Id); errors.Is(err, store.ErrNotFound) {
		return gen.ListScreenRenders404ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: notFoundProblem("screen")}, nil
	} else if err != nil {
		return nil, err
	}
	rows, err := s.store.Renders(ctx, req.Id, 50)
	if err != nil {
		return nil, err
	}
	out := make(gen.ListScreenRenders200JSONResponse, 0, len(rows))
	for _, r := range rows {
		out = append(out, toRender(r))
	}
	return out, nil
}

func (s *Server) RenderScreen(ctx context.Context, req gen.RenderScreenRequestObject) (gen.RenderScreenResponseObject, error) {
	ctx, cancel := context.WithTimeout(ctx, previewTimeout)
	defer cancel()
	rec, err := s.renderer.RenderAndStore(ctx, req.Id, "manual")
	if errors.Is(err, store.ErrNotFound) {
		return gen.RenderScreen404ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: notFoundProblem("screen")}, nil
	}
	if err != nil {
		return nil, err
	}
	s.events.Publish(events.Event{Type: events.ScreenRendered, ScreenID: req.Id})
	return gen.RenderScreen200JSONResponse(toRender(rec)), nil
}

func toRender(r store.Render) gen.Render {
	return gen.Render{
		Id: r.ID, Status: gen.RenderStatus(r.Status), Filename: nullString(r.Filename), Error: nullString(r.Error),
		DurationMs: r.DurationMS, Trigger: r.Trigger, CreatedAt: r.CreatedAt.Time,
	}
}

func toPreview(res render.Result) gen.Preview {
	p := gen.Preview{
		Raw:        "data:image/png;base64," + base64.StdEncoding.EncodeToString(res.Raw),
		Device:     "data:image/png;base64," + base64.StdEncoding.EncodeToString(res.Device),
		DurationMs: res.Duration.Milliseconds(),
		Errors:     make([]gen.TemplateError, 0, len(res.Errors)),
	}
	for _, e := range res.Errors {
		p.Errors = append(p.Errors, gen.TemplateError{Region: e.Region, Line: e.Line, Column: e.Column, Message: e.Message})
	}
	return p
}

func toTemplateError(err error) gen.TemplateError {
	var te *tmpl.Error
	if errors.As(err, &te) {
		return gen.TemplateError{Region: te.Region, Line: te.Line, Column: te.Column, Message: te.Message}
	}
	return gen.TemplateError{Message: err.Error()}
}

func rendererProblem(err error) gen.Problem {
	detail := "the renderer is unavailable: " + err.Error()
	return gen.Problem{Title: http.StatusText(http.StatusServiceUnavailable), Status: http.StatusServiceUnavailable, Detail: &detail}
}

// regionInput is one entry of ScreenUpdate.regions (an anonymous struct in the generated code).
type regionInput = struct {
	H           *int   `json:"h,omitempty"`
	Id          *int64 `json:"id,omitempty"`
	Key         string `json:"key"`
	Template    string `json:"template"`
	Transparent *bool  `json:"transparent,omitempty"`
	W           *int   `json:"w,omitempty"`
	X           *int   `json:"x,omitempty"`
	Y           *int   `json:"y,omitempty"`
	Z           *int   `json:"z,omitempty"`
}
