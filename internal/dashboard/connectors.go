package dashboard

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/TomiCode/cnsl/internal/connector"
	"github.com/TomiCode/cnsl/internal/dashboard/gen"
	"github.com/TomiCode/cnsl/internal/store"
)

func (s *Server) ListConnectorTypes(_ context.Context, _ gen.ListConnectorTypesRequestObject) (gen.ListConnectorTypesResponseObject, error) {
	types := connector.Types()
	out := make(gen.ListConnectorTypes200JSONResponse, 0, len(types))
	for _, t := range types {
		out = append(out, gen.ConnectorType{
			Key: t.Key(), Name: t.Name(), Description: t.Description(),
			ConfigSchema: toFields(t.ConfigSchema()), ParamsSchema: toFields(t.ParamsSchema()),
		})
	}
	return out, nil
}

func toFields(s connector.Schema) []gen.ConnectorField {
	out := make([]gen.ConnectorField, 0, len(s))
	for _, f := range s {
		gf := gen.ConnectorField{Key: f.Key, Label: f.Label, Kind: gen.ConnectorFieldKind(f.Kind), Default: f.Default}
		if f.Help != "" {
			gf.Help = &f.Help
		}
		if f.Required {
			req := true
			gf.Required = &req
		}
		if len(f.Options) > 0 {
			opts := f.Options
			gf.Options = &opts
		}
		out = append(out, gf)
	}
	return out
}

func (s *Server) ListConnectors(ctx context.Context, _ gen.ListConnectorsRequestObject) (gen.ListConnectorsResponseObject, error) {
	conns, err := s.store.Connectors(ctx)
	if err != nil {
		return nil, err
	}
	usage, err := s.store.ConnectorUsage(ctx)
	if err != nil {
		return nil, err
	}
	out := make(gen.ListConnectors200JSONResponse, 0, len(conns))
	for _, c := range conns {
		out = append(out, toConnector(c, usage[c.ID]))
	}
	return out, nil
}

func toConnector(c store.Connector, bindings int) gen.Connector {
	values, _ := connector.ParseValues(c.Config)
	masked := values
	set := []string{}
	if t, ok := connector.Lookup(c.Type); ok {
		var keys []string
		masked, keys = t.ConfigSchema().MaskSecrets(values)
		set = append(set, keys...)
	}
	return gen.Connector{Id: c.ID, Type: c.Type, Name: c.Name, Config: masked, SecretsSet: set, BindingCount: bindings, UpdatedAt: c.UpdatedAt.Time}
}

func (s *Server) CreateConnector(ctx context.Context, req gen.CreateConnectorRequestObject) (gen.CreateConnectorResponseObject, error) {
	invalid := func(err error) gen.CreateConnectorResponseObject {
		return gen.CreateConnector422ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: invalidProblem(err)}
	}
	if req.Body.Type == nil {
		return invalid(errors.New("type is required")), nil
	}
	t, ok := connector.Lookup(*req.Body.Type)
	if !ok {
		return invalid(fmt.Errorf("unknown connector type %q", *req.Body.Type)), nil
	}
	config, err := t.ConfigSchema().Normalize(connector.Values(req.Body.Config))
	if err != nil {
		return invalid(err), nil
	}
	if config, err = s.connectors.SealConfig(t.Key(), config); err != nil {
		return nil, err
	}
	c := store.Connector{Type: t.Key(), Name: strings.TrimSpace(req.Body.Name), Config: config.JSON()}
	if err := s.store.SaveConnector(ctx, &c); err != nil {
		return nil, err
	}
	return gen.CreateConnector201JSONResponse{ConnectorJSONResponse: gen.ConnectorJSONResponse(toConnector(c, 0))}, nil
}

func (s *Server) UpdateConnector(ctx context.Context, req gen.UpdateConnectorRequestObject) (gen.UpdateConnectorResponseObject, error) {
	c, err := s.store.Connector(ctx, req.Id)
	if errors.Is(err, store.ErrNotFound) {
		return gen.UpdateConnector404ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: notFoundProblem("connector")}, nil
	}
	if err != nil {
		return nil, err
	}
	t, ok := connector.Lookup(c.Type)
	if !ok {
		return gen.UpdateConnector422ApplicationProblemPlusJSONResponse(invalidProblem(fmt.Errorf("unknown connector type %q", c.Type))), nil
	}
	stored, _ := connector.ParseValues(c.Config)
	merged := t.ConfigSchema().KeepSecrets(connector.Values(req.Body.Config), stored)
	config, err := t.ConfigSchema().Normalize(merged)
	if err != nil {
		return gen.UpdateConnector422ApplicationProblemPlusJSONResponse(invalidProblem(err)), nil
	}
	// Kept secrets are still sealed (Seal leaves them alone); new ones get sealed here.
	if config, err = s.connectors.SealConfig(c.Type, config); err != nil {
		return nil, err
	}
	c.Name = strings.TrimSpace(req.Body.Name)
	c.Config = config.JSON()
	if err := s.store.SaveConnector(ctx, &c); err != nil {
		return nil, err
	}
	s.rerenderUsers(ctx, c.ID)
	usage, _ := s.store.ConnectorUsage(ctx)
	return gen.UpdateConnector200JSONResponse{ConnectorJSONResponse: gen.ConnectorJSONResponse(toConnector(c, usage[c.ID]))}, nil
}

func (s *Server) DeleteConnector(ctx context.Context, req gen.DeleteConnectorRequestObject) (gen.DeleteConnectorResponseObject, error) {
	affectedScreens := s.screensUsing(ctx, req.Id)
	err := s.store.DeleteConnector(ctx, req.Id)
	if errors.Is(err, store.ErrNotFound) {
		return gen.DeleteConnector404ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: notFoundProblem("connector")}, nil
	}
	if err != nil {
		return nil, err
	}
	for _, id := range affectedScreens {
		s.scheduler.Request(id, "data")
	}
	return gen.DeleteConnector204Response{}, nil
}

func (s *Server) TestConnector(ctx context.Context, req gen.TestConnectorRequestObject) (gen.TestConnectorResponseObject, error) {
	c, err := s.store.Connector(ctx, req.Id)
	if errors.Is(err, store.ErrNotFound) {
		return gen.TestConnector404ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: notFoundProblem("connector")}, nil
	}
	if err != nil {
		return nil, err
	}
	fail := func(err error, d time.Duration) gen.TestConnectorResponseObject {
		msg := err.Error()
		return gen.TestConnector200JSONResponse{Ok: false, Error: &msg, DurationMs: d.Milliseconds()}
	}
	t, ok := connector.Lookup(c.Type)
	if !ok {
		return fail(fmt.Errorf("unknown connector type %q", c.Type), 0), nil
	}
	in := connector.Values{}
	if req.Body.Params != nil {
		in = *req.Body.Params
	}
	params, err := t.ParamsSchema().Normalize(in)
	if err != nil {
		return fail(err, 0), nil
	}
	config, err := s.connectors.OpenConfig(c.Type, c.Config)
	if err != nil {
		return fail(err, 0), nil
	}
	start := time.Now()
	data, err := s.connectors.Test(ctx, c.Type, config, params)
	if err != nil {
		return fail(err, time.Since(start)), nil
	}
	return gen.TestConnector200JSONResponse{Ok: true, Data: data, DurationMs: time.Since(start).Milliseconds()}, nil
}

func (s *Server) ListBindings(ctx context.Context, req gen.ListBindingsRequestObject) (gen.ListBindingsResponseObject, error) {
	if _, err := s.store.Screen(ctx, req.Id); errors.Is(err, store.ErrNotFound) {
		return gen.ListBindings404ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: notFoundProblem("screen")}, nil
	} else if err != nil {
		return nil, err
	}
	bindings, err := s.store.Bindings(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return gen.ListBindings200JSONResponse(toBindings(bindings)), nil
}

var aliasPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

func (s *Server) ReplaceBindings(ctx context.Context, req gen.ReplaceBindingsRequestObject) (gen.ReplaceBindingsResponseObject, error) {
	invalid := func(format string, args ...any) gen.ReplaceBindingsResponseObject {
		return gen.ReplaceBindings422ApplicationProblemPlusJSONResponse(invalidProblem(fmt.Errorf(format, args...)))
	}
	if _, err := s.store.Screen(ctx, req.Id); errors.Is(err, store.ErrNotFound) {
		return gen.ReplaceBindings404ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: notFoundProblem("screen")}, nil
	} else if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	bindings := make([]store.Binding, 0, len(*req.Body))
	for _, in := range *req.Body {
		if !aliasPattern.MatchString(in.Alias) {
			return invalid("alias %q must start with a letter and use only a-z, 0-9 and _", in.Alias), nil
		}
		if seen[in.Alias] {
			return invalid("alias %q is used twice", in.Alias), nil
		}
		seen[in.Alias] = true
		c, err := s.store.Connector(ctx, in.ConnectorId)
		if errors.Is(err, store.ErrNotFound) {
			return invalid("%s: unknown connector %d", in.Alias, in.ConnectorId), nil
		} else if err != nil {
			return nil, err
		}
		t, ok := connector.Lookup(c.Type)
		if !ok {
			return invalid("%s: unknown connector type %q", in.Alias, c.Type), nil
		}
		params, err := t.ParamsSchema().Normalize(in.Params)
		if err != nil {
			return invalid("%s: %v", in.Alias, err), nil
		}
		b := store.Binding{ConnectorID: c.ID, Alias: in.Alias, Params: params.JSON(), Enabled: in.Enabled,
			TTL: min(max(in.Ttl, connector.MinTTL), connector.MaxTTL)}
		if in.Id != nil {
			b.ID = *in.Id
		}
		if in.RegionKey != nil && *in.RegionKey != "" {
			b.RegionKey = sql.NullString{String: *in.RegionKey, Valid: true}
		}
		bindings = append(bindings, b)
	}
	if err := s.store.ReplaceBindings(ctx, req.Id, bindings); err != nil {
		return nil, err
	}
	s.scheduler.Request(req.Id, "data")
	saved, err := s.store.Bindings(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return gen.ReplaceBindings200JSONResponse(toBindings(saved)), nil
}

func toBindings(bs []store.Binding) []gen.Binding {
	out := make([]gen.Binding, 0, len(bs))
	for _, b := range bs {
		id := b.ID
		params, _ := connector.ParseValues(b.Params)
		gb := gen.Binding{Id: &id, ConnectorId: b.ConnectorID, Alias: b.Alias, Params: params, Ttl: b.TTL, Enabled: b.Enabled}
		if b.RegionKey.Valid {
			key := b.RegionKey.String
			gb.RegionKey = &key
		}
		out = append(out, gb)
	}
	return out
}

func (s *Server) GetScreenData(ctx context.Context, req gen.GetScreenDataRequestObject) (gen.GetScreenDataResponseObject, error) {
	if _, err := s.store.Screen(ctx, req.Id); errors.Is(err, store.ErrNotFound) {
		return gen.GetScreenData404ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: notFoundProblem("screen")}, nil
	} else if err != nil {
		return nil, err
	}
	sample := req.Params.Sample != nil && *req.Params.Sample
	results, err := s.connectors.ResolveAll(ctx, req.Id, sample)
	if err != nil {
		return nil, err
	}
	out := gen.GetScreenData200JSONResponse{}
	for alias, r := range results {
		bd := gen.BindingData{Data: r.Data, Stale: r.Stale, FetchedAt: r.FetchedAt}
		if r.Error != "" {
			msg := r.Error
			bd.Error = &msg
		}
		out[alias] = bd
	}
	return out, nil
}

// screensUsing lists screens with a binding to the connector.
func (s *Server) screensUsing(ctx context.Context, connectorID int64) []int64 {
	all, err := s.store.AllBindings(ctx)
	if err != nil {
		return nil
	}
	seen := map[int64]bool{}
	var out []int64
	for _, b := range all {
		if b.ConnectorID == connectorID && !seen[b.ScreenID] {
			seen[b.ScreenID] = true
			out = append(out, b.ScreenID)
		}
	}
	return out
}

func (s *Server) rerenderUsers(ctx context.Context, connectorID int64) {
	for _, id := range s.screensUsing(ctx, connectorID) {
		s.scheduler.Request(id, "data")
	}
}
