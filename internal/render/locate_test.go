package render

import (
	"testing"

	"github.com/TomiCode/cnsl/internal/tmpl"
)

func TestAnnotateLines(t *testing.T) {
	src := `<div class="a">
  {{ if .Data.x }}<span>{{ .Data.x }}</span>{{ end }}
  <!-- <b>not this</b> -->
  {{- component "sensor" .Data.ha.sensor_x }}
  <style>.x{}</style>
</div>`
	got := annotateLines(src)
	want := `<div data-cnsl-line="1" class="a">
  {{ if .Data.x }}<span data-cnsl-line="2">{{ .Data.x }}</span>{{ end }}
  <!-- <b>not this</b> -->
  <div class="cnsl-block" data-cnsl-line="4">{{- component "sensor" .Data.ha.sensor_x }}</div>
  <style>.x{}</style>
</div>`
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
	if err := tmpl.Check("main", got); err != nil {
		t.Fatalf("annotated template does not parse: %v", err)
	}
}
