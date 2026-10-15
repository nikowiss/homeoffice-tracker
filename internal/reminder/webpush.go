package reminder

import (
	"io"

	"github.com/SherClockHolmes/webpush-go"

	"homeoffice-tracker/internal/store"
)

const subscriber = "https://github.com/nikowiss/homeoffice-tracker"

type WebPushSender struct {
	Keys store.VAPIDKeys
}

func (w WebPushSender) Send(sub store.Subscription, payload []byte, ttlSeconds int) (int, error) {
	resp, err := webpush.SendNotification(payload, &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys:     webpush.Keys{P256dh: sub.P256dh, Auth: sub.Auth},
	}, &webpush.Options{
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
