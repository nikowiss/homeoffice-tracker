package reminder

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"homeoffice-tracker/internal/civil"
	"homeoffice-tracker/internal/store"
)

const payload = `{"title":"Home office today?","body":"Open the tracker to mark today."}`

type Sender interface {
	Send(ctx context.Context, sub store.Subscription, payload []byte, ttlSeconds int) (status int, err error)
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
		if err := s.Check(ctx); err != nil {
			slog.Error("reminder check failed", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) Check(ctx context.Context) error {
	now := s.now()
	today := civil.Today(now)
	marked, err := s.store.Marked(ctx, today)
	if err != nil {
		return err
	}
	last, err := s.store.LastReminderDate(ctx)
	if err != nil {
		return err
	}
	if !Due(now, s.at, marked, last) {
		return nil
	}
	accepted, err := s.sendToAll(ctx, now)
	if err != nil || !accepted {
		return err
	}
	return s.store.SetLastReminderDate(ctx, today)
}

func (s *Service) SendNow(ctx context.Context) error {
	_, err := s.sendToAll(ctx, s.now())
	return err
}

func (s *Service) sendToAll(ctx context.Context, now time.Time) (accepted bool, err error) {
	subs, err := s.store.Subscriptions(ctx)
	if err != nil {
		return false, err
	}
	ttl := SecondsUntilMidnight(now)
	for _, sub := range subs {
		status, err := s.sender.Send(ctx, sub, []byte(payload), ttl)
		switch {
		case err != nil:
			slog.Warn("send reminder failed", "endpoint", sub.Endpoint, "err", err)
		case status == http.StatusNotFound || status == http.StatusGone:
			if err := s.store.DeleteSubscription(ctx, sub.Endpoint); err != nil {
				return accepted, err
			}
		case status >= 200 && status < 300:
			accepted = true
		default:
			slog.Warn("push service rejected reminder", "endpoint", sub.Endpoint, "status", status)
		}
	}
	return accepted, nil
}
