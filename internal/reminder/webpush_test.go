package reminder_test

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/SherClockHolmes/webpush-go"

	"homeoffice-tracker/internal/reminder"
	"homeoffice-tracker/internal/store"
)

func TestWebPushSenderGivesUpOnHangingPushService(t *testing.T) {
	release := make(chan struct{})
	pushService := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	t.Cleanup(pushService.Close)
	t.Cleanup(func() { close(release) })

	sender := reminder.WebPushSender{Keys: vapidKeys(t), Timeout: 100 * time.Millisecond}
	done := make(chan error, 1)
	go func() {
		_, err := sender.Send(browserSubscription(t, pushService.URL), []byte("{}"), 60)
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Send to a hanging push service returned no error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Send still blocks after 2s")
	}
}

func vapidKeys(t *testing.T) store.VAPIDKeys {
	t.Helper()
	private, public, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		t.Fatalf("generate VAPID keys: %v", err)
	}
	return store.VAPIDKeys{Private: private, Public: public}
}

func browserSubscription(t *testing.T, endpoint string) store.Subscription {
	t.Helper()
	key, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate browser key: %v", err)
	}
	return store.Subscription{
		Endpoint: endpoint,
		P256dh:   base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()),
		Auth:     base64.RawURLEncoding.EncodeToString([]byte(rand.Text())[:16]),
	}
}
