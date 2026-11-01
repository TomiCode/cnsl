// Package icons serves the vendored icon sets (see cmd/vendor-icons): Weather Icons (names
// starting with "wi-", filled shapes) and Lucide (everything else, stroked outlines). Templates
// inline them as SVG so they stay crisp on e-ink and take the text color.
package icons

import (
	"compress/gzip"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
)

//go:embed data/*.json.gz
var data embed.FS

const (
	SetWeather = "wi"
	SetLucide  = "lucide"
)

type Icon struct {
	Name    string   `json:"name"`
	Set     string   `json:"set"`
	ViewBox string   `json:"viewBox"`
	Body    string   `json:"body"`
	Tags    []string `json:"tags,omitempty"`
}

var (
	loadOnce sync.Once
	byName   map[string]*Icon
	all      []*Icon
	loadErr  error
)

func load() {
	byName = map[string]*Icon{}
	for _, set := range []string{SetWeather, SetLucide} {
		f, err := data.Open("data/" + set + ".json.gz")
		if err != nil {
			loadErr = err
			return
		}
		zr, err := gzip.NewReader(f)
		if err != nil {
			loadErr = err
			return
		}
		var s struct{ Icons []*Icon }
		err = json.NewDecoder(zr).Decode(&s)
		f.Close()
		if err != nil {
			loadErr = fmt.Errorf("icons %s: %w", set, err)
			return
		}
		for _, ic := range s.Icons {
			ic.Set = set
			byName[ic.Name] = ic
			all = append(all, ic)
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })
}

// All lists every icon, sorted by name.
func All() ([]*Icon, error) {
	loadOnce.Do(load)
	return all, loadErr
}

// Lookup finds an icon by name ("wi-day-sunny", "thermometer"; "lucide:thermometer" works too).
func Lookup(name string) (*Icon, bool) {
	loadOnce.Do(load)
	ic, ok := byName[strings.TrimPrefix(name, "lucide:")]
	return ic, ok
}

// Default stroke width for outline icons: Lucide's 2 (of 24) washes out on the panel.
const defaultStroke = 2.5

// SVG renders an icon inline at size×size CSS pixels. stroke applies to outline icons (0 = default).
func SVG(name string, size, stroke float64) (template.HTML, error) {
	ic, ok := Lookup(name)
	if !ok {
		return "", fmt.Errorf("unknown icon %q", name)
	}
	if size <= 0 {
		size = 32
	}
	if stroke <= 0 {
		stroke = defaultStroke
	}
	return template.HTML(ic.markup(fmtNum(size), fmtNum(stroke), "cnsl-icon")), nil
}

func (ic *Icon) markup(size, stroke, class string) string {
	paint := `fill="currentColor"`
	if ic.Set == SetLucide {
		paint = `fill="none" stroke="currentColor" stroke-width="` + stroke + `" stroke-linecap="round" stroke-linejoin="round"`
	}
	// The body is checked when vendoring: plain shapes only.
	return `<svg xmlns="http://www.w3.org/2000/svg" class="` + class + `" width="` + size + `" height="` + size +
		`" viewBox="` + ic.ViewBox + `" ` + paint + ` aria-hidden="true">` + ic.Body + `</svg>`
}

func fmtNum(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// Handler serves GET /<name>.svg standalone (the dashboard's icon browser).
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name, ok := strings.CutSuffix(strings.TrimPrefix(r.URL.Path, "/"), ".svg")
		ic, found := Lookup(name)
		if !ok || !found {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = w.Write([]byte(ic.markup("48", "2", "")))
	})
}
