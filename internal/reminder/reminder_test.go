package reminder_test

import (
	"testing"
	"time"

	"homeoffice-tracker/internal/civil"
	"homeoffice-tracker/internal/reminder"
)

func TestParseTimeOfDay(t *testing.T) {
	valid := map[string]reminder.TimeOfDay{
		"09:00": {Hour: 9, Minute: 0},
		"08:30": {Hour: 8, Minute: 30},
		"00:00": {Hour: 0, Minute: 0},
		"23:59": {Hour: 23, Minute: 59},
	}
	for raw, want := range valid {
		got, err := reminder.ParseTimeOfDay(raw)
		if err != nil {
			t.Errorf("ParseTimeOfDay(%q): %v", raw, err)
			continue
		}
		if got != want {
			t.Errorf("ParseTimeOfDay(%q) = %+v, want %+v", raw, got, want)
		}
	}

	for _, raw := range []string{"", "9:00", "24:00", "09:60", "abc", "09:00:00", " 09:00"} {
		if _, err := reminder.ParseTimeOfDay(raw); err == nil {
			t.Errorf("ParseTimeOfDay(%q) gives no error", raw)
		}
	}
}

func berlinTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.ParseInLocation("2006-01-02 15:04", value, civil.Berlin)
	if err != nil {
		t.Fatalf("parse %q: %v", value, err)
	}
	return parsed
}

func date(t *testing.T, s string) civil.Date {
	t.Helper()
	d, err := civil.ParseDate(s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return d
}

func TestDue(t *testing.T) {
	at := reminder.TimeOfDay{Hour: 9}
	never := civil.Date{}
	tests := []struct {
		name        string
		now         string
		todayMarked bool
		last        civil.Date
		want        bool
	}{
		{"weekday before the time", "2026-10-15 08:59", false, never, false},
		{"weekday at the time", "2026-10-15 09:00", false, never, true},
		{"weekday after the time", "2026-10-15 17:30", false, never, true},
		{"Saturday", "2026-10-17 09:00", false, never, false},
		{"Sunday", "2026-10-18 09:00", false, never, false},
		{"today marked", "2026-10-15 09:00", true, never, false},
		{"already sent today", "2026-10-15 09:00", false, date(t, "2026-10-15"), false},
		{"last sent yesterday", "2026-10-15 09:00", false, date(t, "2026-10-14"), true},
		{"Friday", "2026-10-16 09:00", false, never, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := reminder.Due(berlinTime(t, tt.now), at, tt.todayMarked, tt.last); got != tt.want {
				t.Errorf("Due = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDueUsesBerlinTime(t *testing.T) {
	at := reminder.TimeOfDay{Hour: 9}
	sevenUTC := time.Date(2026, time.October, 15, 7, 0, 0, 0, time.UTC)

	if !reminder.Due(sevenUTC, at, false, civil.Date{}) {
		t.Errorf("Due at 07:00 UTC (09:00 in Berlin) = false, want true")
	}
	if reminder.Due(sevenUTC.Add(-time.Minute), at, false, civil.Date{}) {
		t.Errorf("Due at 06:59 UTC (08:59 in Berlin) = true, want false")
	}
}

func TestSecondsUntilMidnight(t *testing.T) {
	tests := []struct {
		name string
		now  string
		want int
	}{
		{"normal day", "2026-10-15 09:00", 15 * 3600},
		{"one minute before midnight", "2026-10-15 23:59", 60},
		{"daylight saving time starts (23-hour day)", "2026-03-29 01:00", 22 * 3600},
		{"daylight saving time ends (25-hour day)", "2026-10-25 01:00", 24 * 3600},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := reminder.SecondsUntilMidnight(berlinTime(t, tt.now)); got != tt.want {
				t.Errorf("SecondsUntilMidnight = %d, want %d", got, tt.want)
			}
		})
	}
}
