package reminder

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/SherClockHolmes/webpush-go"

	"homeoffice-tracker/internal/store"
)

const (
	subscriber         = "https://github.com/nikowiss/homeoffice-tracker"
	defaultSendTimeout = 10 * time.Second
)

type WebPushSender struct {
	Keys    store.VAPIDKeys
	Timeout time.Duration
}

func (w WebPushSender) Send(ctx context.Context, sub store.Subscription, payload []byte, ttlSeconds int) (int, error) {
	timeout := w.Timeout
	if timeout == 0 {
		timeout = defaultSendTimeout
	}
	resp, err := webpush.SendNotificationWithContext(ctx, payload, &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys:     webpush.Keys{P256dh: sub.P256dh, Auth: sub.Auth},
	}, &webpush.Options{
		HTTPClient:      &http.Client{Timeout: timeout},
		Subscriber:      subscriber,
		VAPIDPublicKey:  w.Keys.Public,
		VAPIDPrivateKey: w.Keys.Private,
		TTL:             ttlSeconds,
		Urgency:         webpush.UrgencyNormal,
	})
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}
