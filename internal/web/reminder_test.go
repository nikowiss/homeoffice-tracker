package web_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"homeoffice-tracker/internal/reminder"
	"homeoffice-tracker/internal/store"
	"homeoffice-tracker/internal/web"
)

type recordingSender struct {
	endpoints []string
}

func (r *recordingSender) Send(_ context.Context, sub store.Subscription, payload []byte, ttl int) (int, error) {
	r.endpoints = append(r.endpoints, sub.Endpoint)
	return http.StatusCreated, nil
}

func newReminderHandler(t *testing.T) (http.Handler, *recordingSender) {
	t.Helper()
	s, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "homeoffice.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	sender := &recordingSender{}
	now := func() time.Time { return wednesdayMorning }
	service := reminder.New(s, sender, reminder.TimeOfDay{Hour: 9}, now)
	return web.NewHandler(s, now, web.ReminderSetup{PublicKey: "test-public-key", SendNow: service.SendNow}), sender
}

func postJSON(t *testing.T, h http.Handler, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func sendTestReminder(t *testing.T, h http.Handler) {
	t.Helper()
	assertStatus(t, do(t, h, http.MethodPost, "/reminder/test"), http.StatusNoContent)
}

const subscriptionJSON = `{"endpoint":"https://push.example/a","expirationTime":null,"keys":{"p256dh":"key-a","auth":"auth-a"}}`

func TestSubscribedBrowserGetsTheTestReminder(t *testing.T) {
	h, sender := newReminderHandler(t)

	assertStatus(t, postJSON(t, h, "/reminder/subscriptions", subscriptionJSON), http.StatusNoContent)
	sendTestReminder(t, h)

	if want := []string{"https://push.example/a"}; !slices.Equal(sender.endpoints, want) {
		t.Errorf("test reminder sent to %v, want %v", sender.endpoints, want)
	}
}

func TestTurnedOffBrowserGetsNoReminder(t *testing.T) {
	h, sender := newReminderHandler(t)
	assertStatus(t, postJSON(t, h, "/reminder/subscriptions", subscriptionJSON), http.StatusNoContent)

	assertStatus(t, postJSON(t, h, "/reminder/subscriptions/delete", `{"endpoint":"https://push.example/a"}`), http.StatusNoContent)
	assertStatus(t, postJSON(t, h, "/reminder/subscriptions/delete", `{"endpoint":"https://push.example/unknown"}`), http.StatusNoContent)
	sendTestReminder(t, h)

	if len(sender.endpoints) != 0 {
		t.Errorf("test reminder sent to %v, want nobody", sender.endpoints)
	}
}

func TestInvalidSubscriptionIsRejected(t *testing.T) {
	h, _ := newReminderHandler(t)

	for _, body := range []string{
		`not json`,
		`{"keys":{"p256dh":"key-a","auth":"auth-a"}}`,
		`{"endpoint":"https://push.example/a","keys":{"auth":"auth-a"}}`,
		`{"endpoint":"https://push.example/a","keys":{"p256dh":"key-a"}}`,
	} {
		rec := postJSON(t, h, "/reminder/subscriptions", body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("subscribe with %s: status = %d, want %d", body, rec.Code, http.StatusBadRequest)
		}
	}
}

func TestMainPageOffersRemindersOnlyWithJavaScript(t *testing.T) {
	h, _ := newReminderHandler(t)

	body := do(t, h, http.MethodGet, "/").Body.String()

	assertContains(t, body, `data-vapid-public-key="test-public-key"`)
	assertContains(t, body, `<script src="/reminder.js" defer></script>`)
	if !regexp.MustCompile(`<p class="reminder"[^>]*\bhidden\b`).MatchString(body) {
		t.Errorf("reminder line is not hidden by default:\n%s", body)
	}
}

func TestServiceWorkerIsServedFromTheRoot(t *testing.T) {
	h, _ := newReminderHandler(t)

	for _, path := range []string{"/sw.js", "/reminder.js"} {
		rec := do(t, h, http.MethodGet, path)
		assertStatus(t, rec, http.StatusOK)
		if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/javascript") {
			t.Errorf("%s Content-Type = %q, want text/javascript", path, got)
		}
	}
}
