// Package i18n holds the words cnsl renders on screens — weekday and month names, relative times,
// weather conditions and the built-in components' labels — in each supported language. It never
// looks at the OS locale: a screen's language comes from the settings.
package i18n

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Default is the language used when none is set (or an unknown one is).
const Default = "en"

type Lang struct {
	Code string
	Name string // in the language itself

	Weekdays      [7]string // Sunday first, like time.Weekday
	WeekdaysShort [7]string // for the "Mon" layout token
	WeekdaysMin   [7]string // two letters, calendar headers
	Months        [12]string
	// MonthsGenitive is used when the layout also has a day number ("2 January"); languages
	// without cases repeat Months.
	MonthsGenitive [12]string
	MonthsShort    [12]string
	// Capitalize names at the start of the output (Polish names are lowercase otherwise).
	Capitalize bool

	Ago, Until string                          // "%s ago", "in %s"
	Units      func(n int, unit string) string // n + unit (s, min, h, days) with plural rules

	conditions [12]string // see conditionGroup
	words      map[string]string
}

var langs = map[string]*Lang{"en": &english, "pl": &polish}

// Languages lists the supported languages, English first.
func Languages() []*Lang { return []*Lang{&english, &polish} }

// Get returns a language by code, falling back to English.
func Get(code string) *Lang {
	if l, ok := langs[code]; ok {
		return l
	}
	return &english
}

func Valid(code string) bool { _, ok := langs[code]; return ok }

// T looks up a word of the built-in vocabulary; unknown keys come back unchanged, so templates can
// pass their own text through it too. Extra args fill printf verbs.
func (l *Lang) T(key string, args ...any) string {
	s, ok := l.words[key]
	if !ok {
		if s, ok = english.words[key]; !ok {
			s = key
		}
	}
	if len(args) > 0 {
		return fmt.Sprintf(s, args...)
	}
	return s
}

// Condition names a WMO weather code (as used by Open-Meteo).
func (l *Lang) Condition(code int) string {
	g := conditionGroup(code)
	if g < 0 {
		return fmt.Sprintf("%s %d", l.T("code"), code)
	}
	return l.conditions[g]
}

func conditionGroup(code int) int {
	switch {
	case code == 0:
		return 0
	case code == 1:
		return 1
	case code == 2:
		return 2
	case code == 3:
		return 3
	case code == 45 || code == 48:
		return 4
	case code >= 51 && code <= 57:
		return 5
	case code >= 61 && code <= 67:
		return 6
	case code >= 71 && code <= 77:
		return 7
	case code >= 80 && code <= 82:
		return 8
	case code == 85 || code == 86:
		return 9
	case code >= 95:
		return 10
	}
	return -1
}

// Relative formats a duration like "5 min" in the language.
func (l *Lang) Relative(d time.Duration) string {
	d = d.Abs()
	switch {
	case d < time.Minute:
		return l.Units(int(d.Seconds()), "s")
	case d < time.Hour:
		return l.Units(int(d.Minutes()), "min")
	case d < 48*time.Hour:
		return l.Units(int(d.Hours()), "h")
	default:
		return l.Units(int(d.Hours()/24), "days")
	}
}

// Format is time.Format with the language's weekday and month names. Name tokens (Monday, Mon,
// January, Jan) are replaced; everything else goes through time.Format unchanged.
func (l *Lang) Format(t time.Time, layout string) string {
	if l == &english {
		return t.Format(layout)
	}
	genitive := hasDay(layout)
	var b strings.Builder
	rest := layout
	for rest != "" {
		i, token := nextName(rest)
		if i < 0 {
			b.WriteString(t.Format(rest))
			break
		}
		if i > 0 {
			b.WriteString(t.Format(rest[:i]))
		}
		name := ""
		switch token {
		case "Monday":
			name = l.Weekdays[t.Weekday()]
		case "Mon":
			name = l.WeekdaysShort[t.Weekday()]
		case "January":
			if genitive {
				name = l.MonthsGenitive[t.Month()-1]
			} else {
				name = l.Months[t.Month()-1]
			}
		case "Jan":
			name = l.MonthsShort[t.Month()-1]
		}
		if l.Capitalize && b.Len() == 0 {
			name = capitalize(name)
		}
		b.WriteString(name)
		rest = rest[i+len(token):]
	}
	return b.String()
}

// nextName finds the first name token, preferring the long forms like time.Format does.
func nextName(s string) (int, string) {
	for i := 0; i < len(s); i++ {
		switch {
		case strings.HasPrefix(s[i:], "January"):
			return i, "January"
		case strings.HasPrefix(s[i:], "Jan"):
			return i, "Jan"
		case strings.HasPrefix(s[i:], "Monday"):
			return i, "Monday"
		case strings.HasPrefix(s[i:], "Mon"):
			return i, "Mon"
		}
	}
	return -1, ""
}

var (
	notDay = strings.NewReplacer("2006", "", "002", "")
	dayTok = regexp.MustCompile(`(^|[^0-9])(_2|02|2)([^0-9]|$)`)
)

// hasDay reports whether a layout prints the day of the month (so month names take the genitive).
func hasDay(layout string) bool {
	return dayTok.MatchString(notDay.Replace(layout))
}

func capitalize(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError {
		return s
	}
	return string(unicode.ToUpper(r)) + s[n:]
}
