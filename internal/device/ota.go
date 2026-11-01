package device

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/TomiCode/cnsl/internal/ota"
	"github.com/TomiCode/cnsl/internal/store"
)

// An offered update is retried at most this often and this many times. The firmware itself waits
// 24 h after a failed attempt; the limit also stops a loop when a firmware's version label
// doesn't match what the device reports after installing it.
const (
	otaRetryAfter  = 24 * time.Hour
	otaMaxAttempts = 3
)

// OTA states for the dashboard.
const (
	OTAOff      = "off"       // auto-update disabled for the device
	OTANoTarget = "no_target" // the device's model has no target firmware (or no chip)
	OTACurrent  = "current"   // the device reports the target version
	OTADue      = "due"       // will be offered at the next wake
	OTAPending  = "pending"   // offered; waiting for the device to report the new version
	OTAGaveUp   = "gave_up"   // offered otaMaxAttempts times without success
)

type OTAStatus struct {
	State     string
	Target    *store.Firmware
	Attempts  int
	OfferedAt store.NullTime
}

// target returns the firmware a device should run, if any.
func (s *Service) target(ctx context.Context, d *store.Device) (*store.Firmware, error) {
	if !d.ModelID.Valid {
		return nil, nil
	}
	m, err := s.store.DeviceModel(ctx, d.ModelID.Int64)
	if err != nil || !m.FirmwareID.Valid || m.Chip == "" {
		return nil, err
	}
	fw, err := s.store.Firmware(ctx, m.FirmwareID.Int64)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if fw.Chip != m.Chip { // checked when assigning; guards against edited data
		return nil, nil
	}
	return &fw, nil
}

// OTA reports a device's update status.
func (s *Service) OTA(ctx context.Context, d store.Device) (OTAStatus, error) {
	fw, err := s.target(ctx, &d)
	if err != nil {
		return OTAStatus{}, err
	}
	st := OTAStatus{Target: fw}
	switch {
	case fw == nil:
		st.State = OTANoTarget
	case ota.SameVersion(d.FirmwareVersion.String, fw.Version):
		st.State = OTACurrent
	case !d.AutoUpdate:
		st.State = OTAOff
	case d.OTAFirmwareID.Int64 != fw.ID || d.OTAAttempts == 0:
		st.State = OTADue
	default:
		st.Attempts, st.OfferedAt = d.OTAAttempts, d.OTAOfferedAt
		st.State = OTAPending
		if d.OTAAttempts >= otaMaxAttempts {
			st.State = OTAGaveUp
		}
	}
	if !d.AutoUpdate && st.State != OTACurrent && st.State != OTANoTarget {
		st.State = OTAOff
	}
	return st, nil
}

// offerFirmware decides whether this display response tells the device to update, and returns
// the firmware's file name if so.
func (s *Service) offerFirmware(ctx context.Context, d *store.Device, now time.Time) (string, error) {
	if !d.AutoUpdate {
		return "", nil
	}
	fw, err := s.target(ctx, d)
	if err != nil || fw == nil {
		return "", err
	}
	if ota.SameVersion(d.FirmwareVersion.String, fw.Version) {
		if d.OTAFirmwareID.Valid { // an offer worked
			s.serverLog(ctx, d.ID, now, "info", fmt.Sprintf("Firmware updated to %s", fw.Version))
			d.OTAFirmwareID, d.OTAAttempts, d.OTAOfferedAt = sql.NullInt64{}, 0, store.NullTime{}
			return "", s.store.ResetOTA(ctx, d.ID)
		}
		return "", nil
	}

	attempts := d.OTAAttempts
	if d.OTAFirmwareID.Int64 != fw.ID {
		attempts = 0
	}
	if attempts >= otaMaxAttempts {
		return "", nil
	}
	if attempts > 0 && d.OTAOfferedAt.Valid && now.Sub(d.OTAOfferedAt.Time.Time) < otaRetryAfter {
		return "", nil
	}
	attempts++
	at := store.Time{Time: now.UTC()}
	if err := s.store.RecordOTAOffer(ctx, d.ID, fw.ID, attempts, at); err != nil {
		return "", err
	}
	d.OTAFirmwareID = sql.NullInt64{Int64: fw.ID, Valid: true}
	d.OTAAttempts, d.OTAOfferedAt = attempts, store.NullTime{Time: at, Valid: true}
	s.serverLog(ctx, d.ID, now, "info", fmt.Sprintf("Offered firmware %s (from %s, attempt %d of %d)",
		fw.Version, d.FirmwareVersion.String, attempts, otaMaxAttempts))
	return ota.Files{}.Name(fw.SHA256), nil
}

// serverLog adds an entry from cnsl itself to the device's log.
func (s *Service) serverLog(ctx context.Context, deviceID int64, now time.Time, level, msg string) {
	s.log.InfoContext(ctx, msg, "device", deviceID)
	err := s.store.InsertDeviceLogs(ctx, []store.DeviceLog{{
		DeviceID: deviceID, Level: sql.NullString{String: level, Valid: true}, Message: msg,
		SourcePath: sql.NullString{String: "cnsl", Valid: true}, ReceivedAt: store.Time{Time: now.UTC()},
	}})
	if err != nil {
		s.log.WarnContext(ctx, "store server log", "device", deviceID, "err", err)
	}
}
