package device

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/TomiCode/cnsl/internal/events"
	"github.com/TomiCode/cnsl/internal/store"
)

// ErrUnknownDevice is returned when a request's API key matches no device.
var ErrUnknownDevice = fmt.Errorf("%w: unknown device", ErrInvalid)

// IngestLogs stores log entries posted by the firmware to /api/log. Each entry is a JSON object;
// known fields get their own columns, everything else is kept in extra.
func (s *Service) IngestLogs(ctx context.Context, apiKey string, entries []map[string]any) (int, error) {
	if apiKey == "" {
		return 0, ErrUnknownDevice
	}
	d, err := s.store.DeviceByAPIKey(ctx, apiKey)
	if err != nil {
		if err == store.ErrNotFound {
			return 0, ErrUnknownDevice
		}
		return 0, err
	}

	now := store.Now()
	logs := make([]store.DeviceLog, 0, len(entries))
	for _, e := range entries {
		l := store.DeviceLog{DeviceID: d.ID, ReceivedAt: now}
		if v, ok := takeString(e, "message"); ok {
			l.Message = v
		}
		if v, ok := takeString(e, "level"); ok {
			l.Level = sql.NullString{String: v, Valid: true}
		}
		if v, ok := takeString(e, "source_path"); ok {
			l.SourcePath = sql.NullString{String: v, Valid: true}
		}
		if v, ok := takeInt(e, "source_line"); ok {
			l.SourceLine = sql.NullInt64{Int64: v, Valid: true}
		}
		if v, ok := takeInt(e, "id"); ok {
			l.DeviceLogID = sql.NullInt64{Int64: v, Valid: true}
		}
		// created_at is epoch seconds from the device clock; tiny values mean "clock not set".
		if v, ok := takeInt(e, "created_at"); ok && v > 1_000_000_000 {
			l.DeviceTime = store.NewNullTime(time.Unix(v, 0))
		}
		if len(e) > 0 {
			extra, err := json.Marshal(e)
			if err != nil {
				return 0, err
			}
			l.Extra = sql.NullString{String: string(extra), Valid: true}
		}
		logs = append(logs, l)
	}
	if err := s.store.InsertDeviceLogs(ctx, logs); err != nil {
		return 0, err
	}
	if len(logs) > 0 {
		s.events.Publish(events.Event{Type: events.DeviceLogs, DeviceID: d.ID})
	}
	return len(logs), nil
}

func takeString(m map[string]any, key string) (string, bool) {
	v, ok := m[key]
	if !ok {
		return "", false
	}
	delete(m, key)
	switch t := v.(type) {
	case string:
		return t, true
	case nil:
		return "", false
	default:
		return fmt.Sprint(t), true
	}
}

func takeInt(m map[string]any, key string) (int64, bool) {
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	delete(m, key)
	switch t := v.(type) {
	case float64:
		if math.IsNaN(t) || math.IsInf(t, 0) {
			return 0, false
		}
		return int64(t), true
	case json.Number:
		n, err := t.Int64()
		return n, err == nil
	default:
		return 0, false
	}
}
