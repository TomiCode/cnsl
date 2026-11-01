package render

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/TomiCode/cnsl/internal/imaging"
	"github.com/TomiCode/cnsl/internal/store"
)

func TestCleanup(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	images, cache := filepath.Join(dir, "images"), filepath.Join(dir, "cache")
	os.MkdirAll(images, 0o755)
	os.MkdirAll(cache, 0o755)
	st, err := store.Open(ctx, filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	r := NewRenderer(Options{Store: st, Files: imaging.Files{Dir: images}, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		CacheDir: cache})

	model, _ := st.DeviceModelByFirmware(ctx, "virtual")
	sc := store.Screen{Name: "s", ModelID: model.ID, FrameworkVersion: "x", RenderInterval: 900}
	if err := st.CreateScreen(ctx, &sc, nil); err != nil {
		t.Fatal(err)
	}
	const kept, orphan, fresh = "00000000000000000000000000000001.png", "00000000000000000000000000000002.png", "00000000000000000000000000000003.png"
	if err := st.InsertRender(ctx, &store.Render{ScreenID: sc.ID, Status: "ok", Trigger: "t",
		Filename: sql.NullString{String: kept, Valid: true}}); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-30 * 24 * time.Hour)
	write := func(path string, mod time.Time) {
		os.WriteFile(path, []byte("x"), 0o644)
		os.Chtimes(path, mod, mod)
	}
	write(filepath.Join(images, kept), old)   // referenced by a render: stays
	write(filepath.Join(images, orphan), old) // unreferenced and old: removed
	write(filepath.Join(images, fresh), time.Now())
	write(filepath.Join(images, "notes.txt"), old) // not ours: stays
	write(filepath.Join(cache, "stale.png"), old)
	write(filepath.Join(cache, "used.png"), time.Now())

	c, i := r.Cleanup(ctx, time.Now())
	if c != 1 || i != 1 {
		t.Fatalf("removed cache=%d images=%d, want 1 and 1", c, i)
	}
	for _, p := range []string{filepath.Join(images, kept), filepath.Join(images, fresh), filepath.Join(images, "notes.txt"), filepath.Join(cache, "used.png")} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("%s should still exist", p)
		}
	}
}
