// Package dashboard implements the OpenAPI-defined dashboard API (api/openapi.yaml).
package dashboard

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/getkin/kin-openapi/routers"
	middleware "github.com/oapi-codegen/nethttp-middleware"

	"github.com/TomiCode/cnsl/internal/dashboard/gen"
	"github.com/TomiCode/cnsl/internal/store"
)

// BasePath is where the API is mounted; it matches `servers` in the spec.
const BasePath = "/api/v1"

type Server struct {
	version string
	store   *store.Store
}

var _ gen.StrictServerInterface = (*Server)(nil)

func NewServer(version string, st *store.Store) *Server {
	return &Server{version: version, store: st}
}

// Handler returns the API handler for BasePath, with requests validated against the spec.
func (s *Server) Handler() (http.Handler, error) {
	spec, err := gen.GetSwagger()
	if err != nil {
		return nil, fmt.Errorf("load openapi spec: %w", err)
	}

	strict := gen.NewStrictHandlerWithOptions(s, nil, gen.StrictHTTPServerOptions{
		RequestErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			writeProblem(w, http.StatusBadRequest, err.Error())
		},
		ResponseErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			slog.ErrorContext(r.Context(), "api handler failed", "path", r.URL.Path, "err", err)
			writeProblem(w, http.StatusInternalServerError, "")
		},
	})
	routes := gen.HandlerWithOptions(strict, gen.StdHTTPServerOptions{
		BaseURL:    BasePath,
		BaseRouter: http.NewServeMux(),
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			writeProblem(w, http.StatusBadRequest, err.Error())
		},
	})

	validate := middleware.OapiRequestValidatorWithOptions(spec, &middleware.Options{
		Prefix:               BasePath,
		DoNotValidateServers: true,
		ErrorHandlerWithOpts: func(ctx context.Context, err error, w http.ResponseWriter, r *http.Request, opts middleware.ErrorHandlerOpts) {
			status := opts.StatusCode
			if errors.Is(err, routers.ErrMethodNotAllowed) {
				status = http.StatusMethodNotAllowed
			}
			writeProblem(w, status, err.Error())
		},
	})
	return validate(routes), nil
}

func (s *Server) GetHealth(ctx context.Context, _ gen.GetHealthRequestObject) (gen.GetHealthResponseObject, error) {
	h := gen.Health{Status: gen.HealthStatusOk, Version: s.version, Database: gen.HealthDatabaseOk}
	if err := s.store.Ping(ctx); err != nil {
		slog.WarnContext(ctx, "health: database ping failed", "err", err)
		h.Status = gen.HealthStatusDegraded
		h.Database = gen.HealthDatabaseError
		return gen.GetHealth503JSONResponse(h), nil
	}
	return gen.GetHealth200JSONResponse(h), nil
}
