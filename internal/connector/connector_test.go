package connector

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TomiCode/cnsl/internal/store"
)

func fakeHA(t *testing.T, fail *atomic.Bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail != nil && fail.Load() {
			http.Error(w, "down", http.StatusBadGateway)
			return
		}
		if r.URL.Path != "/api/states" || r.Header.Get("Authorization") != "Bearer secret-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		io.WriteString(w, `[{"entity_id":"sensor.outdoor_temperature","state":"12.5","attributes":{"unit_of_measurement":"°C","friendly_name":"Outdoor"}},
			{"entity_id":"binary_sensor.door","state":"off","attributes":{}}]`)
	}))
}

func TestHomeAssistant(t *testing.T) {
	srv := fakeHA(t, nil)
	defer srv.Close()
	env := Env{HTTP: srv.Client()}
	cfg := Values{"url": srv.URL, "token": "secret-token"}

	data, err := homeAssistant{}.Fetch(context.Background(), env, cfg, Values{"entities": "sensor.outdoor_temperature\nbinary_sensor.door\nsensor.nope"})
	if err != nil {
		t.Fatal(err)
	}
	m := data.(map[string]any)
	temp := m["sensor_outdoor_temperature"].(map[string]any)
	if temp["value"] != 12.5 || temp["unit"] != "°C" || temp["name"] != "Outdoor" {
		t.Fatalf("temperature = %v", temp)
	}
	if m["binary_sensor_door"].(map[string]any)["state"] != "off" {
		t.Fatalf("door = %v", m["binary_sensor_door"])
	}
	if missing := m["_missing"].([]string); len(missing) != 1 || missing[0] != "sensor.nope" {
		t.Fatalf("missing = %v", m["_missing"])
	}

	if _, err := (homeAssistant{}).Fetch(context.Background(), env, Values{"url": srv.URL, "token": "wrong"}, Values{"entities": "a.b"}); err == nil {
		t.Fatal("expected an auth error")
	}
}

func TestJSONPointer(t *testing.T) {
	doc := map[string]any{"items": []any{map[string]any{"a/b": 1.0}}}
	if v, err := jsonPointer(doc, "/items/0/a~1b"); err != nil || v != 1.0 {
		t.Fatalf("got %v, %v", v, err)
	}
	if _, err := jsonPointer(doc, "/items/5"); err == nil {
		t.Fatal("expected out-of-range error")
	}
}

func TestStatusChecks(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ok.Close()
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	closedAddr := ln.Addr().String()
	ln.Close() // nothing listens here any more

	data, err := statusChecks{}.Fetch(context.Background(), Env{}, Values{"timeout": 2.0},
		Values{"checks": []any{"web = " + ok.URL, "ssh = tcp://" + closedAddr}})
	if err != nil {
		t.Fatal(err)
	}
	m := data.(map[string]any)
	checks := m["checks"].([]any)
	if checks[0].(map[string]any)["up"] != true || checks[1].(map[string]any)["up"] != false || m["up"] != 1 {
		t.Fatalf("checks = %v", m)
	}
}

func TestSchemaSecrets(t *testing.T) {
	s := homeAssistant{}.ConfigSchema()
	if _, err := s.Normalize(Values{"url": "ftp://x"}); err == nil {
		t.Fatal("expected URL validation error")
	}
	stored, err := s.Normalize(Values{"url": "http://ha.local:8123/", "token": "abc"})
	if err != nil || stored["url"] != "http://ha.local:8123" {
		t.Fatalf("normalize: %v %v", stored, err)
	}
	masked, set := s.MaskSecrets(stored)
	if _, ok := masked["token"]; ok || len(set) != 1 {
		t.Fatalf("mask: %v %v", masked, set)
	}
	kept := s.KeepSecrets(Values{"url": "http://new"}, stored)
	if kept["token"] != "abc" {
		t.Fatalf("keep secrets: %v", kept)
	}
}

// The cache serves fetched data, refetches when expired, and keeps old data (stale) when a
// refresh fails.
func TestServiceCacheAndStale(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var fail atomic.Bool
	srv := fakeHA(t, &fail)
	defer srv.Close()

	svc := NewService(st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	svc.env.HTTP = srv.Client()

	model, _ := st.DeviceModelByFirmware(ctx, "virtual")
	sc := store.Screen{Name: "s", ModelID: model.ID, FrameworkVersion: "x", RenderInterval: 900}
	if err := st.CreateScreen(ctx, &sc, nil); err != nil {
		t.Fatal(err)
	}
	conn := store.Connector{Type: "homeassistant", Name: "HA", Config: Values{"url": srv.URL, "token": "secret-token"}.JSON()}
	if err := st.SaveConnector(ctx, &conn); err != nil {
		t.Fatal(err)
	}
	b := store.Binding{ConnectorID: conn.ID, Alias: "ha", Params: `{"entities":["sensor.outdoor_temperature"]}`, TTL: 30, Enabled: true,
		RegionKey: sql.NullString{}}
	if err := st.ReplaceBindings(ctx, sc.ID, []store.Binding{b}); err != nil {
		t.Fatal(err)
	}

	res, err := svc.Resolve(ctx, sc.ID, "main", false)
	if err != nil || res["ha"].Error != "" || res["ha"].Stale {
		t.Fatalf("first resolve: %+v %v", res, err)
	}

	// Expire the entry and make the source fail: old data comes back, marked stale.
	params, _ := ParseValues(b.Params)
	key := CacheKey(conn.ID, params)
	e, _ := st.CacheEntry(ctx, key)
	e.ExpiresAt = store.Time{Time: time.Now().Add(-time.Minute).UTC()}
	if err := st.SaveCacheEntry(ctx, e); err != nil {
		t.Fatal(err)
	}
	fail.Store(true)
	res, _ = svc.Resolve(ctx, sc.ID, "main", false)
	if !res["ha"].Stale || res["ha"].Data == nil || res["ha"].Error == "" {
		t.Fatalf("stale resolve: %+v", res["ha"])
	}

	// Sample data needs no network.
	res, _ = svc.Resolve(ctx, sc.ID, "main", true)
	if res["ha"].Data.(map[string]any)["sensor_outdoor_temperature"] == nil {
		t.Fatalf("sample: %+v", res["ha"])
	}
}

// TestOpenMeteoLive calls the real API; run with CNSL_LIVE=1.
func TestOpenMeteoLive(t *testing.T) {
	if os.Getenv("CNSL_LIVE") == "" {
		t.Skip("set CNSL_LIVE=1 to call the real Open-Meteo API")
	}
	data, err := openMeteo{}.Fetch(context.Background(), Env{HTTP: http.DefaultClient}, nil,
		Values{"latitude": 48.2082, "longitude": 16.3738, "days": 2.0})
	if err != nil {
		t.Fatal(err)
	}
	m := data.(map[string]any)
	cur := m["current"].(map[string]any)
	if len(m["daily"].([]any)) != 2 || cur["condition"] == "" {
		t.Fatalf("unexpected data: %v", m)
	}
	t.Logf("now: %v°, %v, pressure %v", cur["temperature"], cur["condition"], cur["pressure"])
}
