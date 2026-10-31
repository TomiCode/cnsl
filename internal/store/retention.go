package store

import (
	"context"
	"log/slog"
	"time"
)

// RunRetention deletes device logs and telemetry older than the given ages, once at start and
// then hourly, until ctx ends.
func (s *Store) RunRetention(ctx context.Context, logsAge, telemetryAge time.Duration, log *slog.Logger) {
	prune := func() {
		now := time.Now()
		logs, err := s.pruneBefore(ctx, "device_logs", "received_at", now.Add(-logsAge))
		if err != nil {
			log.Warn("retention: device logs", "err", err)
		}
		tel, err := s.pruneBefore(ctx, "device_telemetry", "at", now.Add(-telemetryAge))
		if err != nil {
			log.Warn("retention: telemetry", "err", err)
		}
		if logs+tel > 0 {
			log.Info("retention: pruned old rows", "device_logs", logs, "telemetry", tel)
		}
	}
	prune()
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			prune()
		}
	}
}

// pruneBefore deletes rows whose timestamp column is older than cutoff. Timestamps are stored
// as fixed-format UTC text (see Time), so text comparison orders them correctly.
func (s *Store) pruneBefore(ctx context.Context, table, column string, cutoff time.Time) (int64, error) {
	c, err := Time{cutoff.UTC()}.Value()
	if err != nil {
		return 0, err
	}
	res, err := s.DB.ExecContext(ctx, `DELETE FROM `+table+` WHERE `+column+` < ?`, c)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
