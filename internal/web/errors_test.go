package web_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"homeoffice-tracker/internal/store"
	"homeoffice-tracker/internal/web"
)

func TestServerErrorsAreLoggedNotShown(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	s, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "homeoffice.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	s.Close()
	sendNow := func(context.Context) error { return errors.New("push service exploded") }
	h := web.NewHandler(s, func() time.Time { return wednesdayMorning }, web.ReminderSetup{PublicKey: "test-public-key", SendNow: sendNow})

	tests := []struct {
		name    string
		request func() *httptest.ResponseRecorder
		cause   string
	}{
		{"index", func() *httptest.ResponseRecorder { return do(t, h, http.MethodGet, "/") }, "database is closed"},
		{"mark today", func() *httptest.ResponseRecorder { return post(t, h, "/today", url.Values{}) }, "database is closed"},
		{"export", func() *httptest.ResponseRecorder { return do(t, h, http.MethodGet, "/export/2026") }, "database is closed"},
		{"subscribe", func() *httptest.ResponseRecorder {
			return postJSON(t, h, "/reminder/subscriptions", `{"endpoint":"https://push.example/1","keys":{"p256dh":"key","auth":"auth"}}`)
		}, "database is closed"},
		{"test reminder", func() *httptest.ResponseRecorder { return do(t, h, http.MethodPost, "/reminder/test") }, "push service exploded"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs.Reset()
			rec := tt.request()
			assertStatus(t, rec, http.StatusInternalServerError)
			if body := rec.Body.String(); strings.Contains(body, tt.cause) {
				t.Errorf("body shows the cause %q: %q", tt.cause, body)
			}
			if !strings.Contains(logs.String(), tt.cause) {
				t.Errorf("log does not contain %q: %q", tt.cause, logs.String())
			}
		})
	}
}
