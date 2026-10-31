package device

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/TomiCode/cnsl/internal/events"
	"github.com/TomiCode/cnsl/internal/store"
)

const (
	calibrationRefresh  = 60
	calibrationDuration = 30 * time.Minute
	waitingRefresh      = 30 // seconds, while a playlist screen has no render yet
)

// choose decides the image and refresh rate for an active device on this wake:
// calibration pattern while calibrating, else the device's playlist, else the placeholder.
func (s *Service) choose(ctx context.Context, d *store.Device, now time.Time, wakeReason string) (string, int, error) {
	if s.screens != nil && d.CalibratingUntil.Valid && now.Before(d.CalibratingUntil.Time.Time) && d.ModelID.Valid {
		name, err := s.screens.Calibration(ctx, d.ModelID.Int64)
		if err == nil {
			return name, calibrationRefresh, nil
		}
		s.log.WarnContext(ctx, "calibration pattern unavailable", "device", d.ID, "err", err)
	}

	if s.screens != nil && d.PlaylistID.Valid && d.ModelID.Valid {
		name, rate, ok, err := s.playNext(ctx, d, buttonWake(wakeReason))
		if err != nil {
			return "", 0, err
		}
		if ok {
			return name, rate, nil
		}
		if rate > 0 { // the screen is being rendered: show what we have, come back soon
			name, err := s.placeholder(ctx, d)
			return name, rate, err
		}
	}

	name, err := s.placeholder(ctx, d)
	return name, d.RefreshRate, err
}

// buttonWake reports whether the device was woken by its button (or touch) rather than its timer.
// The TRMNL X sends EXT0 for a button press (observed on firmware 1.8.6).
func buttonWake(reason string) bool {
	switch reason {
	case "button", "EXT0", "EXT1", "touchpad":
		return true
	}
	return false
}

// playNext advances the device's playlist position by one wake (rotation counts wakes, see
// docs/03), or straight to the next item when skip is set (button press), and returns that item's
// latest render. ok is false when nothing is playable yet.
func (s *Service) playNext(ctx context.Context, d *store.Device, skip bool) (string, int, bool, error) {
	items, err := s.store.PlayableItems(ctx, d.PlaylistID.Int64, d.ModelID.Int64)
	if err != nil || len(items) == 0 {
		return "", 0, false, err
	}
	pb, err := s.store.Playback(ctx, d.ID)
	if err != nil {
		return "", 0, false, err
	}

	idx := -1
	for i, it := range items {
		if pb.ItemID.Valid && it.ID == pb.ItemID.Int64 {
			idx = i
			break
		}
	}
	switch {
	case idx < 0: // first time, or the item was removed/disabled
		idx, pb.ShownCount = 0, 0
	case pb.ShownCount >= items[idx].Dwell, skip && pb.ShownCount > 0:
		idx, pb.ShownCount = (idx+1)%len(items), 0
	}
	item := items[idx]
	pb.ItemID = sql.NullInt64{Int64: item.ID, Valid: true}
	pb.ShownCount++
	if err := s.store.SavePlayback(ctx, pb); err != nil {
		return "", 0, false, err
	}

	rate := d.RefreshRate
	if item.RefreshRate.Valid {
		rate = int(item.RefreshRate.Int64)
	}

	// Make sure the item after this one is ready before the device wakes for it.
	if next := items[(idx+1)%len(items)]; next.ScreenID != item.ScreenID {
		if _, ok, err := s.screens.LatestImage(ctx, next.ScreenID); err == nil && !ok {
			s.screens.Request(next.ScreenID, "playlist")
		}
	}

	name, ok, err := s.screens.LatestImage(ctx, item.ScreenID)
	if err != nil {
		return "", 0, false, err
	}
	if !ok {
		s.screens.Request(item.ScreenID, "device")
		// Don't count this wake, and come back soon: the render is usually ready in seconds.
		pb.ShownCount--
		if err := s.store.SavePlayback(ctx, pb); err != nil {
			return "", 0, false, err
		}
		return "", waitingRefresh, false, nil
	}
	return name, rate, true, nil
}

// StartCalibration serves the calibration pattern to the device for the next 30 minutes.
func (s *Service) StartCalibration(ctx context.Context, id int64) (store.Device, error) {
	return s.update(ctx, id, func(d *store.Device) error {
		if !d.ModelID.Valid {
			return fmt.Errorf("%w: assign a device model first", ErrInvalid)
		}
		d.CalibratingUntil = store.NewNullTime(time.Now().Add(calibrationDuration))
		return nil
	})
}

func (s *Service) StopCalibration(ctx context.Context, id int64) (store.Device, error) {
	return s.update(ctx, id, func(d *store.Device) error {
		d.CalibratingUntil = store.NullTime{}
		return nil
	})
}

// AssignPlaylist sets (or clears, with nil) the device's playlist and restarts its rotation.
func (s *Service) AssignPlaylist(ctx context.Context, id int64, playlistID *int64) (store.Device, error) {
	if playlistID != nil {
		if _, err := s.store.Playlist(ctx, *playlistID); err != nil {
			if err == store.ErrNotFound {
				return store.Device{}, fmt.Errorf("%w: unknown playlist %d", ErrInvalid, *playlistID)
			}
			return store.Device{}, err
		}
	}
	d, err := s.update(ctx, id, func(d *store.Device) error {
		d.PlaylistID = sql.NullInt64{}
		if playlistID != nil {
			d.PlaylistID = sql.NullInt64{Int64: *playlistID, Valid: true}
		}
		return nil
	})
	if err != nil {
		return d, err
	}
	if err := s.store.SavePlayback(ctx, store.Playback{DeviceID: d.ID}); err != nil {
		return d, err
	}
	s.events.Publish(events.Event{Type: events.DeviceUpdated, DeviceID: d.ID})
	return d, nil
}
