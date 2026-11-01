package tmpl

import (
	"strings"
	"testing"
	"time"
)

type testLib map[string]Component

func (l testLib) Component(key string) (Component, bool) { c, ok := l[key]; return c, ok }

func TestComponent(t *testing.T) {
	lib := testLib{
		"greet": {Key: "greet", Source: `{{ .greeting }}, {{ .name }}! ({{ .Region.Key }})`,
			Inputs: []Input{{Name: "name", Kind: "string"}, {Name: "greeting", Kind: "string", Default: "Hello"}}},
		"twice": {Key: "twice", Source: `{{ component "greet" .who }}|{{ component "greet" "name" .who "greeting" "Bye" }}`,
			Inputs: []Input{{Name: "who", Kind: "string"}}},
		"loop": {Key: "loop", Source: `{{ component "loop" }}`},
		"days": {Key: "days", Source: `{{ range take .n .items }}{{ . }}{{ end }} {{ num .n 1 }}`,
			Inputs: []Input{{Name: "items", Kind: "data"}, {Name: "n", Kind: "number", Default: float64(2)}}},
	}
	ctx := Context{Region: RegionInfo{Key: "main"}, Data: map[string]any{"who": "Ada", "items": []any{"a", "b", "c"}}}

	cases := map[string]string{
		`{{ component "greet" .Data.who }}`:                "Hello, Ada! (main)",
		`{{ component "twice" .Data.who }}`:                "Hello, Ada! (main)|Bye, Ada! (main)",
		`{{ component "days" .Data.items }}`:               "ab 1.00",
		`{{ component "days" "items" .Data.items "n" 3 }}`: "abc 1.000",
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

	errs := map[string]string{
		`{{ component "nope" }}`:              `unknown component "nope"`,
		`{{ component "loop" }}`:              "nested more than",
		`{{ component "greet" "bad" 1 }}`:     "unknown input bad",
		`{{ component "greet" "a" "b" "c" }}`: "name/value pairs",
	}
	for src, want := range errs {
		_, err := Render("main", src, ctx, lib)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v, want %q", src, err, want)
		}
	}
}

func TestComponentEscapes(t *testing.T) {
	lib := testLib{"b": {Key: "b", Source: `<b>{{ .v }}</b>`, Inputs: []Input{{Name: "v", Kind: "string"}}}}
	out, err := Render("main", `<i>{{ component "b" "<x>" }}</i>`, Context{}, lib)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `<i><b>&lt;x&gt;</b></i>` {
		t.Fatalf("got %q", out)
	}
}

func TestValidateInputs(t *testing.T) {
	bad := [][]Input{
		{{Name: "1x", Kind: "string"}},
		{{Name: "a", Kind: "string"}, {Name: "a", Kind: "bool"}},
		{{Name: "a", Kind: "weird"}},
		{{Name: "Data", Kind: "data"}},
	}
	for _, in := range bad {
		if ValidateInputs(in) == nil {
			t.Errorf("%v: want error", in)
		}
	}
	if err := ValidateInputs([]Input{{Name: "weather", Kind: "data"}, {Name: "days", Kind: "number"}}); err != nil {
		t.Fatal(err)
	}
}

func TestCalendar(t *testing.T) {
	// November 2026 starts on a Sunday.
	weeks := calendar(mustDate("2026-11-15"))
	if len(weeks) != 6 || weeks[0][6] != 1 || weeks[0][5] != 0 || weeks[5][0] != 30 {
		t.Fatalf("weeks = %v", weeks)
	}
}

func mustDate(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}
