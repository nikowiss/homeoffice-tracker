package store_test

import (
	"path/filepath"
	"slices"
	"testing"

	"homeoffice-tracker/internal/civil"
	"homeoffice-tracker/internal/store"
)

func openStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "homeoffice.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func date(t *testing.T, s string) civil.Date {
	t.Helper()
	d, err := civil.ParseDate(s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return d
}

func TestMarkedDayCountsInItsYear(t *testing.T) {
	s := openStore(t)

	if err := s.Mark(t.Context(), date(t, "2026-10-14")); err != nil {
		t.Fatalf("mark: %v", err)
	}

	counts, err := s.YearCounts(t.Context())
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
	if err := s.Mark(t.Context(), day); err != nil {
		t.Fatalf("mark: %v", err)
	}

	if err := s.Unmark(t.Context(), day); err != nil {
		t.Fatalf("unmark: %v", err)
	}

	counts, err := s.YearCounts(t.Context())
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
		if err := s.Mark(t.Context(), day); err != nil {
			t.Fatalf("mark: %v", err)
		}
	}

	counts, err := s.YearCounts(t.Context())
	if err != nil {
		t.Fatalf("year counts: %v", err)
	}
	if got := counts[2026]; got != 1 {
		t.Errorf("year count 2026 = %d, want 1", got)
	}
}

func TestUnmarkingAnUnmarkedDayDoesNothing(t *testing.T) {
	s := openStore(t)
	if err := s.Mark(t.Context(), date(t, "2026-10-13")); err != nil {
		t.Fatalf("mark: %v", err)
	}

	if err := s.Unmark(t.Context(), date(t, "2026-10-14")); err != nil {
		t.Fatalf("unmark: %v", err)
	}

	counts, err := s.YearCounts(t.Context())
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
		if err := s.Mark(t.Context(), date(t, d)); err != nil {
			t.Fatalf("mark %s: %v", d, err)
		}
	}

	counts, err := s.YearCounts(t.Context())
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
		if err := s.Mark(t.Context(), date(t, d)); err != nil {
			t.Fatalf("mark %s: %v", d, err)
		}
	}

	days, err := s.Days(t.Context(), 2026)
	if err != nil {
		t.Fatalf("days: %v", err)
	}

	var got []string
	for _, d := range days {
		got = append(got, d.String())
	}
	want := []string{"2026-01-01", "2026-03-02"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("days = %v, want %v", got, want)
	}
}

func TestApplyMarksAndUnmarksTogether(t *testing.T) {
	s := openStore(t)
	for _, d := range []string{"2025-12-30", "2026-10-01"} {
		if err := s.Mark(t.Context(), date(t, d)); err != nil {
			t.Fatalf("mark %s: %v", d, err)
		}
	}

	err := s.Apply(t.Context(),
		[]civil.Date{date(t, "2025-12-31"), date(t, "2026-10-06"), date(t, "2026-10-01")},
		[]civil.Date{date(t, "2025-12-30"), date(t, "2026-10-02")},
	)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	counts, err := s.YearCounts(t.Context())
	if err != nil {
		t.Fatalf("year counts: %v", err)
	}
	if counts[2025] != 1 || counts[2026] != 2 {
		t.Errorf("year counts = %v, want map[2025:1 2026:2]", counts)
	}
	days, err := s.Days(t.Context(), 2025)
	if err != nil {
		t.Fatalf("days: %v", err)
	}
	if len(days) != 1 || days[0].String() != "2025-12-31" {
		t.Errorf("days 2025 = %v, want [2025-12-31]", days)
	}
}

func assertSubscriptions(t *testing.T, s *store.Store, want ...store.Subscription) {
	t.Helper()
	got, err := s.Subscriptions(t.Context())
	if err != nil {
		t.Fatalf("subscriptions: %v", err)
	}
	if !slices.Equal(got, want) {
		t.Errorf("subscriptions = %+v, want %+v", got, want)
	}
}

func TestSavingASubscriptionAgainReplacesItsKeys(t *testing.T) {
	s := openStore(t)
	old := store.Subscription{Endpoint: "https://push.example/a", P256dh: "key-1", Auth: "auth-1"}
	renewed := store.Subscription{Endpoint: "https://push.example/a", P256dh: "key-2", Auth: "auth-2"}

	for _, sub := range []store.Subscription{old, renewed} {
		if err := s.SaveSubscription(t.Context(), sub); err != nil {
			t.Fatalf("save subscription: %v", err)
		}
	}

	assertSubscriptions(t, s, renewed)
}

func TestDeletedSubscriptionIsGone(t *testing.T) {
	s := openStore(t)
	a := store.Subscription{Endpoint: "https://push.example/a", P256dh: "key-a", Auth: "auth-a"}
	b := store.Subscription{Endpoint: "https://push.example/b", P256dh: "key-b", Auth: "auth-b"}
	for _, sub := range []store.Subscription{a, b} {
		if err := s.SaveSubscription(t.Context(), sub); err != nil {
			t.Fatalf("save subscription: %v", err)
		}
	}

	if err := s.DeleteSubscription(t.Context(), a.Endpoint); err != nil {
		t.Fatalf("delete subscription: %v", err)
	}
	if err := s.DeleteSubscription(t.Context(), "https://push.example/unknown"); err != nil {
		t.Fatalf("delete unknown subscription: %v", err)
	}

	assertSubscriptions(t, s, b)
}

func TestVAPIDKeysStayAfterReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "homeoffice.db")
	keysFrom := func(private, public string) func() (string, string, error) {
		return func() (string, string, error) { return private, public, nil }
	}

	first, err := store.Open(t.Context(), path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	created, err := first.VAPIDKeys(t.Context(), keysFrom("private-1", "public-1"))
	if err != nil {
		t.Fatalf("vapid keys: %v", err)
	}
	first.Close()

	second, err := store.Open(t.Context(), path)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer second.Close()
	loaded, err := second.VAPIDKeys(t.Context(), keysFrom("private-2", "public-2"))
	if err != nil {
		t.Fatalf("vapid keys after reopen: %v", err)
	}

	want := store.VAPIDKeys{Private: "private-1", Public: "public-1"}
	if created != want || loaded != want {
		t.Errorf("keys = %+v then %+v, want %+v both times", created, loaded, want)
	}
}

func TestLastReminderDate(t *testing.T) {
	s := openStore(t)

	initial, err := s.LastReminderDate(t.Context())
	if err != nil {
		t.Fatalf("last reminder date: %v", err)
	}
	if initial != (civil.Date{}) {
		t.Errorf("initial last reminder date = %v, want zero date", initial)
	}

	if err := s.SetLastReminderDate(t.Context(), date(t, "2026-10-14")); err != nil {
		t.Fatalf("set last reminder date: %v", err)
	}
	if err := s.SetLastReminderDate(t.Context(), date(t, "2026-10-15")); err != nil {
		t.Fatalf("set last reminder date again: %v", err)
	}

	got, err := s.LastReminderDate(t.Context())
	if err != nil {
		t.Fatalf("last reminder date: %v", err)
	}
	if want := date(t, "2026-10-15"); got != want {
		t.Errorf("last reminder date = %v, want %v", got, want)
	}
}

func TestMarkedReportsOneDay(t *testing.T) {
	s := openStore(t)
	if err := s.Mark(t.Context(), date(t, "2026-10-15")); err != nil {
		t.Fatalf("mark: %v", err)
	}

	for day, want := range map[string]bool{"2026-10-15": true, "2026-10-14": false} {
		got, err := s.Marked(t.Context(), date(t, day))
		if err != nil {
			t.Fatalf("marked %s: %v", day, err)
		}
		if got != want {
			t.Errorf("marked %s = %v, want %v", day, got, want)
		}
	}
}
