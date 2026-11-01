// Package dashboard implements the OpenAPI-defined dashboard API (api/openapi.yaml).
package dashboard

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/getkin/kin-openapi/routers"
	middleware "github.com/oapi-codegen/nethttp-middleware"

	"github.com/TomiCode/cnsl/internal/assets"
	"github.com/TomiCode/cnsl/internal/connector"
	"github.com/TomiCode/cnsl/internal/dashboard/gen"
	"github.com/TomiCode/cnsl/internal/device"
	"github.com/TomiCode/cnsl/internal/events"
	"github.com/TomiCode/cnsl/internal/render"
	"github.com/TomiCode/cnsl/internal/store"
	"github.com/TomiCode/cnsl/internal/tmpl"
)

// BasePath is where the API is mounted; it matches `servers` in the spec.
const BasePath = "/api/v1"

type Options struct {
	Version         string
	Store           *store.Store
	Devices         *device.Service
	Events          *events.Broker
	Timezone        *time.Location
	DevicePublicURL string
	Renderer        *render.Renderer
	Scheduler       *render.Scheduler
	UploadsDir      string
	Connectors      *connector.Service
}

type Server struct {
	version         string
	store           *store.Store
	devices         *device.Service
	events          *events.Broker
	tz              *time.Location
	devicePublicURL string
	renderer        *render.Renderer
	scheduler       *render.Scheduler
	uploadsDir      string
	connectors      *connector.Service
}

var _ gen.StrictServerInterface = (*Server)(nil)

func NewServer(opts Options) *Server {
	return &Server{
		version:         opts.Version,
		store:           opts.Store,
		devices:         opts.Devices,
		events:          opts.Events,
		tz:              opts.Timezone,
		devicePublicURL: opts.DevicePublicURL,
		renderer:        opts.Renderer,
		scheduler:       opts.Scheduler,
		uploadsDir:      opts.UploadsDir,
		connectors:      opts.Connectors,
	}
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
			writeValidationProblem(w, status, err)
		},
	})

	mux := http.NewServeMux()
	mux.HandleFunc("GET "+BasePath+"/events", s.Events) // streaming, excluded from codegen
	mux.Handle(BasePath+"/", validate(routes))
	return mux, nil
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

func (s *Server) GetSystem(_ context.Context, _ gen.GetSystemRequestObject) (gen.GetSystemResponseObject, error) {
	return gen.GetSystem200JSONResponse{
		Version:           s.version,
		Timezone:          s.tz.String(),
		DevicePublicUrl:   s.devicePublicURL,
		FrameworkVersion:  assets.FrameworkVersion,
		TemplateFunctions: tmpl.FuncNames(),
		Layouts:           toLayouts(),
		Languages:         toLanguages(),
	}, nil
}

func toLayouts() []gen.Layout {
	out := make([]gen.Layout, 0, len(render.Layouts))
	for _, l := range render.Layouts {
		gl := gen.Layout{Key: l.Key, Name: l.Name}
		gl.Slots = make([]struct {
			H    float32 `json:"h"`
			Key  string  `json:"key"`
			View string  `json:"view"`
			W    float32 `json:"w"`
			X    float32 `json:"x"`
			Y    float32 `json:"y"`
		}, len(l.Slots))
		for i, s := range l.Slots {
			gl.Slots[i].Key, gl.Slots[i].View = s.Key, s.View
			gl.Slots[i].X, gl.Slots[i].Y, gl.Slots[i].W, gl.Slots[i].H = float32(s.X), float32(s.Y), float32(s.W), float32(s.H)
		}
		out = append(out, gl)
	}
	return out
}
