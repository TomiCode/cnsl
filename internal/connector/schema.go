package connector

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Field describes one setting of a connector (its config) or of a binding (its params). The
// dashboard renders forms from these.
type Field struct {
	Key      string   `json:"key"`
	Label    string   `json:"label"`
	Help     string   `json:"help,omitempty"`
	Kind     string   `json:"kind"` // string | secret | url | int | number | bool | select | list | text
	Required bool     `json:"required,omitempty"`
	Default  any      `json:"default,omitempty"`
	Options  []string `json:"options,omitempty"` // for select
}

type Schema []Field

// Values are a config or params object as stored (JSON object).
type Values map[string]any

func ParseValues(s string) (Values, error) {
	v := Values{}
	if strings.TrimSpace(s) == "" {
		return v, nil
	}
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return nil, fmt.Errorf("invalid settings JSON: %w", err)
	}
	return v, nil
}

func (v Values) JSON() string {
	b, _ := json.Marshal(v)
	return string(b)
}

func (v Values) String(key string) string {
	switch x := v[key].(type) {
	case string:
		return strings.TrimSpace(x)
	case nil:
		return ""
	default:
		return fmt.Sprint(x)
	}
}

func (v Values) Float(key string, fallback float64) float64 {
	switch x := v[key].(type) {
	case float64:
		return x
	case string:
		if f, err := strconv.ParseFloat(strings.TrimSpace(x), 64); err == nil {
			return f
		}
	}
	return fallback
}

func (v Values) Int(key string, fallback int) int {
	return int(v.Float(key, float64(fallback)))
}

func (v Values) Bool(key string) bool {
	switch x := v[key].(type) {
	case bool:
		return x
	case string:
		return x == "true" || x == "1"
	}
	return false
}

// List reads a list setting: a JSON array, or text with one item per line (commas also split).
func (v Values) List(key string) []string {
	var raw []string
	switch x := v[key].(type) {
	case []any:
		for _, e := range x {
			raw = append(raw, fmt.Sprint(e))
		}
	case string:
		raw = strings.FieldsFunc(x, func(r rune) bool { return r == '\n' || r == ',' })
	}
	out := raw[:0]
	for _, s := range raw {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// Normalize applies defaults, checks required fields and kinds, and returns a clean copy
// containing only known keys.
func (s Schema) Normalize(in Values) (Values, error) {
	out := Values{}
	for _, f := range s {
		raw, ok := in[f.Key]
		if !ok || raw == nil || raw == "" {
			if f.Default != nil {
				out[f.Key] = f.Default
			} else if f.Required {
				return nil, fmt.Errorf("%s is required", f.Label)
			}
			continue
		}
		switch f.Kind {
		case "int", "number":
			n := Values{"v": raw}.Float("v", nanSentinel)
			if n == nanSentinel {
				return nil, fmt.Errorf("%s must be a number", f.Label)
			}
			if f.Kind == "int" {
				out[f.Key] = float64(int(n))
			} else {
				out[f.Key] = n
			}
		case "bool":
			out[f.Key] = Values{"v": raw}.Bool("v")
		case "list":
			items := Values{"v": raw}.List("v")
			if f.Required && len(items) == 0 {
				return nil, fmt.Errorf("%s is required", f.Label)
			}
			list := make([]any, len(items))
			for i, it := range items {
				list[i] = it
			}
			out[f.Key] = list
		case "select":
			str := fmt.Sprint(raw)
			valid := false
			for _, o := range f.Options {
				valid = valid || o == str
			}
			if !valid {
				return nil, fmt.Errorf("%s must be one of %s", f.Label, strings.Join(f.Options, ", "))
			}
			out[f.Key] = str
		case "url":
			str := strings.TrimRight(strings.TrimSpace(fmt.Sprint(raw)), "/")
			u, err := url.Parse(str)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
				return nil, fmt.Errorf("%s must be an http(s) URL", f.Label)
			}
			out[f.Key] = str
		default: // string, secret, text
			out[f.Key] = fmt.Sprint(raw)
		}
	}
	return out, nil
}

const nanSentinel = -1e308

// MaskSecrets returns a copy with secret values removed, plus the keys of secrets that are set.
func (s Schema) MaskSecrets(v Values) (Values, []string) {
	out := Values{}
	var set []string
	for k, val := range v {
		out[k] = val
	}
	for _, f := range s {
		if f.Kind != "secret" {
			continue
		}
		if v.String(f.Key) != "" {
			set = append(set, f.Key)
		}
		delete(out, f.Key)
	}
	return out, set
}

// KeepSecrets fills secrets left empty in an update with their stored values, so the dashboard
// never needs to see them to save other changes.
func (s Schema) KeepSecrets(update, stored Values) Values {
	out := Values{}
	for k, val := range update {
		out[k] = val
	}
	for _, f := range s {
		if f.Kind == "secret" && Values(out).String(f.Key) == "" && stored.String(f.Key) != "" {
			out[f.Key] = stored[f.Key]
		}
	}
	return out
}
