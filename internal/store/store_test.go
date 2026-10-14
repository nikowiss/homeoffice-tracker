package store_test

import (
	"path/filepath"
	"testing"
	"time"

	"homeoffice-tracker/internal/store"
)

func openStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "homeoffice.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func date(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return d
}

func TestMarkedDayCountsInItsYear(t *testing.T) {
	s := openStore(t)

	if err := s.Mark(date(t, "2026-10-14")); err != nil {
		t.Fatalf("mark: %v", err)
	}

	counts, err := s.YearCounts()
	if err != nil {
		t.Fatalf("year counts: %v", err)
	}
	if got := counts[2026]; got != 1 {
		t.Errorf("year count 2026 = %d, want 1", got)
	}
}

func TestUnmarkedDayNoLongerCounts(t *testing.T) {
	s := openStore(t)
	day := date(t, "2026-10-14")
	if err := s.Mark(day); err != nil {
		t.Fatalf("mark: %v", err)
	}

	if err := s.Unmark(day); err != nil {
		t.Fatalf("unmark: %v", err)
	}

	counts, err := s.YearCounts()
	if err != nil {
		t.Fatalf("year counts: %v", err)
	}
	if got := counts[2026]; got != 0 {
		t.Errorf("year count 2026 = %d, want 0", got)
	}
}

func TestMarkingTwiceCountsOnce(t *testing.T) {
	s := openStore(t)
	day := date(t, "2026-10-14")

	for range 2 {
		if err := s.Mark(day); err != nil {
			t.Fatalf("mark: %v", err)
		}
	}

	counts, err := s.YearCounts()
	if err != nil {
		t.Fatalf("year counts: %v", err)
	}
	if got := counts[2026]; got != 1 {
		t.Errorf("year count 2026 = %d, want 1", got)
	}
}

func TestUnmarkingAnUnmarkedDayDoesNothing(t *testing.T) {
	s := openStore(t)
	if err := s.Mark(date(t, "2026-10-13")); err != nil {
		t.Fatalf("mark: %v", err)
	}

	if err := s.Unmark(date(t, "2026-10-14")); err != nil {
		t.Fatalf("unmark: %v", err)
	}

	counts, err := s.YearCounts()
	if err != nil {
		t.Fatalf("year counts: %v", err)
	}
	if got := counts[2026]; got != 1 {
		t.Errorf("year count 2026 = %d, want 1", got)
	}
}

func TestYearCountsSplitAtTheYearBoundary(t *testing.T) {
	s := openStore(t)
	for _, d := range []string{"2025-12-31", "2026-01-01", "2026-01-02"} {
		if err := s.Mark(date(t, d)); err != nil {
			t.Fatalf("mark %s: %v", d, err)
		}
	}

	counts, err := s.YearCounts()
	if err != nil {
		t.Fatalf("year counts: %v", err)
	}
	want := map[int]int{2025: 1, 2026: 2}
	if len(counts) != len(want) || counts[2025] != 1 || counts[2026] != 2 {
		t.Errorf("year counts = %v, want %v", counts, want)
	}
}

func TestDaysListsOneYearOldestFirst(t *testing.T) {
	s := openStore(t)
	for _, d := range []string{"2026-03-02", "2025-12-31", "2026-01-01", "2027-01-01"} {
		if err := s.Mark(date(t, d)); err != nil {
			t.Fatalf("mark %s: %v", d, err)
		}
	}

	days, err := s.Days(2026)
	if err != nil {
		t.Fatalf("days: %v", err)
	}

	var got []string
	for _, d := range days {
		got = append(got, d.Format("2006-01-02"))
	}
	want := []string{"2026-01-01", "2026-03-02"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("days = %v, want %v", got, want)
	}
}
