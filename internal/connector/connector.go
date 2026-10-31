// Package connector fetches external data for screens. A connector type (Home Assistant,
// weather, …) is Go code; a connector is a configured instance of a type (URL, token); a binding
// attaches a connector to a screen under an alias with its own params. Fetched data is cached in
// SQLite and refreshed in the background, so renders never wait on the network for data they
// have seen before.
package connector

import (
	"context"
	"net/http"
	"slices"
)

// Type is one kind of data source.
type Type interface {
	Key() string
	Name() string
	Description() string
	ConfigSchema() Schema // connection settings, shared by all bindings
	ParamsSchema() Schema // what one binding asks for
	// Fetch returns JSON-like data (maps, slices, strings, float64, bool) for templates.
	Fetch(ctx context.Context, env Env, config, params Values) (any, error)
	// Sample returns realistic fake data for previews before a real fetch is possible.
	Sample(params Values) any
}

// Env is what a fetch may use.
type Env struct {
	HTTP *http.Client
}

var registry = map[string]Type{}

func register(t Type) {
	registry[t.Key()] = t
}

func Lookup(key string) (Type, bool) {
	t, ok := registry[key]
	return t, ok
}

// Types returns all connector types, sorted by name.
func Types() []Type {
	out := make([]Type, 0, len(registry))
	for _, t := range registry {
		out = append(out, t)
	}
	slices.SortFunc(out, func(a, b Type) int {
		if a.Name() < b.Name() {
			return -1
		}
		return 1
	})
	return out
}
