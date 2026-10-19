package web

import (
	"encoding/json"
	"net/http"

	"homeoffice-tracker/internal/store"
)

type pushSubscription struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

func (s *server) subscribe(w http.ResponseWriter, r *http.Request) {
	var sub pushSubscription
	if err := json.NewDecoder(r.Body).Decode(&sub); err != nil {
		http.Error(w, "invalid subscription", http.StatusBadRequest)
		return
	}
	if sub.Endpoint == "" || sub.Keys.P256dh == "" || sub.Keys.Auth == "" {
		http.Error(w, "subscription needs endpoint, p256dh and auth", http.StatusBadRequest)
		return
	}
	err := s.store.SaveSubscription(r.Context(), store.Subscription{Endpoint: sub.Endpoint, P256dh: sub.Keys.P256dh, Auth: sub.Keys.Auth})
	if err != nil {
		serverError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) unsubscribe(w http.ResponseWriter, r *http.Request) {
	var sub pushSubscription
	if err := json.NewDecoder(r.Body).Decode(&sub); err != nil || sub.Endpoint == "" {
		http.Error(w, "invalid subscription", http.StatusBadRequest)
		return
	}
	if err := s.store.DeleteSubscription(r.Context(), sub.Endpoint); err != nil {
		serverError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) sendTestReminder(w http.ResponseWriter, r *http.Request) {
	if err := s.reminders.SendNow(r.Context()); err != nil {
		serverError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
