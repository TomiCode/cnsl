package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/TomiCode/cnsl/internal/dashboard/gen"
	"github.com/TomiCode/cnsl/internal/library"
	"github.com/TomiCode/cnsl/internal/render"
	"github.com/TomiCode/cnsl/internal/store"
	"github.com/TomiCode/cnsl/internal/tmpl"
)

func conflictProblem(detail string) gen.Problem {
	return gen.Problem{Title: http.StatusText(http.StatusConflict), Status: http.StatusConflict, Detail: &detail}
}

func templateProblem(detail string, errs []gen.TemplateError) gen.TemplateProblemApplicationProblemPlusJSONResponse {
	p := gen.TemplateProblemApplicationProblemPlusJSONResponse{
		Title: http.StatusText(http.StatusUnprocessableEntity), Status: http.StatusUnprocessableEntity, Detail: &detail,
	}
	if len(errs) > 0 {
		p.TemplateErrors = &errs
	}
	return p
}

func (s *Server) ListComponents(ctx context.Context, _ gen.ListComponentsRequestObject) (gen.ListComponentsResponseObject, error) {
	rows, err := s.store.Components(ctx)
	if err != nil {
		return nil, err
	}
	out := make(gen.ListComponents200JSONResponse, 0, len(rows))
	for _, c := range rows {
		gc, err := s.toComponent(ctx, c)
		if err != nil {
			return nil, err
		}
		out = append(out, gc)
	}
	return out, nil
}

func (s *Server) toComponent(ctx context.Context, c store.Component) (gen.Component, error) {
	used, err := s.store.ScreensUsingComponent(ctx, c.Key)
	if err != nil {
		return gen.Component{}, err
	}
	inputs, err := library.ParseInputs(c.Inputs)
	if err != nil {
		return gen.Component{}, err
	}
	params := make([]gen.ComponentParam, 0, len(inputs))
	for _, in := range inputs {
		params = append(params, gen.ComponentParam{Name: in.Name, Kind: gen.ComponentParamKind(in.Kind),
			Label: optional(in.Label), Connector: optional(in.Connector), Help: optional(in.Help),
			Default: in.Default, Sample: in.Sample})
	}
	if used == nil {
		used = []int64{}
	}
	return gen.Component{Id: c.ID, Key: c.Key, Name: c.Name, Description: c.Description, Source: c.Source,
		Inputs: params, Builtin: c.Builtin, UsedBy: used, UpdatedAt: c.UpdatedAt.Time}, nil
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// componentFromInput validates a component body. A non-nil problem means the input is invalid.
func componentFromInput(in gen.ComponentInput) (tmpl.Component, string, *gen.TemplateProblemApplicationProblemPlusJSONResponse) {
	key := strings.TrimSpace(in.Key)
	inputs := make([]tmpl.Input, 0, len(in.Inputs))
	for _, p := range in.Inputs {
		inputs = append(inputs, tmpl.Input{Name: p.Name, Kind: string(p.Kind), Label: deref(p.Label),
			Connector: deref(p.Connector), Help: deref(p.Help), Default: p.Default, Sample: p.Sample})
	}
	if err := tmpl.ValidateInputs(inputs); err != nil {
		p := templateProblem(err.Error(), nil)
		return tmpl.Component{}, "", &p
	}
	if err := tmpl.Check(key, in.Source); err != nil {
		p := templateProblem("the template has syntax errors", []gen.TemplateError{toTemplateError(err)})
		return tmpl.Component{}, "", &p
	}
	raw, err := json.Marshal(inputs)
	if err != nil {
		p := templateProblem(err.Error(), nil)
		return tmpl.Component{}, "", &p
	}
	return tmpl.Component{Key: key, Source: in.Source, Inputs: inputs}, string(raw), nil
}

func (s *Server) CreateComponent(ctx context.Context, req gen.CreateComponentRequestObject) (gen.CreateComponentResponseObject, error) {
	comp, inputs, problem := componentFromInput(*req.Body)
	if problem != nil {
		return gen.CreateComponent422ApplicationProblemPlusJSONResponse{TemplateProblemApplicationProblemPlusJSONResponse: *problem}, nil
	}
	if _, err := s.store.ComponentByKey(ctx, comp.Key); err == nil {
		return gen.CreateComponent422ApplicationProblemPlusJSONResponse{TemplateProblemApplicationProblemPlusJSONResponse: templateProblem(fmt.Sprintf("the key %q is already used", comp.Key), nil)}, nil
	}
	c := store.Component{Key: comp.Key, Name: strings.TrimSpace(req.Body.Name), Description: deref(req.Body.Description),
		Source: comp.Source, Inputs: inputs}
	if err := s.store.SaveComponent(ctx, &c); err != nil {
		return nil, err
	}
	gc, err := s.toComponent(ctx, c)
	for _, id := range gc.UsedBy { // screens that called it before it existed
		s.scheduler.Request(id, "component")
	}
	return gen.CreateComponent201JSONResponse{ComponentJSONResponse: gen.ComponentJSONResponse(gc)}, err
}

func (s *Server) GetComponent(ctx context.Context, req gen.GetComponentRequestObject) (gen.GetComponentResponseObject, error) {
	c, err := s.store.Component(ctx, req.Id)
	if errors.Is(err, store.ErrNotFound) {
		return gen.GetComponent404ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: notFoundProblem("component")}, nil
	}
	if err != nil {
		return nil, err
	}
	gc, err := s.toComponent(ctx, c)
	return gen.GetComponent200JSONResponse{ComponentJSONResponse: gen.ComponentJSONResponse(gc)}, err
}

func (s *Server) UpdateComponent(ctx context.Context, req gen.UpdateComponentRequestObject) (gen.UpdateComponentResponseObject, error) {
	c, err := s.store.Component(ctx, req.Id)
	if errors.Is(err, store.ErrNotFound) {
		return gen.UpdateComponent404ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: notFoundProblem("component")}, nil
	}
	if err != nil {
		return nil, err
	}
	if c.Builtin {
		return gen.UpdateComponent409ApplicationProblemPlusJSONResponse(conflictProblem("built-in components are read-only; duplicate it to make changes")), nil
	}
	comp, inputs, problem := componentFromInput(*req.Body)
	if problem != nil {
		return gen.UpdateComponent422ApplicationProblemPlusJSONResponse{TemplateProblemApplicationProblemPlusJSONResponse: *problem}, nil
	}
	used, err := s.store.ScreensUsingComponent(ctx, c.Key)
	if err != nil {
		return nil, err
	}
	if comp.Key != c.Key {
		if len(used) > 0 {
			return gen.UpdateComponent409ApplicationProblemPlusJSONResponse(conflictProblem(fmt.Sprintf("%d screen(s) call this component by its key; change them first", len(used)))), nil
		}
		if _, err := s.store.ComponentByKey(ctx, comp.Key); err == nil {
			return gen.UpdateComponent422ApplicationProblemPlusJSONResponse{TemplateProblemApplicationProblemPlusJSONResponse: templateProblem(fmt.Sprintf("the key %q is already used", comp.Key), nil)}, nil
		}
	}
	c.Key, c.Name, c.Description = comp.Key, strings.TrimSpace(req.Body.Name), deref(req.Body.Description)
	c.Source, c.Inputs = comp.Source, inputs
	if err := s.store.SaveComponent(ctx, &c); err != nil {
		return nil, err
	}
	for _, id := range used {
		s.scheduler.Request(id, "component")
	}
	gc, err := s.toComponent(ctx, c)
	return gen.UpdateComponent200JSONResponse{ComponentJSONResponse: gen.ComponentJSONResponse(gc)}, err
}

func (s *Server) DeleteComponent(ctx context.Context, req gen.DeleteComponentRequestObject) (gen.DeleteComponentResponseObject, error) {
	c, err := s.store.Component(ctx, req.Id)
	if errors.Is(err, store.ErrNotFound) {
		return gen.DeleteComponent404ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: notFoundProblem("component")}, nil
	}
	if err != nil {
		return nil, err
	}
	if c.Builtin {
		return gen.DeleteComponent409ApplicationProblemPlusJSONResponse(conflictProblem("built-in components cannot be deleted")), nil
	}
	used, err := s.store.ScreensUsingComponent(ctx, c.Key)
	if err != nil {
		return nil, err
	}
	if len(used) > 0 {
		return gen.DeleteComponent409ApplicationProblemPlusJSONResponse(conflictProblem(fmt.Sprintf("still used by %d screen(s)", len(used)))), nil
	}
	if err := s.store.DeleteComponent(ctx, c.ID); err != nil {
		return nil, err
	}
	return gen.DeleteComponent204Response{}, nil
}

func (s *Server) DuplicateComponent(ctx context.Context, req gen.DuplicateComponentRequestObject) (gen.DuplicateComponentResponseObject, error) {
	c, err := s.store.Component(ctx, req.Id)
	if errors.Is(err, store.ErrNotFound) {
		return gen.DuplicateComponent404ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: notFoundProblem("component")}, nil
	}
	if err != nil {
		return nil, err
	}
	key := c.Key + "-copy"
	for i := 2; ; i++ {
		if _, err := s.store.ComponentByKey(ctx, key); errors.Is(err, store.ErrNotFound) {
			break
		}
		key = fmt.Sprintf("%s-copy-%d", c.Key, i)
	}
	cp := store.Component{Key: key, Name: c.Name + " (copy)", Description: c.Description, Source: c.Source, Inputs: c.Inputs}
	if err := s.store.SaveComponent(ctx, &cp); err != nil {
		return nil, err
	}
	gc, err := s.toComponent(ctx, cp)
	return gen.DuplicateComponent201JSONResponse{ComponentJSONResponse: gen.ComponentJSONResponse(gc)}, err
}

func (s *Server) PreviewComponent(ctx context.Context, req gen.PreviewComponentRequestObject) (gen.PreviewComponentResponseObject, error) {
	comp, _, problem := componentFromInput(req.Body.Component)
	if problem != nil {
		return gen.PreviewComponent422ApplicationProblemPlusJSONResponse{TemplateProblemApplicationProblemPlusJSONResponse: *problem}, nil
	}
	rreq := render.ComponentRequest{ModelID: req.Body.ModelId, Component: comp, View: "full"}
	if req.Body.View != nil {
		rreq.View = string(*req.Body.View)
	}
	if req.Body.Values != nil {
		rreq.Values = *req.Body.Values
	}
	ctx, cancel := context.WithTimeout(ctx, previewTimeout)
	defer cancel()
	res, err := s.renderer.RenderComponent(ctx, rreq)
	switch {
	case errors.Is(err, store.ErrNotFound):
		return gen.PreviewComponent404ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: notFoundProblem("device model")}, nil
	case err != nil:
		return gen.PreviewComponent503ApplicationProblemPlusJSONResponse(rendererProblem(err)), nil
	}
	return gen.PreviewComponent200JSONResponse(toPreview(res)), nil
}

func toSnippet(sn store.Snippet) gen.Snippet {
	return gen.Snippet{Id: sn.ID, Prefix: sn.Prefix, Name: sn.Name, Description: sn.Description, Body: sn.Body,
		Builtin: sn.Builtin, UpdatedAt: sn.UpdatedAt.Time}
}

func (s *Server) ListSnippets(ctx context.Context, _ gen.ListSnippetsRequestObject) (gen.ListSnippetsResponseObject, error) {
	rows, err := s.store.Snippets(ctx)
	if err != nil {
		return nil, err
	}
	out := make(gen.ListSnippets200JSONResponse, 0, len(rows))
	for _, sn := range rows {
		out = append(out, toSnippet(sn))
	}
	return out, nil
}

// prefixTaken reports whether another snippet uses the prefix.
func (s *Server) prefixTaken(ctx context.Context, prefix string, id int64) (bool, error) {
	rows, err := s.store.Snippets(ctx)
	if err != nil {
		return false, err
	}
	for _, sn := range rows {
		if sn.Prefix == prefix && sn.ID != id {
			return true, nil
		}
	}
	return false, nil
}

func (s *Server) CreateSnippet(ctx context.Context, req gen.CreateSnippetRequestObject) (gen.CreateSnippetResponseObject, error) {
	b := req.Body
	if taken, err := s.prefixTaken(ctx, b.Prefix, 0); err != nil {
		return nil, err
	} else if taken {
		return gen.CreateSnippet422ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: invalidProblem(fmt.Errorf("the prefix %q is already used", b.Prefix))}, nil
	}
	sn := store.Snippet{Prefix: b.Prefix, Name: strings.TrimSpace(b.Name), Description: deref(b.Description), Body: b.Body}
	if err := s.store.SaveSnippet(ctx, &sn); err != nil {
		return nil, err
	}
	return gen.CreateSnippet201JSONResponse{SnippetJSONResponse: gen.SnippetJSONResponse(toSnippet(sn))}, nil
}

func (s *Server) UpdateSnippet(ctx context.Context, req gen.UpdateSnippetRequestObject) (gen.UpdateSnippetResponseObject, error) {
	sn, err := s.store.Snippet(ctx, req.Id)
	if errors.Is(err, store.ErrNotFound) {
		return gen.UpdateSnippet404ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: notFoundProblem("snippet")}, nil
	}
	if err != nil {
		return nil, err
	}
	if sn.Builtin {
		return gen.UpdateSnippet409ApplicationProblemPlusJSONResponse(conflictProblem("built-in snippets are read-only")), nil
	}
	b := req.Body
	if taken, err := s.prefixTaken(ctx, b.Prefix, sn.ID); err != nil {
		return nil, err
	} else if taken {
		return gen.UpdateSnippet422ApplicationProblemPlusJSONResponse(invalidProblem(fmt.Errorf("the prefix %q is already used", b.Prefix))), nil
	}
	sn.Prefix, sn.Name, sn.Description, sn.Body = b.Prefix, strings.TrimSpace(b.Name), deref(b.Description), b.Body
	if err := s.store.SaveSnippet(ctx, &sn); err != nil {
		return nil, err
	}
	return gen.UpdateSnippet200JSONResponse{SnippetJSONResponse: gen.SnippetJSONResponse(toSnippet(sn))}, nil
}

func (s *Server) DeleteSnippet(ctx context.Context, req gen.DeleteSnippetRequestObject) (gen.DeleteSnippetResponseObject, error) {
	sn, err := s.store.Snippet(ctx, req.Id)
	if errors.Is(err, store.ErrNotFound) {
		return gen.DeleteSnippet404ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: notFoundProblem("snippet")}, nil
	}
	if err != nil {
		return nil, err
	}
	if sn.Builtin {
		return gen.DeleteSnippet409ApplicationProblemPlusJSONResponse(conflictProblem("built-in snippets cannot be deleted")), nil
	}
	if err := s.store.DeleteSnippet(ctx, sn.ID); err != nil {
		return nil, err
	}
	return gen.DeleteSnippet204Response{}, nil
}
