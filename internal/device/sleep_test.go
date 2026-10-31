package device

import (
	"testing"
	"time"
)

func TestRefreshRate(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2026, 10, 31, h, m, 0, 0, time.UTC) }
	cases := []struct {
		name       string
		now        time.Time
		start, end string
		want       int
	}{
		{"no window", at(23, 0), "", "", 900},
		{"outside window", at(12, 0), "22:00", "06:00", 900},
		{"before midnight", at(23, 0), "22:00", "06:00", 7 * 3600},
		{"after midnight", at(5, 30), "22:00", "06:00", 1800},
		{"same-day window", at(13, 0), "12:00", "14:00", 3600},
		{"end of window", at(14, 0), "12:00", "14:00", 900},
		{"short remainder keeps rate", at(5, 59), "22:00", "06:00", 900},
	}
	for _, c := range cases {
		if got := refreshRate(c.now, 900, c.start, c.end); got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, got, c.want)
		}
	}
}

func TestParseClock(t *testing.T) {
	for in, ok := range map[string]bool{"06:30": true, "23:59": true, "24:00": false, "6:30": false, "aa:bb": false, "": false} {
		if _, got := parseClock(in); got != ok {
			t.Errorf("parseClock(%q) ok = %v, want %v", in, got, ok)
		}
	}
}
