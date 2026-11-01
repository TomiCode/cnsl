package i18n

import (
	"testing"
	"time"
)

func TestFormat(t *testing.T) {
	sun := time.Date(2026, 11, 1, 9, 5, 0, 0, time.UTC)
	pl := Get("pl")
	cases := map[string]string{
		"Monday, 2 January": "Niedziela, 1 listopada",
		"January 2006":      "Listopad 2026",
		"Mon 2 Jan":         "Niedz. 1 lis",
		"2 January 2006":    "1 listopada 2026",
		"15:04, Monday":     "09:05, niedziela",
		"2006-01-02":        "2026-11-01",
		"02.01.2006 Monday": "01.11.2026 niedziela",
		"January":           "Listopad",
	}
	for layout, want := range cases {
		if got := pl.Format(sun, layout); got != want {
			t.Errorf("pl %q = %q, want %q", layout, got, want)
		}
	}
	if got := Get("en").Format(sun, "Monday, 2 January"); got != "Sunday, 1 November" {
		t.Errorf("en = %q", got)
	}
	if Get("xx") != Get("en") {
		t.Error("unknown languages should fall back to English")
	}
}

func TestWords(t *testing.T) {
	pl := Get("pl")
	if got := pl.T("n_of_m_up", 2, 3); got != "2 z 3 działa" {
		t.Errorf("T = %q", got)
	}
	if got := pl.T("my own text"); got != "my own text" {
		t.Errorf("unknown key = %q", got)
	}
	if got := pl.Condition(61); got != "Deszcz" {
		t.Errorf("Condition = %q", got)
	}
	if got := pl.Condition(5); got != "Kod 5" {
		t.Errorf("unknown code = %q", got)
	}
	if got := pl.Relative(26 * time.Hour); got != "26 godz." {
		t.Errorf("Relative = %q", got)
	}
	if got := pl.Relative(72 * time.Hour); got != "3 dni" {
		t.Errorf("Relative = %q", got)
	}
}
