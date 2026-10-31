package device

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/TomiCode/cnsl/internal/events"
	"github.com/TomiCode/cnsl/internal/imaging"
	"github.com/TomiCode/cnsl/internal/store"
)

type fakeScreens struct {
	rendered  map[int64]bool
	requested []int64
}

func (f *fakeScreens) LatestImage(_ context.Context, id int64) (string, bool, error) {
	if !f.rendered[id] {
		return "", false, nil
	}
	return fmt.Sprintf("screen-%d.png", id), true, nil
}
func (f *fakeScreens) Request(id int64, _ string) { f.requested = append(f.requested, id) }
func (f *fakeScreens) Calibration(context.Context, int64) (string, error) {
	return "calibration.png", nil
}

func TestPlaylistRotation(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(ctx, filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	svc := NewService(st, events.NewBroker(), imaging.Files{Dir: dir}, time.UTC, slog.New(slog.NewTextHandler(io.Discard, nil)))
	screens := &fakeScreens{rendered: map[int64]bool{}}
	svc.SetScreens(screens)

	model, err := st.DeviceModelByFirmware(ctx, "virtual")
	if err != nil {
		t.Fatal(err)
	}
	var a, b, other store.Screen
	for _, sc := range []*store.Screen{&a, &b} {
		*sc = store.Screen{Name: "s", ModelID: model.ID, FrameworkVersion: "x", RenderInterval: 900}
		if err := st.CreateScreen(ctx, sc, nil); err != nil {
			t.Fatal(err)
		}
	}
	xModel, _ := st.DeviceModelByFirmware(ctx, "x")
	other = store.Screen{Name: "for another model", ModelID: xModel.ID, FrameworkVersion: "x", RenderInterval: 900}
	if err := st.CreateScreen(ctx, &other, nil); err != nil {
		t.Fatal(err)
	}

	pl := store.Playlist{Name: "p"}
	items := []store.PlaylistItem{
		{ScreenID: a.ID, Dwell: 2, Enabled: true},
		{ScreenID: other.ID, Dwell: 1, Enabled: true}, // skipped: wrong model
		{ScreenID: b.ID, Dwell: 1, Enabled: true, RefreshRate: sql.NullInt64{Int64: 60, Valid: true}},
	}
	if err := st.SavePlaylist(ctx, &pl, items); err != nil {
		t.Fatal(err)
	}

	d, err := svc.Setup(ctx, Identity{MAC: "02:00:00:00:00:01", Model: "virtual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Approve(ctx, d.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AssignPlaylist(ctx, d.ID, &pl.ID); err != nil {
		t.Fatal(err)
	}
	wake := func() DisplayResult {
		t.Helper()
		res, err := svc.Display(ctx, DisplayRequest{APIKey: d.APIKey})
		if err != nil {
			t.Fatal(err)
		}
		return res
	}

	// Nothing rendered yet: the device gets the placeholder, a short refresh, and a render request.
	if res := wake(); res.RefreshRate != waitingRefresh || len(screens.requested) == 0 {
		t.Fatalf("unrendered: %+v, requested %v", res, screens.requested)
	}

	screens.rendered[a.ID], screens.rendered[b.ID] = true, true
	var got []string
	var rates []int
	for range 6 {
		res := wake()
		got = append(got, res.Filename)
		rates = append(rates, res.RefreshRate)
	}
	want := []string{
		fmt.Sprintf("screen-%d.png", a.ID), fmt.Sprintf("screen-%d.png", a.ID), fmt.Sprintf("screen-%d.png", b.ID),
		fmt.Sprintf("screen-%d.png", a.ID), fmt.Sprintf("screen-%d.png", a.ID), fmt.Sprintf("screen-%d.png", b.ID),
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("rotation = %v, want %v", got, want)
	}
	if rates[0] != 900 || rates[2] != 60 {
		t.Fatalf("refresh rates = %v", rates)
	}

	// Calibration overrides the playlist.
	if _, err := svc.StartCalibration(ctx, d.ID); err != nil {
		t.Fatal(err)
	}
	if res := wake(); res.Filename != "calibration.png" || res.RefreshRate != calibrationRefresh {
		t.Fatalf("calibrating: %+v", res)
	}
}
