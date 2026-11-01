package dashboard

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"strings"

	"github.com/TomiCode/cnsl/internal/dashboard/gen"
	"github.com/TomiCode/cnsl/internal/device"
	"github.com/TomiCode/cnsl/internal/events"
	"github.com/TomiCode/cnsl/internal/ota"
	"github.com/TomiCode/cnsl/internal/store"
)

func (s *Server) firmwareFiles() ota.Files { return ota.Files{Dir: s.firmwareDir} }

func (s *Server) toFirmware(ctx context.Context, f store.Firmware) (gen.Firmware, error) {
	models, err := s.store.DeviceModels(ctx)
	if err != nil {
		return gen.Firmware{}, err
	}
	ids := []int64{}
	for _, m := range models {
		if m.FirmwareID.Valid && m.FirmwareID.Int64 == f.ID {
			ids = append(ids, m.ID)
		}
	}
	return gen.Firmware{Id: f.ID, Version: f.Version, Chip: f.Chip, Project: f.Project, Built: f.Built, Idf: f.IDF,
		Size: f.Size, Sha256: f.SHA256, Filename: f.Filename, Notes: f.Notes, CreatedAt: f.CreatedAt.Time, ModelIds: ids}, nil
}

func (s *Server) ListFirmware(ctx context.Context, _ gen.ListFirmwareRequestObject) (gen.ListFirmwareResponseObject, error) {
	rows, err := s.store.Firmwares(ctx)
	if err != nil {
		return nil, err
	}
	out := make(gen.ListFirmware200JSONResponse, 0, len(rows))
	for _, f := range rows {
		gf, err := s.toFirmware(ctx, f)
		if err != nil {
			return nil, err
		}
		out = append(out, gf)
	}
	return out, nil
}

func (s *Server) UploadFirmware(ctx context.Context, req gen.UploadFirmwareRequestObject) (gen.UploadFirmwareResponseObject, error) {
	invalid := func(format string, args ...any) gen.UploadFirmwareResponseObject {
		return gen.UploadFirmware422ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: invalidProblem(fmt.Errorf(format, args...))}
	}
	var data []byte
	var filename, version, notes string
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
			filename = path.Base(filepath.ToSlash(part.FileName()))
			data, err = io.ReadAll(io.LimitReader(part, ota.MaxUpload+1))
			if err != nil {
				return invalid("read upload: %v", err), nil
			}
			if len(data) > ota.MaxUpload {
				return invalid("%s is larger than %d MiB", filename, ota.MaxUpload>>20), nil
			}
		case "version":
			v, _ := io.ReadAll(io.LimitReader(part, 100))
			version = strings.TrimSpace(string(v))
		case "notes":
			v, _ := io.ReadAll(io.LimitReader(part, 1000))
			notes = strings.TrimSpace(string(v))
		}
		part.Close()
	}
	if data == nil {
		return invalid("missing file"), nil
	}
	// Full-flash images (as downloaded for web flashers) carry the app in a partition; only that
	// part is stored and sent to devices.
	app, full, err := ota.App(data)
	if err != nil {
		return invalid("%s: %v", filename, err), nil
	}
	if full {
		data = app
		note := "App taken from a full-flash image."
		notes = strings.TrimSpace(note + " " + notes)
	}
	info, err := ota.Parse(data)
	if err != nil {
		return invalid("%s: %v", filename, err), nil
	}
	if existing, err := s.store.FirmwareBySHA(ctx, info.SHA256); err == nil {
		gf, err := s.toFirmware(ctx, existing) // the same file uploaded before
		return gen.UploadFirmware201JSONResponse{FirmwareJSONResponse: gen.FirmwareJSONResponse(gf)}, err
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	if version == "" {
		version = ota.GuessVersion(info, filename)
	}
	if version == "" {
		return invalid("enter the version the device reports after installing it (e.g. 1.8.19): it isn't in the file name or the image"), nil
	}
	if err := s.firmwareFiles().Save(info, data); err != nil {
		return nil, err
	}
	f := store.Firmware{Version: version, Chip: info.Chip, Project: info.Project, Built: info.Built, IDF: info.IDF,
		Size: int64(info.Size), SHA256: info.SHA256, Filename: filename, Notes: notes}
	if err := s.store.InsertFirmware(ctx, &f); err != nil {
		return nil, err
	}
	gf, err := s.toFirmware(ctx, f)
	return gen.UploadFirmware201JSONResponse{FirmwareJSONResponse: gen.FirmwareJSONResponse(gf)}, err
}

func (s *Server) UpdateFirmware(ctx context.Context, req gen.UpdateFirmwareRequestObject) (gen.UpdateFirmwareResponseObject, error) {
	notes := ""
	if req.Body.Notes != nil {
		notes = strings.TrimSpace(*req.Body.Notes)
	}
	err := s.store.UpdateFirmware(ctx, req.Id, strings.TrimSpace(req.Body.Version), notes)
	if errors.Is(err, store.ErrNotFound) {
		return gen.UpdateFirmware404ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: notFoundProblem("firmware")}, nil
	}
	if err != nil {
		return nil, err
	}
	f, err := s.store.Firmware(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	gf, err := s.toFirmware(ctx, f)
	return gen.UpdateFirmware200JSONResponse{FirmwareJSONResponse: gen.FirmwareJSONResponse(gf)}, err
}

func (s *Server) DeleteFirmware(ctx context.Context, req gen.DeleteFirmwareRequestObject) (gen.DeleteFirmwareResponseObject, error) {
	f, err := s.store.Firmware(ctx, req.Id)
	if errors.Is(err, store.ErrNotFound) {
		return gen.DeleteFirmware404ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: notFoundProblem("firmware")}, nil
	}
	if err != nil {
		return nil, err
	}
	if err := s.store.DeleteFirmware(ctx, f.ID); err != nil {
		return nil, err
	}
	if err := s.firmwareFiles().Remove(f.SHA256); err != nil {
		return nil, err
	}
	s.events.Publish(events.Event{Type: events.ModelUpdated})
	return gen.DeleteFirmware204Response{}, nil
}

func (s *Server) SetModelFirmware(ctx context.Context, req gen.SetModelFirmwareRequestObject) (gen.SetModelFirmwareResponseObject, error) {
	invalid := func(format string, args ...any) gen.SetModelFirmwareResponseObject {
		return gen.SetModelFirmware422ApplicationProblemPlusJSONResponse(invalidProblem(fmt.Errorf(format, args...)))
	}
	m, err := s.store.DeviceModel(ctx, req.Id)
	if errors.Is(err, store.ErrNotFound) {
		return gen.SetModelFirmware404ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: notFoundProblem("device model")}, nil
	}
	if err != nil {
		return nil, err
	}
	var target sql.NullInt64
	if id := req.Body.FirmwareId; id != nil {
		if m.Chip == "" {
			return invalid("%s doesn't take firmware updates", m.Name), nil
		}
		f, err := s.store.Firmware(ctx, *id)
		if errors.Is(err, store.ErrNotFound) {
			return invalid("unknown firmware %d", *id), nil
		}
		if err != nil {
			return nil, err
		}
		if f.Chip != m.Chip {
			return invalid("firmware %s is built for %s, but %s needs %s", f.Version, ota.ChipName(f.Chip), m.Name, ota.ChipName(m.Chip)), nil
		}
		target = sql.NullInt64{Int64: f.ID, Valid: true}
	}
	if err := s.store.SetModelFirmware(ctx, m.ID, target); err != nil {
		return nil, err
	}
	m.FirmwareID = target
	s.events.Publish(events.Event{Type: events.ModelUpdated, ModelID: m.ID})
	return gen.SetModelFirmware200JSONResponse(toDeviceModel(m)), nil
}

func (s *Server) RetryDeviceOTA(ctx context.Context, req gen.RetryDeviceOTARequestObject) (gen.RetryDeviceOTAResponseObject, error) {
	d, err := s.store.Device(ctx, req.Id)
	if errors.Is(err, store.ErrNotFound) {
		return gen.RetryDeviceOTA404ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: notFoundProblem("device")}, nil
	}
	if err != nil {
		return nil, err
	}
	if err := s.store.ResetOTA(ctx, d.ID); err != nil {
		return nil, err
	}
	if d, err = s.store.Device(ctx, d.ID); err != nil {
		return nil, err
	}
	s.events.Publish(events.Event{Type: events.DeviceUpdated, DeviceID: d.ID})
	resp, err := s.deviceResponse(ctx, d)
	return gen.RetryDeviceOTA200JSONResponse{DeviceJSONResponse: resp}, err
}

// toOTA converts a device's update status for the API.
func toOTA(st device.OTAStatus) *gen.OTAStatus {
	out := &gen.OTAStatus{State: gen.OTAStatusState(st.State)}
	if st.Target != nil {
		out.TargetId, out.TargetVersion = &st.Target.ID, &st.Target.Version
	}
	if st.Attempts > 0 {
		out.Attempts = &st.Attempts
	}
	out.OfferedAt = st.OfferedAt.Ptr()
	return out
}
