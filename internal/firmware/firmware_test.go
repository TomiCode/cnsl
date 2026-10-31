package firmware

import (
	"context"
	"encoding/json"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TomiCode/cnsl/internal/device"
	"github.com/TomiCode/cnsl/internal/events"
	"github.com/TomiCode/cnsl/internal/imaging"
	"github.com/TomiCode/cnsl/internal/store"
)

// TestDeviceLifecycle plays a TRMNL X through setup → pending → approval → image → logs,
// using the header names the current firmware sends.
func TestDeviceLifecycle(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(ctx, filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	devices := device.NewService(st, events.NewBroker(), imaging.Files{Dir: dir}, time.UTC, log)
	srv := httptest.NewServer(Handler(Options{Devices: devices, ImagesDir: dir, PublicURL: "http://cnsl.test", Log: log}))
	defer srv.Close()

	call := func(method, path string, headers map[string]string, body string) map[string]any {
		t.Helper()
		req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusNoContent {
			return nil
		}
		var out map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatalf("%s %s: decode: %v", method, path, err)
		}
		return out
	}

	const mac = "aa:bb:cc:dd:ee:ff"
	setup := call("GET", "/api/setup", map[string]string{"ID": mac, "Model": "x", "Width": "1872", "Height": "1404", "FW-Version": "1.7.0"}, "")
	if setup["status"] != float64(200) || setup["api_key"] == "" || setup["friendly_id"] == "" {
		t.Fatalf("setup = %v", setup)
	}
	key := setup["api_key"].(string)

	again := call("GET", "/api/setup", map[string]string{"ID": mac}, "")
	if again["api_key"] != key {
		t.Fatalf("second setup returned a different key: %v", again)
	}

	displayHeaders := map[string]string{
		"ID": mac, "Access-Token": key, "Model": "x", "Battery-Voltage": "4.01", "Percent-Charged": "87",
		"RSSI": "-61", "Update-Source": "timer", "Width": "1872", "Height": "1404",
	}
	if got := call("GET", "/api/display", displayHeaders, ""); got["status"] != float64(202) {
		t.Fatalf("pending display = %v, want status 202", got)
	}

	d, err := st.DeviceByMAC(ctx, "AA:BB:CC:DD:EE:FF")
	if err != nil {
		t.Fatal(err)
	}
	if d.Status != store.DevicePending || !d.ModelID.Valid {
		t.Fatalf("device after setup: status=%s model=%v", d.Status, d.ModelID)
	}
	if _, err := devices.Approve(ctx, d.ID); err != nil {
		t.Fatal(err)
	}

	got := call("GET", "/api/display", displayHeaders, "")
	if got["status"] != float64(0) || got["refresh_rate"] != float64(900) {
		t.Fatalf("active display = %v", got)
	}
	filename := got["filename"].(string)
	if got["image_url"] != "http://cnsl.test/images/"+filename {
		t.Fatalf("image_url = %v", got["image_url"])
	}

	// The placeholder must be a 4-bit grayscale PNG at panel size.
	resp, err := http.Get(srv.URL + "/images/" + filename)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := png.DecodeConfig(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Width != 1872 || cfg.Height != 1404 {
		t.Fatalf("image size %dx%d", cfg.Width, cfg.Height)
	}

	latest, err := st.LatestTelemetry(ctx, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	tel := latest[d.ID]
	if tel.BatteryPercent.Float64 != 87 || tel.RSSI.Int64 != -61 || tel.WakeReason.String != "timer" {
		t.Fatalf("telemetry = %+v", tel)
	}

	logBody := `{"logs":[{"id":7,"message":"Wi-Fi connected","level":"info","created_at":1793476800,"source_path":"src/bl.cpp","source_line":42,"wifi_signal":-61}]}`
	call("POST", "/api/log", map[string]string{"Access-Token": key, "Content-Type": "application/json"}, logBody)
	logs, err := st.DeviceLogs(ctx, store.LogFilter{DeviceID: d.ID, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || logs[0].Message != "Wi-Fi connected" || logs[0].SourceLine.Int64 != 42 || !logs[0].DeviceTime.Valid {
		t.Fatalf("logs = %+v", logs)
	}
	if !strings.Contains(logs[0].Extra.String, "wifi_signal") {
		t.Fatalf("extra = %s", logs[0].Extra.String)
	}

	// Legacy underscore headers still authenticate.
	if got := call("GET", "/api/display", map[string]string{"ACCESS_TOKEN": key}, ""); got["status"] != float64(0) {
		t.Fatalf("underscore header display = %v", got)
	}

	// A deleted device comes back as pending with its existing key.
	if err := devices.Delete(ctx, d.ID); err != nil {
		t.Fatal(err)
	}
	if got := call("GET", "/api/display", displayHeaders, ""); got["status"] != float64(202) {
		t.Fatalf("display after delete = %v", got)
	}
	readded, err := st.DeviceByAPIKey(ctx, key)
	if err != nil || readded.Status != store.DevicePending {
		t.Fatalf("re-added device: %+v, %v", readded, err)
	}
}
