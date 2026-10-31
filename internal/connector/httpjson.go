package connector

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// HTTP JSON: GET any JSON API; optionally select part of the response with a JSON pointer.
type httpJSON struct{}

func init() { register(httpJSON{}) }

func (httpJSON) Key() string  { return "http_json" }
func (httpJSON) Name() string { return "HTTP JSON" }
func (httpJSON) Description() string {
	return "Any JSON API. The (optionally narrowed) response is available as .Data.<alias>."
}

func (httpJSON) ConfigSchema() Schema {
	return Schema{
		{Key: "base_url", Label: "Base URL", Kind: "url", Required: true, Help: "e.g. https://api.example.com"},
		{Key: "headers", Label: "Headers", Kind: "secret", Help: "Optional, one per line: Authorization: Bearer …"},
	}
}

func (httpJSON) ParamsSchema() Schema {
	return Schema{
		{Key: "path", Label: "Path", Kind: "string", Help: "Appended to the base URL, e.g. /v1/status?full=1"},
		{Key: "pointer", Label: "JSON pointer", Kind: "string", Help: "Optional part of the response, e.g. /items/0 (RFC 6901)"},
	}
}

func (httpJSON) Fetch(ctx context.Context, env Env, config, params Values) (any, error) {
	headers := map[string]string{}
	for _, line := range strings.Split(config.String("headers"), "\n") {
		if name, value, ok := strings.Cut(line, ":"); ok && strings.TrimSpace(name) != "" {
			headers[strings.TrimSpace(name)] = strings.TrimSpace(value)
		}
	}
	path := params.String("path")
	if path != "" && !strings.HasPrefix(path, "/") && !strings.HasPrefix(path, "?") {
		path = "/" + path
	}
	var data any
	if err := getJSON(ctx, env.HTTP, config.String("base_url")+path, headers, &data); err != nil {
		return nil, err
	}
	return jsonPointer(data, params.String("pointer"))
}

// jsonPointer resolves an RFC 6901 pointer ("" = whole document).
func jsonPointer(doc any, pointer string) (any, error) {
	if pointer == "" || pointer == "/" {
		return doc, nil
	}
	if !strings.HasPrefix(pointer, "/") {
		return nil, fmt.Errorf("JSON pointer must start with /")
	}
	cur := doc
	for _, tok := range strings.Split(pointer[1:], "/") {
		tok = strings.NewReplacer("~1", "/", "~0", "~").Replace(tok)
		switch v := cur.(type) {
		case map[string]any:
			next, ok := v[tok]
			if !ok {
				return nil, fmt.Errorf("JSON pointer %s: no key %q", pointer, tok)
			}
			cur = next
		case []any:
			i, err := strconv.Atoi(tok)
			if err != nil || i < 0 || i >= len(v) {
				return nil, fmt.Errorf("JSON pointer %s: no index %q", pointer, tok)
			}
			cur = v[i]
		default:
			return nil, fmt.Errorf("JSON pointer %s: %q is not inside an object or array", pointer, tok)
		}
	}
	return cur, nil
}

func (httpJSON) Sample(Values) any {
	return map[string]any{"status": "ok", "items": []any{map[string]any{"name": "Example", "value": 42}}}
}
