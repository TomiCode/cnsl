package dashboard

import (
	"database/sql"
	"encoding/json"

	"github.com/TomiCode/cnsl/internal/dashboard/gen"
	"github.com/TomiCode/cnsl/internal/store"
)

func toDevice(d store.Device, latest *store.Telemetry) gen.Device {
	out := gen.Device{
		Id:               d.ID,
		Mac:              d.MAC,
		ApiKey:           d.APIKey,
		FriendlyId:       d.FriendlyID,
		Label:            d.Label,
		Status:           gen.DeviceStatus(d.Status),
		ModelId:          nullInt64(d.ModelID),
		RefreshRate:      d.RefreshRate,
		SleepStart:       nullString(d.SleepStart),
		SleepEnd:         nullString(d.SleepEnd),
		PlaylistId:       nullInt64(d.PlaylistID),
		CalibratingUntil: d.CalibratingUntil.Ptr(),
		FirmwareVersion:  nullString(d.FirmwareVersion),
		ReportedModel:    nullString(d.ReportedModel),
		ReportedWidth:    nullInt(d.ReportedWidth),
		ReportedHeight:   nullInt(d.ReportedHeight),
		LastSeenAt:       d.LastSeenAt.Ptr(),
		ApprovedAt:       d.ApprovedAt.Ptr(),
		CreatedAt:        d.CreatedAt.Time,
		UpdatedAt:        d.UpdatedAt.Time,
		AutoUpdate:       &d.AutoUpdate,
	}
	if latest != nil {
		t := toTelemetry(*latest)
		out.Latest = &t
	}
	return out
}

func toDeviceModel(m store.DeviceModel) gen.DeviceModel {
	return gen.DeviceModel{
		Id:            m.ID,
		Key:           m.Key,
		Name:          m.Name,
		FirmwareModel: nullString(m.FirmwareModel),
		Width:         m.Width,
		Height:        m.Height,
		BitDepth:      gen.DeviceModelBitDepth(m.BitDepth),
		ToneCurve:     toToneCurve(m.ToneCurve),
		Chip:          &m.Chip,
		FirmwareId:    nullInt64(m.FirmwareID),
	}
}

func toTelemetry(t store.Telemetry) gen.Telemetry {
	out := gen.Telemetry{
		Id:              t.ID,
		At:              t.At.Time,
		BatteryVoltage:  nullFloat(t.BatteryVoltage),
		BatteryPercent:  nullFloat(t.BatteryPercent),
		Charging:        nullBool(t.Charging),
		UsbConnected:    nullBool(t.USBConnected),
		Rssi:            nullInt(t.RSSI),
		WakeReason:      nullString(t.WakeReason),
		WakeTime:        nullInt(t.WakeTime),
		FirmwareVersion: nullString(t.FirmwareVersion),
		ImageCached:     nullBool(t.ImageCached),
		ServedFilename:  nullString(t.ServedFilename),
	}
	if t.RawHeaders.Valid {
		var raw map[string]string
		if json.Unmarshal([]byte(t.RawHeaders.String), &raw) == nil {
			out.RawHeaders = &raw
		}
	}
	return out
}

func toDeviceLog(l store.DeviceLog) gen.DeviceLog {
	out := gen.DeviceLog{
		Id:          l.ID,
		DeviceLogId: nullInt64(l.DeviceLogID),
		Level:       nullString(l.Level),
		Message:     l.Message,
		SourcePath:  nullString(l.SourcePath),
		SourceLine:  nullInt(l.SourceLine),
		DeviceTime:  l.DeviceTime.Ptr(),
		ReceivedAt:  l.ReceivedAt.Time,
	}
	if l.Extra.Valid {
		var extra map[string]any
		if json.Unmarshal([]byte(l.Extra.String), &extra) == nil {
			out.Extra = &extra
		}
	}
	return out
}

func nullString(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	return &v.String
}

func nullInt64(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	return &v.Int64
}

func nullInt(v sql.NullInt64) *int {
	if !v.Valid {
		return nil
	}
	n := int(v.Int64)
	return &n
}

func nullFloat(v sql.NullFloat64) *float32 {
	if !v.Valid {
		return nil
	}
	f := float32(v.Float64)
	return &f
}

func nullBool(v sql.NullBool) *bool {
	if !v.Valid {
		return nil
	}
	return &v.Bool
}
