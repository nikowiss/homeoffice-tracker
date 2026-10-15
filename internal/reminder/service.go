package reminder

import (
	"context"
	"log"
	"net/http"
	"time"

	"homeoffice-tracker/internal/civil"
	"homeoffice-tracker/internal/store"
)

const payload = `{"title":"Home office today?","body":"Open the tracker to mark today."}`

type Sender interface {
	Send(sub store.Subscription, payload []byte, ttlSeconds int) (status int, err error)
}

type Service struct {
	store  *store.Store
	sender Sender
	at     TimeOfDay
	now    func() time.Time
}

func New(s *store.Store, sender Sender, at TimeOfDay, now func() time.Time) *Service {
	return &Service{store: s, sender: sender, at: at, now: now}
}

func (r *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		if err := r.Check(); err != nil {
			log.Printf("reminder check: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (r *Service) Check() error {
	now := r.now()
	today := civil.DateOf(now.In(berlin))
	marked, err := r.store.Marked(today)
	if err != nil {
		return err
	}
	last, err := r.store.LastReminderDate()
	if err != nil {
		return err
	}
	if !Due(now, r.at, marked, last) {
		return nil
	}
	accepted, err := r.sendToAll(now)
	if err != nil || !accepted {
		return err
	}
	return r.store.SetLastReminderDate(today)
}

func (r *Service) SendNow() error {
	_, err := r.sendToAll(r.now())
	return err
}

func (r *Service) sendToAll(now time.Time) (accepted bool, err error) {
	subs, err := r.store.Subscriptions()
	if err != nil {
		return false, err
	}
	ttl := SecondsUntilMidnight(now)
	for _, sub := range subs {
		status, err := r.sender.Send(sub, []byte(payload), ttl)
		switch {
		case err != nil:
			log.Printf("send reminder to %s: %v", sub.Endpoint, err)
		case status == http.StatusNotFound || status == http.StatusGone:
			if err := r.store.DeleteSubscription(sub.Endpoint); err != nil {
				return accepted, err
			}
		case status >= 200 && status < 300:
			accepted = true
		default:
			log.Printf("send reminder to %s: push service answered %d", sub.Endpoint, status)
		}
	}
	return accepted, nil
}
