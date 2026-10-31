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
		Id: sc.ID, Name: sc.Name, ModelId: sc.ModelID, RenderInterval: sc.RenderInterval, UpdatedAt: sc.UpdatedAt.Time,
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
	out := gen.ScreenJSONResponse{
		Id: sum.Id, Name: sum.Name, ModelId: sum.ModelId, RenderInterval: sum.RenderInterval, UpdatedAt: sum.UpdatedAt,
		LatestRender: sum.LatestRender, LatestImage: sum.LatestImage,
		FrameworkVersion: sc.FrameworkVersion, Head: sc.Head, Regions: make([]gen.Region, 0, len(regions)),
	}
	for _, r := range regions {
		out.Regions = append(out.Regions, gen.Region{Id: r.ID, Key: r.Key, X: r.X, Y: r.Y, W: r.W, H: r.H, Template: r.Template})
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
	model, err := s.store.DeviceModel(ctx, req.Body.ModelId)
	if errors.Is(err, store.ErrNotFound) {
		return gen.CreateScreen422ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: invalidProblem(fmt.Errorf("unknown device model %d", req.Body.ModelId))}, nil
	}
	if err != nil {
		return nil, err
	}
	sc := store.Screen{
		Name:             strings.TrimSpace(req.Body.Name),
		ModelID:          model.ID,
		FrameworkVersion: assets.FrameworkVersion,
		RenderInterval:   900,
	}
	regions := []store.Region{{Key: "main", W: model.Width, H: model.Height, Template: render.DefaultTemplate}}
	if err := s.store.CreateScreen(ctx, &sc, regions); err != nil {
		return nil, err
	}
	s.events.Publish(events.Event{Type: events.ScreenUpdated, ScreenID: sc.ID})
	s.scheduler.Request(sc.ID, "edit")
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
	regions, err := s.store.Regions(ctx, sc.ID)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]*store.Region, len(regions))
	for i := range regions {
		byID[regions[i].ID] = &regions[i]
	}

	var templateErrs []gen.TemplateError
	for _, in := range b.Regions {
		r, ok := byID[in.Id]
		if !ok {
			return invalid(fmt.Sprintf("region %d does not belong to this screen", in.Id), nil), nil
		}
		r.Template = in.Template
		if err := tmpl.Check(r.Key, in.Template); err != nil {
			templateErrs = append(templateErrs, toTemplateError(err))
		}
	}
	if len(templateErrs) > 0 {
		return invalid("the template has syntax errors", templateErrs), nil
	}

	// Phase 2 screens have one full-size region; keep it full-size when the model changes.
	if model.ID != sc.ModelID && len(regions) == 1 {
		regions[0].X, regions[0].Y, regions[0].W, regions[0].H = 0, 0, model.Width, model.Height
	}
	sc.Name = strings.TrimSpace(b.Name)
	sc.ModelID = model.ID
	sc.Head = b.Head
	sc.RenderInterval = b.RenderInterval
	if err := s.store.UpdateScreen(ctx, &sc, regions); err != nil {
		return nil, err
	}
	s.events.Publish(events.Event{Type: events.ScreenUpdated, ScreenID: sc.ID})
	s.scheduler.Request(sc.ID, "edit")
	resp, err := s.screenResponse(ctx, sc)
	return gen.UpdateScreen200JSONResponse{ScreenJSONResponse: resp}, err
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
