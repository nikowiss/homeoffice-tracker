package reminder_test

import (
	"errors"
	"net/http"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"homeoffice-tracker/internal/reminder"
	"homeoffice-tracker/internal/store"
)

type sentPush struct {
	Endpoint string
	Payload  string
	TTL      int
}

type fakeSender struct {
	statuses map[string]int
	err      error
	sent     []sentPush
}

func (f *fakeSender) Send(sub store.Subscription, payload []byte, ttl int) (int, error) {
	f.sent = append(f.sent, sentPush{Endpoint: sub.Endpoint, Payload: string(payload), TTL: ttl})
	if f.err != nil {
		return 0, f.err
	}
	if status, ok := f.statuses[sub.Endpoint]; ok {
		return status, nil
	}
	return http.StatusCreated, nil
}

func (f *fakeSender) endpoints() []string {
	var endpoints []string
	for _, push := range f.sent {
		endpoints = append(endpoints, push.Endpoint)
	}
	return endpoints
}

func openStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "homeoffice.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func subscribe(t *testing.T, s *store.Store, endpoints ...string) {
	t.Helper()
	for _, endpoint := range endpoints {
		if err := s.SaveSubscription(store.Subscription{Endpoint: endpoint, P256dh: "key", Auth: "auth"}); err != nil {
			t.Fatalf("save subscription: %v", err)
		}
	}
}

func newService(t *testing.T, s *store.Store, sender reminder.Sender, now string) *reminder.Service {
	t.Helper()
	at := berlinTime(t, now)
	return reminder.New(s, sender, reminder.TimeOfDay{Hour: 9}, func() time.Time { return at })
}

func check(t *testing.T, service *reminder.Service) {
	t.Helper()
	if err := service.Check(); err != nil {
		t.Fatalf("check: %v", err)
	}
}

func assertSent(t *testing.T, sender *fakeSender, want ...string) {
	t.Helper()
	if got := sender.endpoints(); !slices.Equal(got, want) {
		t.Errorf("pushes sent to %v, want %v", got, want)
	}
}

const reminderPayload = `{"title":"Home office today?","body":"Open the tracker to mark today."}`

func TestDueReminderGoesToEverySubscription(t *testing.T) {
	s := openStore(t)
	subscribe(t, s, "https://push.example/a", "https://push.example/b")
	sender := &fakeSender{}

	check(t, newService(t, s, sender, "2026-10-15 09:00"))

	want := []sentPush{
		{Endpoint: "https://push.example/a", Payload: reminderPayload, TTL: 15 * 3600},
		{Endpoint: "https://push.example/b", Payload: reminderPayload, TTL: 15 * 3600},
	}
	if !slices.Equal(sender.sent, want) {
		t.Errorf("sent = %+v, want %+v", sender.sent, want)
	}
}

func TestReminderIsSentOncePerDay(t *testing.T) {
	s := openStore(t)
	subscribe(t, s, "https://push.example/a")
	sender := &fakeSender{}

	check(t, newService(t, s, sender, "2026-10-15 09:00"))
	check(t, newService(t, s, sender, "2026-10-15 09:01"))

	assertSent(t, sender, "https://push.example/a")
}

func TestReminderComesAgainTheNextWeekday(t *testing.T) {
	s := openStore(t)
	subscribe(t, s, "https://push.example/a")
	sender := &fakeSender{}

	check(t, newService(t, s, sender, "2026-10-15 09:00"))
	check(t, newService(t, s, sender, "2026-10-16 09:00"))

	assertSent(t, sender, "https://push.example/a", "https://push.example/a")
}

func TestNoReminderWhenTodayIsMarked(t *testing.T) {
	s := openStore(t)
	subscribe(t, s, "https://push.example/a")
	if err := s.Mark(date(t, "2026-10-15")); err != nil {
		t.Fatalf("mark: %v", err)
	}
	sender := &fakeSender{}

	check(t, newService(t, s, sender, "2026-10-15 09:00"))

	assertSent(t, sender)
}

func TestReminderIsRetriedWhenNoPushWasAccepted(t *testing.T) {
	s := openStore(t)
	subscribe(t, s, "https://push.example/a")
	offline := &fakeSender{err: errors.New("network is unreachable")}
	online := &fakeSender{}

	check(t, newService(t, s, offline, "2026-10-15 09:00"))
	check(t, newService(t, s, online, "2026-10-15 09:01"))

	assertSent(t, online, "https://push.example/a")
}

func TestReminderComesAfterTheFirstSubscription(t *testing.T) {
	s := openStore(t)
	sender := &fakeSender{}

	check(t, newService(t, s, sender, "2026-10-15 09:00"))
	subscribe(t, s, "https://push.example/a")
	check(t, newService(t, s, sender, "2026-10-15 14:00"))

	assertSent(t, sender, "https://push.example/a")
}

func TestGoneSubscriptionsAreDeleted(t *testing.T) {
	s := openStore(t)
	subscribe(t, s, "https://push.example/gone", "https://push.example/not-found", "https://push.example/broken", "https://push.example/ok")
	sender := &fakeSender{statuses: map[string]int{
		"https://push.example/gone":      http.StatusGone,
		"https://push.example/not-found": http.StatusNotFound,
		"https://push.example/broken":    http.StatusInternalServerError,
	}}

	check(t, newService(t, s, sender, "2026-10-15 09:00"))

	subs, err := s.Subscriptions()
	if err != nil {
		t.Fatalf("subscriptions: %v", err)
	}
	var endpoints []string
	for _, sub := range subs {
		endpoints = append(endpoints, sub.Endpoint)
	}
	if want := []string{"https://push.example/broken", "https://push.example/ok"}; !slices.Equal(endpoints, want) {
		t.Errorf("subscriptions left = %v, want %v", endpoints, want)
	}
}

func TestSendNowIgnoresTheRulesAndDoesNotCountAsTodaysReminder(t *testing.T) {
	s := openStore(t)
	subscribe(t, s, "https://push.example/a")
	sender := &fakeSender{}

	if err := newService(t, s, sender, "2026-10-15 08:00").SendNow(); err != nil {
		t.Fatalf("send now: %v", err)
	}
	check(t, newService(t, s, sender, "2026-10-15 09:00"))

	assertSent(t, sender, "https://push.example/a", "https://push.example/a")
}
