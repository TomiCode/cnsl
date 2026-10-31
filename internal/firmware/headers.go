package firmware

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/TomiCode/cnsl/internal/device"
	"github.com/TomiCode/cnsl/internal/store"
)

func identity(r *http.Request) device.Identity {
	width, _ := strconv.Atoi(header(r, "Width"))
	height, _ := strconv.Atoi(header(r, "Height"))
	return device.Identity{
		MAC:             header(r, "ID"),
		Model:           header(r, "Model"),
		FirmwareVersion: header(r, "FW-Version"),
		Width:           width,
		Height:          height,
	}
}

// telemetry maps the firmware's status headers (lib/trmnl/src/api-client/request_headers.cpp).
// Everything except the API key is also kept verbatim in raw_headers.
func telemetry(r *http.Request) store.Telemetry {
	t := store.Telemetry{
		BatteryVoltage:  float(r, "Battery-Voltage"),
		BatteryPercent:  float(r, "Percent-Charged"),
		Charging:        boolean(r, "Battery-Charging"),
		USBConnected:    boolean(r, "USB-Connected"),
		RSSI:            integer(r, "RSSI"),
		WakeReason:      str(r, "Update-Source"),
		WakeTime:        integer(r, "Wake-Time"),
		FirmwareVersion: str(r, "FW-Version"),
		ImageCached:     boolean(r, "Image-Cached"),
	}
	// The firmware reports -1 when the fuel gauge has no reading.
	if t.BatteryPercent.Valid && t.BatteryPercent.Float64 < 0 {
		t.BatteryPercent.Valid = false
	}

	raw := make(map[string]string, len(r.Header))
	for name, values := range r.Header {
		switch strings.ToLower(strings.ReplaceAll(name, "_", "-")) {
		case "access-token", "accept", "accept-encoding", "content-type", "connection", "user-agent", "content-length":
			continue
		}
		raw[name] = strings.Join(values, ", ")
	}
	if b, err := json.Marshal(raw); err == nil {
		t.RawHeaders = sql.NullString{String: string(b), Valid: true}
	}
	return t
}

func str(r *http.Request, name string) sql.NullString {
	v := header(r, name)
	return sql.NullString{String: v, Valid: v != ""}
}

func float(r *http.Request, name string) sql.NullFloat64 {
	v, err := strconv.ParseFloat(header(r, name), 64)
	return sql.NullFloat64{Float64: v, Valid: err == nil}
}

func integer(r *http.Request, name string) sql.NullInt64 {
	v, err := strconv.ParseInt(header(r, name), 10, 64)
	return sql.NullInt64{Int64: v, Valid: err == nil}
}

func boolean(r *http.Request, name string) sql.NullBool {
	switch strings.ToLower(header(r, name)) {
	case "1", "true":
		return sql.NullBool{Bool: true, Valid: true}
	case "0", "false":
		return sql.NullBool{Bool: false, Valid: true}
	default:
		return sql.NullBool{}
	}
}
