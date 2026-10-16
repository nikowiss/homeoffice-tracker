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

func New(st *store.Store, sender Sender, at TimeOfDay, now func() time.Time) *Service {
	return &Service{store: st, sender: sender, at: at, now: now}
}

func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		if err := s.Check(); err != nil {
			log.Printf("reminder check: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) Check() error {
	now := s.now()
	today := civil.Today(now)
	marked, err := s.store.Marked(today)
	if err != nil {
		return err
	}
	last, err := s.store.LastReminderDate()
	if err != nil {
		return err
	}
	if !Due(now, s.at, marked, last) {
		return nil
	}
	accepted, err := s.sendToAll(now)
	if err != nil || !accepted {
		return err
	}
	return s.store.SetLastReminderDate(today)
}

func (s *Service) SendNow() error {
	_, err := s.sendToAll(s.now())
	return err
}

func (s *Service) sendToAll(now time.Time) (accepted bool, err error) {
	subs, err := s.store.Subscriptions()
	if err != nil {
		return false, err
	}
	ttl := SecondsUntilMidnight(now)
	for _, sub := range subs {
		status, err := s.sender.Send(sub, []byte(payload), ttl)
		switch {
		case err != nil:
			log.Printf("send reminder to %s: %v", sub.Endpoint, err)
		case status == http.StatusNotFound || status == http.StatusGone:
			if err := s.store.DeleteSubscription(sub.Endpoint); err != nil {
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
