package render

import (
	"strings"
	"testing"

	"github.com/TomiCode/cnsl/internal/tmpl"
)

type testLib map[string]tmpl.Component

func (l testLib) Component(key string) (tmpl.Component, bool) { c, ok := l[key]; return c, ok }

func TestBlocksTemplate(t *testing.T) {
	lib := testLib{"sensor": {Key: "sensor", Inputs: []tmpl.Input{
		{Name: "entity", Kind: "data"}, {Name: "label", Kind: "string"}, {Name: "places", Kind: "number"}, {Name: "big", Kind: "bool"},
	}}}
	blocks := []Block{
		{Component: "sensor", Args: map[string]BlockArg{
			"places": {Value: float64(2)}, "entity": {Path: ".Data.ha.sensor_x"}, "label": {Value: `Living "room"`}, "big": {Value: true},
		}},
		{HTML: `<span class="title">{{ .Screen.Name }}</span>`},
		{Component: "sensor", Args: map[string]BlockArg{"entity": {Path: ".Data.ha.sensor_y"}, "label": {}}},
	}
	got, err := BlocksTemplate(blocks, lib)
	if err != nil {
		t.Fatal(err)
	}
	want := `<div class="cnsl-block" data-cnsl-block="0">{{ component "sensor" "entity" .Data.ha.sensor_x "label" "Living \"room\"" "places" 2 "big" true }}</div>
<div class="cnsl-block" data-cnsl-block="1"><span class="title">{{ .Screen.Name }}</span></div>
<div class="cnsl-block" data-cnsl-block="2">{{ component "sensor" "entity" .Data.ha.sensor_y }}</div>
`
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
	if err := tmpl.Check("main", got); err != nil {
		t.Fatalf("generated template does not parse: %v", err)
	}

	bad := map[string][]Block{
		"unknown component": {{Component: "nope"}},
		"has no input":      {{Component: "sensor", Args: map[string]BlockArg{"x": {Value: "1"}}}},
		"not a data path":   {{Component: "sensor", Args: map[string]BlockArg{"entity": {Path: `.Data }}{{ evil`}}}},
		"not a number":      {{Component: "sensor", Args: map[string]BlockArg{"places": {Value: "two"}}}},
	}
	for want, blocks := range bad {
		if _, err := BlocksTemplate(blocks, lib); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v", want, err)
		}
	}
}
