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
	"calendar": calendar,

	// Numbers
	"add": func(a, b any) float64 { return toFloat(a) + toFloat(b) },
	"sub": func(a, b any) float64 { return toFloat(a) - toFloat(b) },
	"mul": func(a, b any) float64 { return toFloat(a) * toFloat(b) },
	"div": divide,
	"round": func(places, v any) float64 {
		p := math.Pow(10, float64(toInt(places)))
		return math.Round(toFloat(v)*p) / p
	},
	"num": func(places, v any) string {
		return fmt.Sprintf("%.*f", max(toInt(places), 0), toFloat(v))
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
	"take": func(n any, items any) any {
		v := reflect.ValueOf(items)
		if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
			return items
		}
		return v.Slice(0, max(min(toInt(n), v.Len()), 0)).Interface()
	},
	"seq": func(n any) []int {
		out := make([]int, max(toInt(n), 0))
		for i := range out {
			out[i] = i
		}
		return out
	},
	"safeHTML": func(s string) template.HTML { return template.HTML(s) },
	// Replaced per render (see componentCaller); listed here so templates parse and editors complete it.
	"component": func(string, ...any) (template.HTML, error) { return "", nil },
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

// toInt accepts numbers of any type, so inputs from JSON (float64) work where counts are expected.
func toInt(v any) int { return int(math.Round(toFloat(v))) }

// calendar returns the weeks of t's month, Monday first; days outside the month are 0.
func calendar(t time.Time) [][]int {
	first := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
	days := first.AddDate(0, 1, -1).Day()
	offset := (int(first.Weekday()) + 6) % 7
	var weeks [][]int
	week := make([]int, 7)
	for i := 0; i < offset+days; i++ {
		if i >= offset {
			week[i%7] = i - offset + 1
		}
		if i%7 == 6 {
			weeks = append(weeks, week)
			week = make([]int, 7)
		}
	}
	if (offset+days)%7 != 0 {
		weeks = append(weeks, week)
	}
	return weeks
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
