package device

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/TomiCode/cnsl/internal/events"
	"github.com/TomiCode/cnsl/internal/imaging"
	"github.com/TomiCode/cnsl/internal/store"
)

func TestFirmwareOffer(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(ctx, filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	svc := NewService(st, events.NewBroker(), imaging.Files{Dir: dir}, time.UTC, slog.New(slog.NewTextHandler(io.Discard, nil)))

	model, err := st.DeviceModelByFirmware(ctx, "x")
	if err != nil {
		t.Fatal(err)
	}
	fw := store.Firmware{Version: "1.9.0", Chip: "esp32s3", Size: 1, SHA256: "abc", Filename: "fw.bin"}
	if err := st.InsertFirmware(ctx, &fw); err != nil {
		t.Fatal(err)
	}
	if err := st.SetModelFirmware(ctx, model.ID, sql.NullInt64{Int64: fw.ID, Valid: true}); err != nil {
		t.Fatal(err)
	}
	created, err := svc.Setup(ctx, Identity{MAC: "02:00:00:00:00:01", Model: "x", FirmwareVersion: "1.8.19"})
	if err != nil {
		t.Fatal(err)
	}
	load := func() *store.Device {
		d, err := st.Device(ctx, created.ID)
		if err != nil {
			t.Fatal(err)
		}
		return &d
	}
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	offer := func(at time.Time) string {
		name, err := svc.offerFirmware(ctx, load(), at)
		if err != nil {
			t.Fatal(err)
		}
		return name
	}

	if offer(now) != "" {
		t.Fatal("offered without auto-update")
	}
	if err := st.SetAutoUpdate(ctx, created.ID, true); err != nil {
		t.Fatal(err)
	}
	if got := offer(now); got != "abc.bin" {
		t.Fatalf("first offer = %q", got)
	}
	if offer(now.Add(time.Hour)) != "" {
		t.Fatal("re-offered within 24 h")
	}
	if st, _ := svc.OTA(ctx, *load()); st.State != OTAPending || st.Attempts != 1 {
		t.Fatalf("status = %+v", st)
	}
	offer(now.Add(25 * time.Hour))
	offer(now.Add(50 * time.Hour))
	if offer(now.Add(75*time.Hour)) != "" {
		t.Fatal("offered more than 3 times")
	}
	if st, _ := svc.OTA(ctx, *load()); st.State != OTAGaveUp {
		t.Fatalf("status = %+v", st)
	}

	// The device comes back on the new version: the offer state is cleared.
	d := load()
	d.FirmwareVersion = sql.NullString{String: "1.9.0", Valid: true}
	if err := st.UpdateDevice(ctx, d); err != nil {
		t.Fatal(err)
	}
	if offer(now.Add(80*time.Hour)) != "" {
		t.Fatal("offered to an up-to-date device")
	}
	if d := load(); d.OTAFirmwareID.Valid || d.OTAAttempts != 0 {
		t.Fatalf("offer state not cleared: %+v", d)
	}
	if st, _ := svc.OTA(ctx, *load()); st.State != OTACurrent {
		t.Fatalf("status = %+v", st)
	}
}
