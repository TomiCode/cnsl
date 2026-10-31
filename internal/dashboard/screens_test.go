package dashboard

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/TomiCode/cnsl/internal/render"
	"github.com/TomiCode/cnsl/internal/store"
)

// Changing the layout and filling in the new regions in the same save must keep the content
// (the editor sends the new regions without ids).
func TestMergeRegionsLayoutChangeKeepsContent(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	model, _ := st.DeviceModelByFirmware(ctx, "x")
	sc := store.Screen{Name: "s", ModelID: model.ID, FrameworkVersion: "x", RenderInterval: 900}
	if err := st.CreateScreen(ctx, &sc, []store.Region{{Key: "main", W: model.Width, H: model.Height, Template: "old main"}}); err != nil {
		t.Fatal(err)
	}
	main, _ := st.Regions(ctx, sc.ID)
	mainID := main[0].ID

	s := &Server{store: st}
	layout, _ := render.LayoutByKey("1Lx2R")
	tr := true
	out, err := s.mergeRegions(ctx, sc, model, layout, []regionInput{
		{Id: &mainID, Key: "left", Template: "left content"},
		{Key: "top_right", Template: "top right content", Transparent: &tr},
		{Key: "bottom_right", Template: "bottom right content"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]store.Region{}
	for _, r := range out {
		got[r.Key] = r
	}
	if got["left"].Template != "left content" || got["left"].ID != mainID {
		t.Fatalf("left = %+v (want main's id %d reused)", got["left"], mainID)
	}
	if got["top_right"].Template != "top right content" || !got["top_right"].Transparent {
		t.Fatalf("top_right = %+v", got["top_right"])
	}
	if got["bottom_right"].Template != "bottom right content" {
		t.Fatalf("bottom_right = %+v", got["bottom_right"])
	}
	if got["left"].W != model.Width/2 || got["top_right"].H != model.Height/2 {
		t.Fatalf("rects not from the layout: %+v", out)
	}
}
