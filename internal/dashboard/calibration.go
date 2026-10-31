package dashboard

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"

	"github.com/TomiCode/cnsl/internal/dashboard/gen"
	"github.com/TomiCode/cnsl/internal/device"
	"github.com/TomiCode/cnsl/internal/events"
	"github.com/TomiCode/cnsl/internal/imaging"
	"github.com/TomiCode/cnsl/internal/store"
)

// errInvalidInput marks dashboard-level validation failures (mapped to 422).
var errInvalidInput = device.ErrInvalid

func (s *Server) SetDevicePlaylist(ctx context.Context, req gen.SetDevicePlaylistRequestObject) (gen.SetDevicePlaylistResponseObject, error) {
	d, err := s.devices.AssignPlaylist(ctx, req.Id, req.Body.PlaylistId)
	switch {
	case errors.Is(err, store.ErrNotFound):
		return gen.SetDevicePlaylist404ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: notFoundProblem("device")}, nil
	case errors.Is(err, device.ErrInvalid):
		return gen.SetDevicePlaylist422ApplicationProblemPlusJSONResponse(invalidProblem(err)), nil
	case err != nil:
		return nil, err
	}
	resp, err := s.deviceResponse(ctx, d)
	return gen.SetDevicePlaylist200JSONResponse{DeviceJSONResponse: resp}, err
}

func (s *Server) StartDeviceCalibration(ctx context.Context, req gen.StartDeviceCalibrationRequestObject) (gen.StartDeviceCalibrationResponseObject, error) {
	d, err := s.devices.StartCalibration(ctx, req.Id)
	switch {
	case errors.Is(err, store.ErrNotFound):
		return gen.StartDeviceCalibration404ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: notFoundProblem("device")}, nil
	case errors.Is(err, device.ErrInvalid):
		return gen.StartDeviceCalibration422ApplicationProblemPlusJSONResponse(invalidProblem(err)), nil
	case err != nil:
		return nil, err
	}
	resp, err := s.deviceResponse(ctx, d)
	return gen.StartDeviceCalibration200JSONResponse{DeviceJSONResponse: resp}, err
}

func (s *Server) StopDeviceCalibration(ctx context.Context, req gen.StopDeviceCalibrationRequestObject) (gen.StopDeviceCalibrationResponseObject, error) {
	d, err := s.devices.StopCalibration(ctx, req.Id)
	if errors.Is(err, store.ErrNotFound) {
		return gen.StopDeviceCalibration404ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: notFoundProblem("device")}, nil
	}
	if err != nil {
		return nil, err
	}
	resp, err := s.deviceResponse(ctx, d)
	return gen.StopDeviceCalibration200JSONResponse{DeviceJSONResponse: resp}, err
}

func (s *Server) SetToneCurve(ctx context.Context, req gen.SetToneCurveRequestObject) (gen.SetToneCurveResponseObject, error) {
	curve := fromToneCurve(*req.Body)
	if err := curve.Validate(); err != nil {
		return gen.SetToneCurve422ApplicationProblemPlusJSONResponse(invalidProblem(err)), nil
	}
	m, err := s.store.DeviceModel(ctx, req.Id)
	if errors.Is(err, store.ErrNotFound) {
		return gen.SetToneCurve404ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: notFoundProblem("device model")}, nil
	}
	if err != nil {
		return nil, err
	}
	b, _ := json.Marshal(curve)
	m.ToneCurve = sql.NullString{String: string(b), Valid: true}
	if err := s.store.UpdateToneCurve(ctx, m.ID, m.ToneCurve); err != nil {
		return nil, err
	}

	// Re-render every screen for this model so devices pick up the new grays.
	screens, err := s.store.Screens(ctx)
	if err != nil {
		return nil, err
	}
	for _, sc := range screens {
		if sc.ModelID == m.ID {
			s.scheduler.Request(sc.ID, "calibration")
		}
	}
	s.events.Publish(events.Event{Type: events.ModelUpdated, ModelID: m.ID})
	return gen.SetToneCurve200JSONResponse(toDeviceModel(m)), nil
}

func (s *Server) PreviewToneCurve(ctx context.Context, req gen.PreviewToneCurveRequestObject) (gen.PreviewToneCurveResponseObject, error) {
	curve := fromToneCurve(*req.Body)
	if err := curve.Validate(); err != nil {
		return gen.PreviewToneCurve422ApplicationProblemPlusJSONResponse(invalidProblem(err)), nil
	}
	m, err := s.store.DeviceModel(ctx, req.Id)
	if errors.Is(err, store.ErrNotFound) {
		return gen.PreviewToneCurve404ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: notFoundProblem("device model")}, nil
	}
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, previewTimeout)
	defer cancel()
	res, err := s.renderer.CalibrationPreview(ctx, m, curve)
	if err != nil {
		return gen.PreviewToneCurve503ApplicationProblemPlusJSONResponse(rendererProblem(err)), nil
	}
	return gen.PreviewToneCurve200JSONResponse(toPreview(res)), nil
}

func fromToneCurve(c gen.ToneCurve) imaging.ToneCurve {
	round := func(v float32) float64 { return math.Round(float64(v)*1000) / 1000 } // float32 noise
	return imaging.ToneCurve{Gamma: round(c.Gamma), Black: round(c.Black), White: round(c.White)}
}

func toToneCurve(s sql.NullString) gen.ToneCurve {
	c, _ := imaging.ParseToneCurve(s.String)
	return gen.ToneCurve{Gamma: float32(c.Gamma), Black: float32(c.Black), White: float32(c.White)}
}
