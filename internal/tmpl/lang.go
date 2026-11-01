package tmpl

import (
	"fmt"
	"html/template"
	"time"

	"github.com/TomiCode/cnsl/internal/i18n"
	"github.com/TomiCode/cnsl/internal/icons"
)

// langFuncs are the helpers that depend on the screen's language and timezone. They replace the
// English placeholders in funcs for each render.
func langFuncs(l *i18n.Lang, loc *time.Location) template.FuncMap {
	if loc == nil {
		loc = time.Local
	}
	return template.FuncMap{
		"date": func(layout string, v any) (string, error) {
			t, err := toTime(v, loc)
			if err != nil || t.IsZero() {
				return "", err
			}
			return l.Format(t, layout), nil
		},
		"ago": func(v any) (string, error) {
			t, err := toTime(v, loc)
			if err != nil || t.IsZero() {
				return "", err
			}
			return fmt.Sprintf(l.Ago, l.Relative(time.Since(t))), nil
		},
		"until": func(v any) (string, error) {
			t, err := toTime(v, loc)
			if err != nil || t.IsZero() {
				return "", err
			}
			return fmt.Sprintf(l.Until, l.Relative(time.Until(t))), nil
		},
		"t":         l.T,
		"condition": func(code any) string { return l.Condition(toInt(code)) },
		"weekdays": func() []string { // two-letter names, Monday first (matches calendar)
			out := make([]string, 7)
			for i := range out {
				out[i] = l.WeekdaysMin[(i+1)%7]
			}
			return out
		},
	}
}

// timeLayouts are accepted for times given as strings (connector data): RFC 3339 and the
// zone-less forms Open-Meteo and others use, read in the household timezone.
var timeLayouts = []string{time.RFC3339Nano, "2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02 15:04:05", "2006-01-02"}

func toTime(v any, loc *time.Location) (time.Time, error) {
	switch t := v.(type) {
	case time.Time:
		return t.In(loc), nil
	case *time.Time:
		if t == nil {
			return time.Time{}, nil
		}
		return t.In(loc), nil
	case string:
		if t == "" {
			return time.Time{}, nil
		}
		for _, layout := range timeLayouts {
			if parsed, err := time.ParseInLocation(layout, t, loc); err == nil {
				return parsed.In(loc), nil
			}
		}
		return time.Time{}, fmt.Errorf("cannot read %q as a time", t)
	case nil:
		return time.Time{}, nil
	}
	return time.Time{}, fmt.Errorf("cannot use %T as a time", v)
}

// icon renders an icon inline: {{ icon "wi-day-sunny" }}, {{ icon "thermometer" 40 }},
// {{ icon "battery" 40 3 }} (size in CSS pixels, stroke width for outline icons).
func icon(name string, opts ...any) (template.HTML, error) {
	var size, stroke float64
	if len(opts) > 0 {
		size = toFloat(opts[0])
	}
	if len(opts) > 1 {
		stroke = toFloat(opts[1])
	}
	if len(opts) > 2 {
		return "", fmt.Errorf("icon takes a name, size and stroke width")
	}
	return icons.SVG(name, size, stroke)
}
