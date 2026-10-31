package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	sq "github.com/Masterminds/squirrel"
)

// ErrNotFound is returned when a looked-up row does not exist.
var ErrNotFound = errors.New("not found")

type DeviceStatus string

const (
	DevicePending  DeviceStatus = "pending"
	DeviceActive   DeviceStatus = "active"
	DeviceDisabled DeviceStatus = "disabled"
)

type DeviceModel struct {
	ID            int64          `db:"id"`
	Key           string         `db:"key"`
	Name          string         `db:"name"`
	FirmwareModel sql.NullString `db:"firmware_model"`
	Width         int            `db:"width"`
	Height        int            `db:"height"`
	BitDepth      int            `db:"bit_depth"`
	Rotation      int            `db:"rotation"`
	ToneCurve     sql.NullString `db:"tone_curve"`
	Builtin       bool           `db:"builtin"`
	Scale         float64        `db:"scale"`
	ScreenClasses string         `db:"screen_classes"`
}

type Device struct {
	ID               int64          `db:"id"`
	MAC              string         `db:"mac"`
	APIKey           string         `db:"api_key"`
	FriendlyID       string         `db:"friendly_id"`
	Label            string         `db:"label"`
	Status           DeviceStatus   `db:"status"`
	ModelID          sql.NullInt64  `db:"model_id"`
	RefreshRate      int            `db:"refresh_rate"`
	SleepStart       sql.NullString `db:"sleep_start"`
	SleepEnd         sql.NullString `db:"sleep_end"`
	FirmwareVersion  sql.NullString `db:"firmware_version"`
	ReportedModel    sql.NullString `db:"reported_model"`
	ReportedWidth    sql.NullInt64  `db:"reported_width"`
	ReportedHeight   sql.NullInt64  `db:"reported_height"`
	LastSeenAt       NullTime       `db:"last_seen_at"`
	ApprovedAt       NullTime       `db:"approved_at"`
	CreatedAt        Time           `db:"created_at"`
	UpdatedAt        Time           `db:"updated_at"`
	PlaylistID       sql.NullInt64  `db:"playlist_id"`
	CalibratingUntil NullTime       `db:"calibrating_until"`
}

func (s *Store) DeviceModels(ctx context.Context) ([]DeviceModel, error) {
	var models []DeviceModel
	err := s.DB.SelectContext(ctx, &models, `SELECT * FROM device_models ORDER BY id`)
	return models, err
}

func (s *Store) DeviceModel(ctx context.Context, id int64) (DeviceModel, error) {
	var m DeviceModel
	err := s.DB.GetContext(ctx, &m, `SELECT * FROM device_models WHERE id = ?`, id)
	return m, notFound(err)
}

// DeviceModelByFirmware finds the model whose firmware_model matches the device's Model header.
func (s *Store) DeviceModelByFirmware(ctx context.Context, firmwareModel string) (DeviceModel, error) {
	var m DeviceModel
	err := s.DB.GetContext(ctx, &m, `SELECT * FROM device_models WHERE firmware_model = ? ORDER BY id LIMIT 1`, firmwareModel)
	return m, notFound(err)
}

func (s *Store) Devices(ctx context.Context) ([]Device, error) {
	var devices []Device
	err := s.DB.SelectContext(ctx, &devices, `SELECT * FROM devices ORDER BY status = 'pending' DESC, label, id`)
	return devices, err
}

func (s *Store) Device(ctx context.Context, id int64) (Device, error) {
	var d Device
	err := s.DB.GetContext(ctx, &d, `SELECT * FROM devices WHERE id = ?`, id)
	return d, notFound(err)
}

func (s *Store) DeviceByMAC(ctx context.Context, mac string) (Device, error) {
	var d Device
	err := s.DB.GetContext(ctx, &d, `SELECT * FROM devices WHERE mac = ?`, mac)
	return d, notFound(err)
}

func (s *Store) DeviceByAPIKey(ctx context.Context, key string) (Device, error) {
	var d Device
	err := s.DB.GetContext(ctx, &d, `SELECT * FROM devices WHERE api_key = ?`, key)
	return d, notFound(err)
}

func (s *Store) CreateDevice(ctx context.Context, d *Device) error {
	now := Now()
	d.CreatedAt, d.UpdatedAt = now, now
	res, err := s.DB.NamedExecContext(ctx, `
		INSERT INTO devices (mac, api_key, friendly_id, label, status, model_id, refresh_rate,
			firmware_version, reported_model, reported_width, reported_height, last_seen_at, created_at, updated_at)
		VALUES (:mac, :api_key, :friendly_id, :label, :status, :model_id, :refresh_rate,
			:firmware_version, :reported_model, :reported_width, :reported_height, :last_seen_at, :created_at, :updated_at)`, d)
	if err != nil {
		return fmt.Errorf("insert device: %w", err)
	}
	d.ID, err = res.LastInsertId()
	return err
}

// UpdateDevice writes every mutable column of d.
func (s *Store) UpdateDevice(ctx context.Context, d *Device) error {
	d.UpdatedAt = Now()
	res, err := s.DB.NamedExecContext(ctx, `
		UPDATE devices SET
			mac = :mac, label = :label, status = :status, model_id = :model_id, refresh_rate = :refresh_rate,
			sleep_start = :sleep_start, sleep_end = :sleep_end,
			firmware_version = :firmware_version, reported_model = :reported_model,
			reported_width = :reported_width, reported_height = :reported_height,
			last_seen_at = :last_seen_at, approved_at = :approved_at, updated_at = :updated_at,
			playlist_id = :playlist_id, calibrating_until = :calibrating_until
		WHERE id = :id`, d)
	if err != nil {
		return fmt.Errorf("update device: %w", err)
	}
	return affected(res)
}

func (s *Store) DeleteDevice(ctx context.Context, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM devices WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return affected(res)
}

// Telemetry is one /api/display request's worth of device state.
type Telemetry struct {
	ID              int64           `db:"id"`
	DeviceID        int64           `db:"device_id"`
	At              Time            `db:"at"`
	BatteryVoltage  sql.NullFloat64 `db:"battery_voltage"`
	BatteryPercent  sql.NullFloat64 `db:"battery_percent"`
	Charging        sql.NullBool    `db:"charging"`
	USBConnected    sql.NullBool    `db:"usb_connected"`
	RSSI            sql.NullInt64   `db:"rssi"`
	WakeReason      sql.NullString  `db:"wake_reason"`
	WakeTime        sql.NullInt64   `db:"wake_time"`
	FirmwareVersion sql.NullString  `db:"firmware_version"`
	ImageCached     sql.NullBool    `db:"image_cached"`
	ServedFilename  sql.NullString  `db:"served_filename"`
	RawHeaders      sql.NullString  `db:"raw_headers"`
}

func (s *Store) InsertTelemetry(ctx context.Context, t *Telemetry) error {
	res, err := s.DB.NamedExecContext(ctx, `
		INSERT INTO device_telemetry (device_id, at, battery_voltage, battery_percent, charging, usb_connected,
			rssi, wake_reason, wake_time, firmware_version, image_cached, served_filename, raw_headers)
		VALUES (:device_id, :at, :battery_voltage, :battery_percent, :charging, :usb_connected,
			:rssi, :wake_reason, :wake_time, :firmware_version, :image_cached, :served_filename, :raw_headers)`, t)
	if err != nil {
		return fmt.Errorf("insert telemetry: %w", err)
	}
	t.ID, err = res.LastInsertId()
	return err
}

// LatestTelemetry returns the newest telemetry row per device, keyed by device id.
func (s *Store) LatestTelemetry(ctx context.Context, deviceIDs ...int64) (map[int64]Telemetry, error) {
	q := sq.Select("t.*").From("device_telemetry t").
		Where("t.id = (SELECT MAX(id) FROM device_telemetry WHERE device_id = t.device_id)")
	if len(deviceIDs) > 0 {
		q = q.Where(sq.Eq{"t.device_id": deviceIDs})
	}
	query, args, err := q.ToSql()
	if err != nil {
		return nil, err
	}
	var rows []Telemetry
	if err := s.DB.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	out := make(map[int64]Telemetry, len(rows))
	for _, t := range rows {
		out[t.DeviceID] = t
	}
	return out, nil
}

// TelemetryHistory returns up to limit rows for a device, newest first, older than beforeID if set.
func (s *Store) TelemetryHistory(ctx context.Context, deviceID int64, beforeID int64, limit int) ([]Telemetry, error) {
	q := sq.Select("*").From("device_telemetry").Where(sq.Eq{"device_id": deviceID}).
		OrderBy("id DESC").Limit(uint64(limit))
	if beforeID > 0 {
		q = q.Where(sq.Lt{"id": beforeID})
	}
	query, args, err := q.ToSql()
	if err != nil {
		return nil, err
	}
	var rows []Telemetry
	err = s.DB.SelectContext(ctx, &rows, query, args...)
	return rows, err
}

func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func affected(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) UpdateToneCurve(ctx context.Context, modelID int64, curve sql.NullString) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE device_models SET tone_curve = ? WHERE id = ?`, curve, modelID)
	if err != nil {
		return err
	}
	return affected(res)
}
