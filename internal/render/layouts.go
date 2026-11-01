package render

import (
	"math"
	"strings"

	"github.com/TomiCode/cnsl/internal/store"
)

// Slot is one region of a preset layout. Presets other than "full" are trmnl-framework mashups,
// so the framework sizes and spaces the views; the rectangle (fractions of the screen) is only an
// approximation used for .Region.Width/Height and the editor's overlay.
type Slot struct {
	Key  string
	View string // framework view variant: full, half_vertical, half_horizontal, quadrant
	X, Y float64
	W, H float64
}

// Layout is a preset, in the order the framework expects the views in the DOM.
type Layout struct {
	Key   string
	Name  string
	Slots []Slot
}

const (
	LayoutFull = "full"
	LayoutFree = "free"
)

var Layouts = []Layout{
	{LayoutFull, "Full screen", []Slot{{"main", "full", 0, 0, 1, 1}}},
	{"1Lx1R", "Left | right", []Slot{{"left", "half_vertical", 0, 0, .5, 1}, {"right", "half_vertical", .5, 0, .5, 1}}},
	{"1Tx1B", "Top / bottom", []Slot{{"top", "half_horizontal", 0, 0, 1, .5}, {"bottom", "half_horizontal", 0, .5, 1, .5}}},
	{"1Lx2R", "Left | 2 right", []Slot{{"left", "half_vertical", 0, 0, .5, 1}, {"top_right", "quadrant", .5, 0, .5, .5}, {"bottom_right", "quadrant", .5, .5, .5, .5}}},
	{"2Lx1R", "2 left | right", []Slot{{"top_left", "quadrant", 0, 0, .5, .5}, {"bottom_left", "quadrant", 0, .5, .5, .5}, {"right", "half_vertical", .5, 0, .5, 1}}},
	{"1Tx2B", "Top / 2 bottom", []Slot{{"top", "half_horizontal", 0, 0, 1, .5}, {"bottom_left", "quadrant", 0, .5, .5, .5}, {"bottom_right", "quadrant", .5, .5, .5, .5}}},
	{"2Tx1B", "2 top / bottom", []Slot{{"top_left", "quadrant", 0, 0, .5, .5}, {"top_right", "quadrant", .5, 0, .5, .5}, {"bottom", "half_horizontal", 0, .5, 1, .5}}},
	{"2x2", "Quadrants", []Slot{{"top_left", "quadrant", 0, 0, .5, .5}, {"top_right", "quadrant", .5, 0, .5, .5}, {"bottom_left", "quadrant", 0, .5, .5, .5}, {"bottom_right", "quadrant", .5, .5, .5, .5}}},
	{LayoutFree, "Free regions", nil},
}

func LayoutByKey(key string) (Layout, bool) {
	for _, l := range Layouts {
		if l.Key == key {
			return l, true
		}
	}
	return Layout{}, false
}

// DefaultTemplate is what a new full-screen region starts with. New regions use DefaultArrange,
// so their templates only list content: the region supplies the layout and title bar.
const DefaultTemplate = `<span class="value value--xxxlarge value--tnums">{{ date "15:04" .Now }}</span>
<span class="label label--large">{{ date "Monday, 2 January" .Now }}</span>
`

// slotTemplate is the starter content for a new region of a split layout.
const slotTemplate = `<span class="value value--large">{{ .Region.Key }}</span>
`

// FreeTemplate is the starter content for a free region (no framework view around it).
const FreeTemplate = `<span class="value value--large value--tnums">{{ date "15:04" .Now }}</span>
`

// Oriented returns the model as a screen with this rotation sees it: width and height swap for
// portrait (90/270), and the framework gets its portrait modifier.
func Oriented(m store.DeviceModel, rotation int) store.DeviceModel {
	if rotation == 90 || rotation == 270 {
		m.Width, m.Height = m.Height, m.Width
		m.ScreenClasses = strings.TrimSpace(m.ScreenClasses + " screen--portrait")
	}
	return m
}

// ValidRotation reports whether a screen rotation is supported.
func ValidRotation(r int) bool { return r == 0 || r == 90 || r == 180 || r == 270 }

// ApplyLayout returns the regions a screen should have for a preset layout. Regions whose key
// matches a slot keep their template; when switching between presets, templates of regions that
// don't fit any slot are moved into the remaining new slots in order, so no work is lost.
func ApplyLayout(layout Layout, model store.DeviceModel, existing []store.Region) []store.Region {
	byKey := make(map[string]store.Region, len(existing))
	for _, r := range existing {
		byKey[r.Key] = r
	}
	used := map[string]bool{}
	for _, s := range layout.Slots {
		if _, ok := byKey[s.Key]; ok {
			used[s.Key] = true
		}
	}
	var leftovers []store.Region
	for _, r := range existing {
		if !used[r.Key] {
			leftovers = append(leftovers, r)
		}
	}

	out := make([]store.Region, 0, len(layout.Slots))
	for _, s := range layout.Slots {
		r, ok := byKey[s.Key]
		if !ok {
			r = store.Region{Key: s.Key, Template: slotTemplate, Arrange: DefaultArrange.JSON()}
			if s.View == "full" {
				r.Template = DefaultTemplate
			}
			if len(leftovers) > 0 { // reuse an orphaned region (keeps its id and template)
				r = leftovers[0]
				leftovers = leftovers[1:]
				r.Key = s.Key
			}
		}
		r.X = int(math.Round(s.X * float64(model.Width)))
		r.Y = int(math.Round(s.Y * float64(model.Height)))
		r.W = int(math.Round(s.W * float64(model.Width)))
		r.H = int(math.Round(s.H * float64(model.Height)))
		r.Z = 0
		out = append(out, r)
	}
	return out
}

// legacyView reports whether a template already contains its framework view element (screens
// written before layouts existed), in which case the composer must not wrap it again.
func legacyView(template string) bool {
	return strings.HasPrefix(strings.TrimSpace(template), `<div class="view`)
}
