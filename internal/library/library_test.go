package library

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TomiCode/cnsl/internal/connector"
	"github.com/TomiCode/cnsl/internal/store"
	"github.com/TomiCode/cnsl/internal/tmpl"
)

func TestSeedAndBuiltins(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	// A user component that takes a built-in's key keeps it.
	mine := store.Component{Key: "clock", Name: "My clock", Source: "mine", Inputs: "[]"}
	if err := st.SaveComponent(ctx, &mine); err != nil {
		t.Fatal(err)
	}
	for range 2 { // seeding is idempotent
		if err := Seed(ctx, st); err != nil {
			t.Fatal(err)
		}
	}
	lib, err := Load(ctx, st)
	if err != nil {
		t.Fatal(err)
	}
	if len(lib) != len(builtinComponents) || lib["clock"].Source != "mine" {
		t.Fatalf("library = %d components, clock = %q", len(lib), lib["clock"].Source)
	}
	snippets, _ := st.Snippets(ctx)
	if len(snippets) != len(builtinSnippets) {
		t.Fatalf("snippets = %d", len(snippets))
	}

	// Every built-in renders with sample data, and without data at all.
	for _, b := range builtinComponents {
		if b.Key == "clock" {
			continue
		}
		if err := tmpl.ValidateInputs(b.Inputs); err != nil {
			t.Fatalf("%s: %v", b.Key, err)
		}
		args := ""
		data := map[string]any{}
		for _, in := range b.Inputs {
			if in.Kind != "data" {
				continue
			}
			typ, ok := connector.Lookup(in.Connector)
			if !ok {
				t.Fatalf("%s: unknown connector %q", b.Key, in.Connector)
			}
			v := in.Sample
			if v == nil {
				v = typ.Sample(connector.Values{})
			}
			data[in.Name] = v
			args += ` "` + in.Name + `" .Data.` + in.Name
		}
		ctx := tmpl.Context{Now: time.Date(2026, 11, 1, 9, 30, 0, 0, time.UTC), Screen: tmpl.ScreenInfo{Name: "Home"}, Data: data}
		out, err := tmpl.Render("main", `{{ component "`+b.Key+`"`+args+` }}`, ctx, lib)
		if err != nil {
			t.Fatalf("%s: %v", b.Key, err)
		}
		if strings.Contains(string(out), "No ") && args != "" {
			t.Errorf("%s rendered the empty state with sample data:\n%s", b.Key, out)
		}
		if _, err := tmpl.Render("main", `{{ component "`+b.Key+`" }}`, tmpl.Context{}, lib); err != nil {
			t.Errorf("%s without data: %v", b.Key, err)
		}
	}
}
