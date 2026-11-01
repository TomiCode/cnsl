package store

import (
	"context"
	"database/sql"
	"fmt"
)

type Firmware struct {
	ID        int64  `db:"id"`
	Version   string `db:"version"`
	Chip      string `db:"chip"`
	Project   string `db:"project"`
	Built     string `db:"built"`
	IDF       string `db:"idf"`
	Size      int64  `db:"size"`
	SHA256    string `db:"sha256"`
	Filename  string `db:"filename"`
	Notes     string `db:"notes"`
	CreatedAt Time   `db:"created_at"`
}

func (s *Store) Firmwares(ctx context.Context) ([]Firmware, error) {
	var out []Firmware
	err := s.DB.SelectContext(ctx, &out, `SELECT * FROM firmware ORDER BY created_at DESC, id DESC`)
	return out, err
}

func (s *Store) Firmware(ctx context.Context, id int64) (Firmware, error) {
	var f Firmware
	err := s.DB.GetContext(ctx, &f, `SELECT * FROM firmware WHERE id = ?`, id)
	return f, notFound(err)
}

func (s *Store) FirmwareBySHA(ctx context.Context, sha string) (Firmware, error) {
	var f Firmware
	err := s.DB.GetContext(ctx, &f, `SELECT * FROM firmware WHERE sha256 = ?`, sha)
	return f, notFound(err)
}

func (s *Store) InsertFirmware(ctx context.Context, f *Firmware) error {
	f.CreatedAt = Now()
	res, err := s.DB.NamedExecContext(ctx, `
		INSERT INTO firmware (version, chip, project, built, idf, size, sha256, filename, notes, created_at)
		VALUES (:version, :chip, :project, :built, :idf, :size, :sha256, :filename, :notes, :created_at)`, f)
	if err != nil {
		return fmt.Errorf("insert firmware: %w", err)
	}
	f.ID, err = res.LastInsertId()
	return err
}

func (s *Store) UpdateFirmware(ctx context.Context, id int64, version, notes string) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE firmware SET version = ?, notes = ? WHERE id = ?`, version, notes, id)
	if err != nil {
		return err
	}
	return affected(res)
}

// DeleteFirmware removes a firmware and clears every reference to it.
func (s *Store) DeleteFirmware(ctx context.Context, id int64) error {
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE device_models SET firmware_id = NULL WHERE firmware_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE devices SET ota_firmware_id = NULL, ota_attempts = 0, ota_offered_at = NULL WHERE ota_firmware_id = ?`, id); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM firmware WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if err := affected(res); err != nil {
		return err
	}
	return tx.Commit()
}

// SetModelFirmware sets (or clears, with an invalid id) a model's target firmware.
func (s *Store) SetModelFirmware(ctx context.Context, modelID int64, firmwareID sql.NullInt64) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE device_models SET firmware_id = ? WHERE id = ?`, firmwareID, modelID)
	if err != nil {
		return err
	}
	return affected(res)
}

func (s *Store) SetAutoUpdate(ctx context.Context, deviceID int64, on bool) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE devices SET auto_update = ? WHERE id = ?`, on, deviceID)
	if err != nil {
		return err
	}
	return affected(res)
}

// RecordOTAOffer stores that a firmware was offered to a device (attempts counts offers of the
// same firmware).
func (s *Store) RecordOTAOffer(ctx context.Context, deviceID, firmwareID int64, attempts int, at Time) error {
	_, err := s.DB.ExecContext(ctx, `
		UPDATE devices SET ota_firmware_id = ?, ota_attempts = ?, ota_offered_at = ? WHERE id = ?`,
		firmwareID, attempts, at, deviceID)
	return err
}

// ResetOTA clears a device's offer state (after a successful update, or to retry).
func (s *Store) ResetOTA(ctx context.Context, deviceID int64) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE devices SET ota_firmware_id = NULL, ota_attempts = 0, ota_offered_at = NULL WHERE id = ?`, deviceID)
	return err
}
