package tmpl

import (
	"strings"
	"testing"
	"time"
)

func TestLanguageHelpers(t *testing.T) {
	warsaw, _ := time.LoadLocation("Europe/Warsaw")
	ctx := Context{Now: time.Date(2026, 11, 1, 9, 5, 0, 0, warsaw), Lang: "pl",
		Data: map[string]any{"day": "2026-11-03", "code": float64(61)}}
	lib := testLib{"c": {Key: "c", Source: `{{ date "Mon" .day }}`, Inputs: []Input{{Name: "day", Kind: "string"}}}}
	cases := map[string]string{
		`{{ date "Monday, 2 January" .Now }}`: "Niedziela, 1 listopada",
		`{{ date "Mon 2" .Data.day }}`:        "Wt. 3",
		`{{ condition .Data.code }}`:          "Deszcz",
		`{{ t "humidity" }}`:                  "Wilgotność",
		`{{ join " " weekdays }}`:             "Pn Wt Śr Cz Pt So Nd",
		`{{ component "c" "day" .Data.day }}`: "Wt.",
	}
	for src, want := range cases {
		out, err := Render("main", src, ctx, lib)
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		if string(out) != want {
			t.Errorf("%s = %q, want %q", src, out, want)
		}
	}
	ctx.Lang = ""
	if out, _ := Render("main", `{{ date "Monday" .Now }}`, ctx, nil); out != "Sunday" {
		t.Errorf("default language = %q", out)
	}
	if _, err := Render("main", `{{ date "Mon" "soon" }}`, ctx, nil); err == nil {
		t.Error("want an error for an unreadable time")
	}
}

func TestIcon(t *testing.T) {
	out, err := Render("main", `{{ icon "wi-day-sunny" }}{{ icon "thermometer" 40 3 }}`, Context{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if strings.Count(s, "<svg") != 2 || !strings.Contains(s, `fill="currentColor"`) || !strings.Contains(s, `stroke-width="3"`) || !strings.Contains(s, `width="40"`) {
		t.Fatalf("icons = %s", s)
	}
	if _, err := Render("main", `{{ icon "no-such-icon" }}`, Context{}, nil); err == nil || !strings.Contains(err.Error(), "unknown icon") {
		t.Fatalf("err = %v", err)
	}
}
