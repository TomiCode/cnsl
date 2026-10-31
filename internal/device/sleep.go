package device

import (
	"fmt"
	"time"
)

// refreshRate returns how long the device should sleep: its refresh rate, or until the end of
// its sleep window when now falls inside it. now must be in the household timezone.
func refreshRate(now time.Time, rate int, sleepStart, sleepEnd string) int {
	start, okStart := parseClock(sleepStart)
	end, okEnd := parseClock(sleepEnd)
	if !okStart || !okEnd || start == end {
		return rate
	}

	minute := now.Hour()*60 + now.Minute()
	inWindow := minute >= start && minute < end
	if start > end { // window crosses midnight, e.g. 22:00–06:00
		inWindow = minute >= start || minute < end
	}
	if !inWindow {
		return rate
	}

	wake := time.Date(now.Year(), now.Month(), now.Day(), end/60, end%60, 0, 0, now.Location())
	if !wake.After(now) {
		wake = wake.AddDate(0, 0, 1)
	}
	return max(rate, int(wake.Sub(now).Seconds()))
}

// parseClock parses "HH:MM" into minutes after midnight.
func parseClock(s string) (int, bool) {
	var h, m int
	if n, err := fmt.Sscanf(s, "%d:%d", &h, &m); err != nil || n != 2 || len(s) != 5 {
		return 0, false
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}
