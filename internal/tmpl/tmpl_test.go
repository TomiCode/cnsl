package tmpl

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRender(t *testing.T) {
	ctx := Context{
		Now:    time.Date(2026, 10, 31, 9, 5, 0, 0, time.UTC),
		Region: RegionInfo{Key: "main", Width: 800, Height: 480},
		Data:   map[string]any{"temp": 21.456},
	}
	out, err := Render("main", `<b>{{ date "Mon 2 Jan" .Now }}</b> {{ num 1 .Data.temp }}°{{ default "—" .Data.missing }} <i>{{ "<x>" }}</i>`, ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := `<b>Sat 31 Oct</b> 21.5°— <i>&lt;x&gt;</i>`
	if string(out) != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

func TestErrorPositions(t *testing.T) {
	_, err := Render("main", "line one\n{{ .Now.Foo }}", Context{}, nil)
	var e *Error
	if !errors.As(err, &e) || e.Line != 2 || e.Region != "main" {
		t.Fatalf("exec error = %#v", err)
	}

	err = Check("main", "ok\n\n  {{ if }}")
	if !errors.As(err, &e) || e.Line != 3 {
		t.Fatalf("parse error = %#v", err)
	}
	if !strings.Contains(e.Message, "if") {
		t.Fatalf("message = %q", e.Message)
	}
}

func TestOutputLimit(t *testing.T) {
	_, err := Render("main", `{{ range seq 3000000 }}x{{ end }}`, Context{}, nil)
	var e *Error
	if !errors.As(err, &e) || !strings.Contains(e.Message, "too large") {
		t.Fatalf("err = %v", err)
	}
}
