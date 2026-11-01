// Package library holds reusable template parts: components (called as {{ component "key" … }})
// and editor snippets. Built-ins ship with cnsl and are refreshed on every start; users add their
// own next to them.
package library

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"

	"github.com/TomiCode/cnsl/internal/store"
	"github.com/TomiCode/cnsl/internal/tmpl"
)

//go:embed components/*.html
var sources embed.FS

// Lib resolves components for a render. It is a snapshot; load a new one per render.
type Lib map[string]tmpl.Component

func (l Lib) Component(key string) (tmpl.Component, bool) {
	c, ok := l[key]
	return c, ok
}

// Load reads all components from the store.
func Load(ctx context.Context, st *store.Store) (Lib, error) {
	rows, err := st.Components(ctx)
	if err != nil {
		return nil, err
	}
	lib := make(Lib, len(rows))
	for _, row := range rows {
		c, err := FromStore(row)
		if err != nil {
			return nil, err
		}
		lib[c.Key] = c
	}
	return lib, nil
}

// FromStore converts a stored component.
func FromStore(row store.Component) (tmpl.Component, error) {
	inputs, err := ParseInputs(row.Inputs)
	if err != nil {
		return tmpl.Component{}, fmt.Errorf("component %q: %w", row.Key, err)
	}
	return tmpl.Component{Key: row.Key, Source: row.Source, Inputs: inputs}, nil
}

func ParseInputs(raw string) ([]tmpl.Input, error) {
	inputs := []tmpl.Input{}
	if raw == "" {
		return inputs, nil
	}
	if err := json.Unmarshal([]byte(raw), &inputs); err != nil {
		return nil, fmt.Errorf("invalid inputs: %w", err)
	}
	return inputs, nil
}

// Seed creates or refreshes the built-in components and snippets, and removes built-ins that no
// longer ship. User entries are never touched.
func Seed(ctx context.Context, st *store.Store) error {
	keep := map[string]bool{}
	for _, b := range builtinComponents {
		src, err := sources.ReadFile("components/" + b.Key + ".html")
		if err != nil {
			return err
		}
		inputs, err := json.Marshal(b.Inputs)
		if err != nil {
			return err
		}
		if err := st.UpsertBuiltinComponent(ctx, store.Component{Key: b.Key, Name: b.Name,
			Description: b.Description, Source: string(src), Inputs: string(inputs)}); err != nil {
			return fmt.Errorf("seed component %s: %w", b.Key, err)
		}
		keep[b.Key] = true
	}
	rows, err := st.Components(ctx)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row.Builtin && !keep[row.Key] {
			if err := st.DeleteComponent(ctx, row.ID); err != nil {
				return err
			}
		}
	}

	keep = map[string]bool{}
	for _, sn := range builtinSnippets {
		if err := st.UpsertBuiltinSnippet(ctx, sn); err != nil {
			return fmt.Errorf("seed snippet %s: %w", sn.Prefix, err)
		}
		keep[sn.Prefix] = true
	}
	snippets, err := st.Snippets(ctx)
	if err != nil {
		return err
	}
	for _, sn := range snippets {
		if sn.Builtin && !keep[sn.Prefix] {
			if err := st.DeleteSnippet(ctx, sn.ID); err != nil {
				return err
			}
		}
	}
	return nil
}
