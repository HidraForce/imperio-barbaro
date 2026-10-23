package main

import (
	"fmt"
	"net/http"
	"testing"
)

func TestDeviceTestUsesRealPushWithoutBroadcastOrHistory(t *testing.T) {
	for _, postgres := range []bool{false, true} {
		t.Run(fmt.Sprint("postgres=", postgres), func(t *testing.T) {
			a := &app{db: integrationDatabase(t, postgres)}
			var err error
			a.notifications, err = initNotifications(a.db)
			if err != nil {
				t.Fatal(err)
			}
			h := a.publicPush(http.NotFoundHandler())
			s := enrollPush(t, h, "1")
			enrollPush(t, h, "1")
			client := &pushTestClient{201, 0, t}
			a.notifications.client = client
			unknown := testPushSubscription(t)
			if w := adminTestRequest(h, "POST", "https://imperiobarbaro.com.br/api/push/test", map[string]any{"subscription": unknown}, false); w.Code != 409 {
				t.Fatal("unregistered device tested", w.Code)
			}
			bad := s
			bad.Keys.Auth = "AAAAAAAAAAAAAAAAAAAAAQ"
			if w := adminTestRequest(h, "POST", "https://imperiobarbaro.com.br/api/push/test", map[string]any{"subscription": bad}, false); w.Code != 409 {
				t.Fatal("ownership not checked", w.Code)
			}
			if client.calls != 0 {
				t.Fatal("unowned push sent")
			}
			for _, want := range []int{202, 429} {
				w := adminTestRequest(h, "POST", "https://imperiobarbaro.com.br/api/push/test", map[string]any{"subscription": map[string]any{"endpoint": s.Endpoint, "keys": s.Keys, "expirationTime": nil}}, false)
				if w.Code != want {
					t.Fatal(w.Code, w.Body.String())
				}
			}
			if client.calls != 1 || pushCount(t, a, `SELECT COUNT(*) FROM announcements`) != 0 || pushCount(t, a, `SELECT COUNT(*) FROM push_jobs`) != 0 || pushCount(t, a, `SELECT COUNT(*) FROM announcement_cooldowns`) != 0 {
				t.Fatal("test broadcast or modified clan history/cooldowns")
			}
			_, _ = a.db.Exec(`UPDATE push_test_limits SET next_at=0`)
			client.status = 410
			if w := adminTestRequest(h, "POST", "https://imperiobarbaro.com.br/api/push/test", map[string]any{"subscription": map[string]any{"endpoint": s.Endpoint, "keys": s.Keys, "expirationTime": nil}}, false); w.Code != 409 {
				t.Fatal("expired device not reported", w.Code)
			}
			if pushCount(t, a, `SELECT COUNT(*) FROM push_subscriptions`) != 1 {
				t.Fatal("wrong device removed")
			}
		})
	}
}
