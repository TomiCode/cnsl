package store

import (
	"context"
	"database/sql"
	"fmt"

	sq "github.com/Masterminds/squirrel"
)

type DeviceLog struct {
	ID          int64          `db:"id"`
	DeviceID    int64          `db:"device_id"`
	DeviceLogID sql.NullInt64  `db:"device_log_id"`
	Level       sql.NullString `db:"level"`
	Message     string         `db:"message"`
	SourcePath  sql.NullString `db:"source_path"`
	SourceLine  sql.NullInt64  `db:"source_line"`
	DeviceTime  NullTime       `db:"device_time"`
	ReceivedAt  Time           `db:"received_at"`
	Extra       sql.NullString `db:"extra"` // JSON object
}

func (s *Store) InsertDeviceLogs(ctx context.Context, logs []DeviceLog) error {
	if len(logs) == 0 {
		return nil
	}
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareNamedContext(ctx, `
		INSERT INTO device_logs (device_id, device_log_id, level, message, source_path, source_line,
			device_time, received_at, extra)
		VALUES (:device_id, :device_log_id, :level, :message, :source_path, :source_line,
			:device_time, :received_at, :extra)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for i := range logs {
		if _, err := stmt.ExecContext(ctx, &logs[i]); err != nil {
			return fmt.Errorf("insert device log: %w", err)
		}
	}
	return tx.Commit()
}

type LogFilter struct {
	DeviceID int64
	BeforeID int64 // page backwards: only rows with id < BeforeID
	AfterID  int64 // live tail: only rows with id > AfterID
	Levels   []string
	Search   string // substring match on message
	Limit    int
}

// DeviceLogs returns logs matching f, newest first.
func (s *Store) DeviceLogs(ctx context.Context, f LogFilter) ([]DeviceLog, error) {
	q := sq.Select("*").From("device_logs").Where(sq.Eq{"device_id": f.DeviceID}).
		OrderBy("id DESC").Limit(uint64(f.Limit))
	if f.BeforeID > 0 {
		q = q.Where(sq.Lt{"id": f.BeforeID})
	}
	if f.AfterID > 0 {
		q = q.Where(sq.Gt{"id": f.AfterID})
	}
	if len(f.Levels) > 0 {
		q = q.Where(sq.Eq{"level": f.Levels})
	}
	if f.Search != "" {
		q = q.Where("message LIKE ? ESCAPE '\\'", "%"+escapeLike(f.Search)+"%")
	}
	query, args, err := q.ToSql()
	if err != nil {
		return nil, err
	}
	var rows []DeviceLog
	err = s.DB.SelectContext(ctx, &rows, query, args...)
	return rows, err
}

func escapeLike(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r == '%' || r == '_' || r == '\\' {
			out = append(out, '\\')
		}
		out = append(out, r)
	}
	return string(out)
}
