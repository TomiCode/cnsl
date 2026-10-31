// Package device implements device lifecycle: provisioning, approval, telemetry, logs and the
// image a device is served.
package device

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/TomiCode/cnsl/internal/events"
	"github.com/TomiCode/cnsl/internal/imaging"
	"github.com/TomiCode/cnsl/internal/store"
)

// ErrInvalid marks a request the caller got wrong (bad input, wrong state).
var ErrInvalid = errors.New("invalid request")

const defaultRefreshRate = 900

type Service struct {
	store   *store.Store
	events  *events.Broker
	files   imaging.Files
	tz      *time.Location
	log     *slog.Logger
	screens Screens
}

// Screens is what the device service needs from rendering.
type Screens interface {
	// LatestImage is the filename of the screen's newest successful render.
	LatestImage(ctx context.Context, screenID int64) (filename string, ok bool, err error)
	// Request asks for a render soon (e.g. a device is waiting for a screen without one).
	Request(screenID int64, trigger string)
	// Calibration is the gray calibration pattern for a model with its current tone curve.
	Calibration(ctx context.Context, modelID int64) (string, error)
}

// SetScreens connects the renderer; until then devices only get static images.
func (s *Service) SetScreens(sc Screens) {
	s.screens = sc
}

func NewService(st *store.Store, ev *events.Broker, files imaging.Files, tz *time.Location, log *slog.Logger) *Service {
	return &Service{store: st, events: ev, files: files, tz: tz, log: log}
}

// Identity is what a device reports about itself on every request.
type Identity struct {
	MAC             string
	Model           string
	FirmwareVersion string
	Width, Height   int
}

// Setup handles /api/setup: unknown devices are created as pending; known ones get their key back.
func (s *Service) Setup(ctx context.Context, id Identity) (store.Device, error) {
	mac := NormalizeMAC(id.MAC)
	if mac == "" {
		return store.Device{}, fmt.Errorf("%w: missing MAC address", ErrInvalid)
	}
	d, err := s.store.DeviceByMAC(ctx, mac)
	if err == nil {
		s.applyIdentity(&d, id)
		return d, s.store.UpdateDevice(ctx, &d)
	}
	if !errors.Is(err, store.ErrNotFound) {
		return store.Device{}, err
	}
	return s.createPending(ctx, mac, rand.Text(), id)
}

func (s *Service) createPending(ctx context.Context, mac, apiKey string, id Identity) (store.Device, error) {
	d := store.Device{
		MAC:         mac,
		APIKey:      apiKey,
		Status:      store.DevicePending,
		RefreshRate: defaultRefreshRate,
		LastSeenAt:  store.NewNullTime(time.Now()),
	}
	s.applyIdentity(&d, id)
	if m, err := s.store.DeviceModelByFirmware(ctx, id.Model); err == nil {
		d.ModelID = sql.NullInt64{Int64: m.ID, Valid: true}
	}

	// friendly_id is unique; retry on the (unlikely) collision.
	var err error
	for range 5 {
		d.FriendlyID = friendlyID()
		if err = s.store.CreateDevice(ctx, &d); err == nil {
			break
		}
	}
	if err != nil {
		return store.Device{}, err
	}
	s.log.InfoContext(ctx, "new pending device", "mac", mac, "friendly_id", d.FriendlyID, "model", id.Model)
	s.events.Publish(events.Event{Type: events.DeviceCreated, DeviceID: d.ID})
	return d, nil
}

func (s *Service) applyIdentity(d *store.Device, id Identity) {
	if id.FirmwareVersion != "" {
		d.FirmwareVersion = sql.NullString{String: id.FirmwareVersion, Valid: true}
	}
	if id.Model != "" {
		d.ReportedModel = sql.NullString{String: id.Model, Valid: true}
	}
	if id.Width > 0 && id.Height > 0 {
		d.ReportedWidth = sql.NullInt64{Int64: int64(id.Width), Valid: true}
		d.ReportedHeight = sql.NullInt64{Int64: int64(id.Height), Valid: true}
	}
}

// DisplayRequest is one /api/display call.
type DisplayRequest struct {
	APIKey    string
	Identity  Identity
	Telemetry store.Telemetry // DeviceID and At are filled in by Display
}

type DisplayOutcome int

const (
	// NotRegistered: unknown, pending or disabled device. The firmware fast-polls.
	NotRegistered DisplayOutcome = iota
	ShowImage
)

type DisplayResult struct {
	Outcome     DisplayOutcome
	Filename    string
	RefreshRate int
}

// Display handles /api/display: records telemetry and decides what the device should show.
func (s *Service) Display(ctx context.Context, req DisplayRequest) (DisplayResult, error) {
	d, err := s.deviceForKey(ctx, req.APIKey, req.Identity)
	if err != nil || d == nil {
		return DisplayResult{Outcome: NotRegistered}, err
	}

	now := time.Now()
	s.applyIdentity(d, req.Identity)
	d.LastSeenAt = store.NewNullTime(now)

	result := DisplayResult{Outcome: NotRegistered}
	if d.Status == store.DeviceActive {
		filename, rate, err := s.choose(ctx, d, now, req.Telemetry.WakeReason.String)
		if err != nil {
			return DisplayResult{}, err
		}
		result = DisplayResult{
			Outcome:     ShowImage,
			Filename:    filename,
			RefreshRate: refreshRate(now.In(s.tz), rate, d.SleepStart.String, d.SleepEnd.String),
		}
	}

	t := req.Telemetry
	t.DeviceID = d.ID
	t.At = store.Time{Time: now.UTC()}
	if result.Filename != "" {
		t.ServedFilename = sql.NullString{String: result.Filename, Valid: true}
	}
	if err := s.store.InsertTelemetry(ctx, &t); err != nil {
		return DisplayResult{}, err
	}
	if err := s.store.UpdateDevice(ctx, d); err != nil {
		return DisplayResult{}, err
	}
	s.events.Publish(events.Event{Type: events.DeviceSeen, DeviceID: d.ID})
	return result, nil
}

// deviceForKey finds the device for an API key. A device that still has a key but isn't known
// (e.g. deleted in the dashboard) is re-created as pending with that key, so it shows up for
// approval again without being re-flashed. Returns nil when the request can't be attributed.
func (s *Service) deviceForKey(ctx context.Context, apiKey string, id Identity) (*store.Device, error) {
	if apiKey == "" {
		return nil, nil
	}
	d, err := s.store.DeviceByAPIKey(ctx, apiKey)
	if err == nil {
		return &d, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}

	mac := NormalizeMAC(id.MAC)
	if mac == "" {
		return nil, nil
	}
	if _, err := s.store.DeviceByMAC(ctx, mac); err == nil {
		s.log.WarnContext(ctx, "api key does not match the device's MAC", "mac", mac)
		return nil, nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	created, err := s.createPending(ctx, mac, apiKey, id)
	if err != nil {
		return nil, err
	}
	return &created, nil
}

// placeholder returns the "nothing to show" image for the device's model.
func (s *Service) placeholder(ctx context.Context, d *store.Device) (string, error) {
	m, err := s.modelFor(ctx, d)
	if err != nil {
		return "", err
	}
	return s.files.Save(imaging.Placeholder(m.Width, m.Height, m.BitDepth), m.BitDepth)
}

func (s *Service) modelFor(ctx context.Context, d *store.Device) (store.DeviceModel, error) {
	if !d.ModelID.Valid {
		return store.DeviceModel{}, fmt.Errorf("%w: device has no model assigned", ErrInvalid)
	}
	return s.store.DeviceModel(ctx, d.ModelID.Int64)
}

// Approve activates a pending or disabled device.
func (s *Service) Approve(ctx context.Context, id int64) (store.Device, error) {
	return s.update(ctx, id, func(d *store.Device) error {
		if !d.ModelID.Valid {
			return fmt.Errorf("%w: assign a device model before approving", ErrInvalid)
		}
		d.Status = store.DeviceActive
		if !d.ApprovedAt.Valid {
			d.ApprovedAt = store.NewNullTime(time.Now())
		}
		return nil
	})
}

func (s *Service) Disable(ctx context.Context, id int64) (store.Device, error) {
	return s.update(ctx, id, func(d *store.Device) error {
		d.Status = store.DeviceDisabled
		return nil
	})
}

// Settings are the user-editable device fields; nil means unchanged.
type Settings struct {
	Label       *string
	ModelID     *int64
	RefreshRate *int
	SleepStart  *string // "" clears
	SleepEnd    *string // "" clears
}

func (s *Service) UpdateSettings(ctx context.Context, id int64, in Settings) (store.Device, error) {
	if in.ModelID != nil {
		if _, err := s.store.DeviceModel(ctx, *in.ModelID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return store.Device{}, fmt.Errorf("%w: unknown device model %d", ErrInvalid, *in.ModelID)
			}
			return store.Device{}, err
		}
	}
	return s.update(ctx, id, func(d *store.Device) error {
		if in.Label != nil {
			d.Label = strings.TrimSpace(*in.Label)
		}
		if in.ModelID != nil {
			d.ModelID = sql.NullInt64{Int64: *in.ModelID, Valid: true}
		}
		if in.RefreshRate != nil {
			if *in.RefreshRate < 30 || *in.RefreshRate > 86400 {
				return fmt.Errorf("%w: refresh_rate must be between 30 and 86400 seconds", ErrInvalid)
			}
			d.RefreshRate = *in.RefreshRate
		}
		for _, f := range []struct {
			in  *string
			out *sql.NullString
		}{{in.SleepStart, &d.SleepStart}, {in.SleepEnd, &d.SleepEnd}} {
			if f.in == nil {
				continue
			}
			if *f.in == "" {
				*f.out = sql.NullString{}
				continue
			}
			if _, ok := parseClock(*f.in); !ok {
				return fmt.Errorf("%w: sleep times must be HH:MM, got %q", ErrInvalid, *f.in)
			}
			*f.out = sql.NullString{String: *f.in, Valid: true}
		}
		if d.SleepStart.Valid != d.SleepEnd.Valid {
			return fmt.Errorf("%w: set both sleep_start and sleep_end, or neither", ErrInvalid)
		}
		return nil
	})
}

func (s *Service) update(ctx context.Context, id int64, change func(*store.Device) error) (store.Device, error) {
	d, err := s.store.Device(ctx, id)
	if err != nil {
		return d, err
	}
	if err := change(&d); err != nil {
		return d, err
	}
	if err := s.store.UpdateDevice(ctx, &d); err != nil {
		return d, err
	}
	s.events.Publish(events.Event{Type: events.DeviceUpdated, DeviceID: d.ID})
	return d, nil
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	if err := s.store.DeleteDevice(ctx, id); err != nil {
		return err
	}
	s.events.Publish(events.Event{Type: events.DeviceDeleted, DeviceID: id})
	return nil
}

// NormalizeMAC upper-cases a MAC address and trims whitespace.
func NormalizeMAC(mac string) string {
	return strings.ToUpper(strings.TrimSpace(mac))
}

// friendlyID is a short code the user can match against the device's setup screen.
func friendlyID() string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no 0/O, 1/I
	b := make([]byte, 6)
	rand.Read(b)
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(b)
}
