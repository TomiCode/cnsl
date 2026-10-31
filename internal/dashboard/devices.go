package dashboard

import (
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif" // registered decoders for uploads
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"strings"

	"github.com/TomiCode/cnsl/internal/dashboard/gen"
	"github.com/TomiCode/cnsl/internal/device"
	"github.com/TomiCode/cnsl/internal/imaging"
	"github.com/TomiCode/cnsl/internal/store"
)

const maxUploadBytes = 25 << 20

func (s *Server) ListDeviceModels(ctx context.Context, _ gen.ListDeviceModelsRequestObject) (gen.ListDeviceModelsResponseObject, error) {
	models, err := s.store.DeviceModels(ctx)
	if err != nil {
		return nil, err
	}
	out := make(gen.ListDeviceModels200JSONResponse, 0, len(models))
	for _, m := range models {
		out = append(out, toDeviceModel(m))
	}
	return out, nil
}

func (s *Server) ListDevices(ctx context.Context, _ gen.ListDevicesRequestObject) (gen.ListDevicesResponseObject, error) {
	devices, err := s.store.Devices(ctx)
	if err != nil {
		return nil, err
	}
	latest, err := s.store.LatestTelemetry(ctx)
	if err != nil {
		return nil, err
	}
	out := make(gen.ListDevices200JSONResponse, 0, len(devices))
	for _, d := range devices {
		var t *store.Telemetry
		if l, ok := latest[d.ID]; ok {
			t = &l
		}
		out = append(out, toDevice(d, t))
	}
	return out, nil
}

// deviceResponse loads the device's latest telemetry and converts it for the API.
func (s *Server) deviceResponse(ctx context.Context, d store.Device) (gen.DeviceJSONResponse, error) {
	latest, err := s.store.LatestTelemetry(ctx, d.ID)
	if err != nil {
		return gen.DeviceJSONResponse{}, err
	}
	var t *store.Telemetry
	if l, ok := latest[d.ID]; ok {
		t = &l
	}
	return gen.DeviceJSONResponse(toDevice(d, t)), nil
}

func (s *Server) GetDevice(ctx context.Context, req gen.GetDeviceRequestObject) (gen.GetDeviceResponseObject, error) {
	d, err := s.store.Device(ctx, req.Id)
	if errors.Is(err, store.ErrNotFound) {
		return gen.GetDevice404ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: notFoundProblem("device")}, nil
	}
	if err != nil {
		return nil, err
	}
	resp, err := s.deviceResponse(ctx, d)
	return gen.GetDevice200JSONResponse{DeviceJSONResponse: resp}, err
}

func (s *Server) UpdateDevice(ctx context.Context, req gen.UpdateDeviceRequestObject) (gen.UpdateDeviceResponseObject, error) {
	b := req.Body
	d, err := s.devices.UpdateSettings(ctx, req.Id, device.Settings{
		Label:       b.Label,
		ModelID:     b.ModelId,
		RefreshRate: b.RefreshRate,
		SleepStart:  b.SleepStart,
		SleepEnd:    b.SleepEnd,
	})
	switch {
	case errors.Is(err, store.ErrNotFound):
		return gen.UpdateDevice404ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: notFoundProblem("device")}, nil
	case errors.Is(err, device.ErrInvalid):
		return gen.UpdateDevice422ApplicationProblemPlusJSONResponse(invalidProblem(err)), nil
	case err != nil:
		return nil, err
	}
	resp, err := s.deviceResponse(ctx, d)
	return gen.UpdateDevice200JSONResponse{DeviceJSONResponse: resp}, err
}

func (s *Server) DeleteDevice(ctx context.Context, req gen.DeleteDeviceRequestObject) (gen.DeleteDeviceResponseObject, error) {
	err := s.devices.Delete(ctx, req.Id)
	if errors.Is(err, store.ErrNotFound) {
		return gen.DeleteDevice404ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: notFoundProblem("device")}, nil
	}
	if err != nil {
		return nil, err
	}
	return gen.DeleteDevice204Response{}, nil
}

func (s *Server) ApproveDevice(ctx context.Context, req gen.ApproveDeviceRequestObject) (gen.ApproveDeviceResponseObject, error) {
	d, err := s.devices.Approve(ctx, req.Id)
	switch {
	case errors.Is(err, store.ErrNotFound):
		return gen.ApproveDevice404ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: notFoundProblem("device")}, nil
	case errors.Is(err, device.ErrInvalid):
		return gen.ApproveDevice422ApplicationProblemPlusJSONResponse(invalidProblem(err)), nil
	case err != nil:
		return nil, err
	}
	resp, err := s.deviceResponse(ctx, d)
	return gen.ApproveDevice200JSONResponse{DeviceJSONResponse: resp}, err
}

func (s *Server) DisableDevice(ctx context.Context, req gen.DisableDeviceRequestObject) (gen.DisableDeviceResponseObject, error) {
	d, err := s.devices.Disable(ctx, req.Id)
	if errors.Is(err, store.ErrNotFound) {
		return gen.DisableDevice404ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: notFoundProblem("device")}, nil
	}
	if err != nil {
		return nil, err
	}
	resp, err := s.deviceResponse(ctx, d)
	return gen.DisableDevice200JSONResponse{DeviceJSONResponse: resp}, err
}

func (s *Server) SetDeviceImage(ctx context.Context, req gen.SetDeviceImageRequestObject) (gen.SetDeviceImageResponseObject, error) {
	invalid := func(format string, args ...any) gen.SetDeviceImageResponseObject {
		return gen.SetDeviceImage422ApplicationProblemPlusJSONResponse(invalidProblem(fmt.Errorf(format, args...)))
	}

	var (
		img    image.Image
		fit    = imaging.FitContain
		dither = imaging.DitherFloydSteinberg
	)
	for {
		part, err := req.Body.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return invalid("read upload: %v", err), nil
		}
		switch part.FormName() {
		case "file":
			img, _, err = image.Decode(io.LimitReader(part, maxUploadBytes))
			if err != nil {
				return invalid("unsupported or broken image (PNG, JPEG or GIF): %v", err), nil
			}
		case "fit":
			v, _ := io.ReadAll(io.LimitReader(part, 64))
			fit = imaging.Fit(strings.TrimSpace(string(v)))
		case "dither":
			v, _ := io.ReadAll(io.LimitReader(part, 64))
			dither = imaging.Dither(strings.TrimSpace(string(v)))
		}
		part.Close()
	}
	if img == nil {
		return invalid("missing file"), nil
	}
	if fit != imaging.FitContain && fit != imaging.FitCover {
		return invalid("fit must be contain or cover"), nil
	}
	if dither != imaging.DitherNone && dither != imaging.DitherFloydSteinberg {
		return invalid("dither must be none or floyd-steinberg"), nil
	}

	d, err := s.devices.SetImage(ctx, req.Id, img, fit, dither)
	switch {
	case errors.Is(err, store.ErrNotFound):
		return gen.SetDeviceImage404ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: notFoundProblem("device")}, nil
	case errors.Is(err, device.ErrInvalid):
		return gen.SetDeviceImage422ApplicationProblemPlusJSONResponse(invalidProblem(err)), nil
	case err != nil:
		return nil, err
	}
	resp, err := s.deviceResponse(ctx, d)
	return gen.SetDeviceImage200JSONResponse{DeviceJSONResponse: resp}, err
}

func (s *Server) ClearDeviceImage(ctx context.Context, req gen.ClearDeviceImageRequestObject) (gen.ClearDeviceImageResponseObject, error) {
	d, err := s.devices.ClearImage(ctx, req.Id)
	if errors.Is(err, store.ErrNotFound) {
		return gen.ClearDeviceImage404ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: notFoundProblem("device")}, nil
	}
	if err != nil {
		return nil, err
	}
	resp, err := s.deviceResponse(ctx, d)
	return gen.ClearDeviceImage200JSONResponse{DeviceJSONResponse: resp}, err
}

func (s *Server) ListDeviceLogs(ctx context.Context, req gen.ListDeviceLogsRequestObject) (gen.ListDeviceLogsResponseObject, error) {
	if _, err := s.store.Device(ctx, req.Id); errors.Is(err, store.ErrNotFound) {
		return gen.ListDeviceLogs404ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: notFoundProblem("device")}, nil
	} else if err != nil {
		return nil, err
	}

	p := req.Params
	limit := limitOr(p.Limit, 100)
	f := store.LogFilter{DeviceID: req.Id, Limit: limit + 1} // one extra row tells us if there's a next page
	if p.BeforeId != nil {
		f.BeforeID = *p.BeforeId
	}
	if p.AfterId != nil {
		f.AfterID = *p.AfterId
	}
	if p.Level != nil {
		f.Levels = *p.Level
	}
	if p.Q != nil {
		f.Search = strings.TrimSpace(*p.Q)
	}
	logs, err := s.store.DeviceLogs(ctx, f)
	if err != nil {
		return nil, err
	}

	page := gen.ListDeviceLogs200JSONResponse{Items: make([]gen.DeviceLog, 0, len(logs))}
	if len(logs) > limit {
		logs = logs[:limit]
		next := logs[len(logs)-1].ID
		page.NextBeforeId = &next
	}
	for _, l := range logs {
		page.Items = append(page.Items, toDeviceLog(l))
	}
	return page, nil
}

func (s *Server) ListDeviceTelemetry(ctx context.Context, req gen.ListDeviceTelemetryRequestObject) (gen.ListDeviceTelemetryResponseObject, error) {
	if _, err := s.store.Device(ctx, req.Id); errors.Is(err, store.ErrNotFound) {
		return gen.ListDeviceTelemetry404ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: notFoundProblem("device")}, nil
	} else if err != nil {
		return nil, err
	}
	var before int64
	if req.Params.BeforeId != nil {
		before = *req.Params.BeforeId
	}
	rows, err := s.store.TelemetryHistory(ctx, req.Id, before, limitOr(req.Params.Limit, 100))
	if err != nil {
		return nil, err
	}
	out := make(gen.ListDeviceTelemetry200JSONResponse, 0, len(rows))
	for _, t := range rows {
		out = append(out, toTelemetry(t))
	}
	return out, nil
}

func limitOr(l *gen.Limit, fallback int) int {
	if l == nil {
		return fallback
	}
	return int(*l)
}

func notFoundProblem(what string) gen.ProblemApplicationProblemPlusJSONResponse {
	detail := what + " not found"
	return gen.ProblemApplicationProblemPlusJSONResponse{
		Title:  http.StatusText(http.StatusNotFound),
		Status: http.StatusNotFound,
		Detail: &detail,
	}
}

func invalidProblem(err error) gen.ProblemApplicationProblemPlusJSONResponse {
	detail := strings.TrimPrefix(err.Error(), device.ErrInvalid.Error()+": ")
	return gen.ProblemApplicationProblemPlusJSONResponse{
		Title:  http.StatusText(http.StatusUnprocessableEntity),
		Status: http.StatusUnprocessableEntity,
		Detail: &detail,
	}
}
