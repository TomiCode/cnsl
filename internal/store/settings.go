package store

import (
	"context"
	"database/sql"
	"errors"
)

// Setting keys.
const SettingLanguage = "language"

// Setting returns a global setting, or fallback when it is not set.
func (s *Store) Setting(ctx context.Context, key, fallback string) (string, error) {
	var v string
	err := s.DB.GetContext(ctx, &v, `SELECT value FROM settings WHERE key = ?`, key)
	if errors.Is(err, sql.ErrNoRows) {
		return fallback, nil
	}
	return v, err
}

func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, value, Now())
	return err
}
