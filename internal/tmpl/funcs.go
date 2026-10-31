package tmpl

import (
	"errors"
	"fmt"
	"html/template"
	"math"
	"reflect"
	"strings"
	"time"
)

// funcs is the helper set available in every template. Keep it small and documented: the
// editor lists these names for completion.
var funcs = template.FuncMap{
	// Time
	"date": func(layout string, t time.Time) string { return t.Format(layout) },
	"ago":  func(t time.Time) string { return humanDuration(time.Since(t)) + " ago" },
	"until": func(t time.Time) string {
		return "in " + humanDuration(time.Until(t))
	},

	// Numbers
	"add": func(a, b any) float64 { return toFloat(a) + toFloat(b) },
	"sub": func(a, b any) float64 { return toFloat(a) - toFloat(b) },
	"mul": func(a, b any) float64 { return toFloat(a) * toFloat(b) },
	"div": divide,
	"round": func(places int, v any) float64 {
		p := math.Pow(10, float64(places))
		return math.Round(toFloat(v)*p) / p
	},
	"num": func(places int, v any) string {
		return fmt.Sprintf("%.*f", places, toFloat(v))
	},

	// Values
	"default": func(fallback, v any) any {
		if isEmpty(v) {
			return fallback
		}
		return v
	},
	"upper": strings.ToUpper,
	"lower": strings.ToLower,
	"trim":  strings.TrimSpace,
	"join":  func(sep string, items []string) string { return strings.Join(items, sep) },
	"list":  func(items ...any) []any { return items },
	"dict":  dict,
	"take": func(n int, items any) any {
		v := reflect.ValueOf(items)
		if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
			return items
		}
		return v.Slice(0, min(n, v.Len())).Interface()
	},
	"seq": func(n int) []int {
		out := make([]int, max(n, 0))
		for i := range out {
			out[i] = i
		}
		return out
	},
	"safeHTML": func(s string) template.HTML { return template.HTML(s) },
}

// FuncNames lists the helpers for editor completion.
func FuncNames() []string {
	names := make([]string, 0, len(funcs))
	for name := range funcs {
		names = append(names, name)
	}
	return names
}

func divide(a, b any) (float64, error) {
	d := toFloat(b)
	if d == 0 {
		return 0, errors.New("division by zero")
	}
	return toFloat(a) / d, nil
}

func dict(pairs ...any) (map[string]any, error) {
	if len(pairs)%2 != 0 {
		return nil, errors.New("dict needs key/value pairs")
	}
	m := make(map[string]any, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		k, ok := pairs[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict key %v is not a string", pairs[i])
		}
		m[k] = pairs[i+1]
	}
	return m, nil
}

func toFloat(v any) float64 {
	switch n := v.(type) {
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case float64:
		return n
	case float32:
		return float64(n)
	case string:
		var f float64
		fmt.Sscan(n, &f)
		return f
	default:
		rv := reflect.ValueOf(v)
		switch rv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return float64(rv.Int())
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return float64(rv.Uint())
		case reflect.Float32, reflect.Float64:
			return rv.Float()
		}
		return 0
	}
}

func isEmpty(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.String, reflect.Slice, reflect.Map, reflect.Array:
		return rv.Len() == 0
	case reflect.Pointer, reflect.Interface:
		return rv.IsNil()
	}
	return rv.IsZero()
}

func humanDuration(d time.Duration) string {
	d = d.Abs()
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%d s", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%d min", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%d h", int(d.Hours()))
	default:
		return fmt.Sprintf("%d days", int(d.Hours()/24))
	}
}
