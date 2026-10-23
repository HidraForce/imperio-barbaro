package main

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/go-chi/chi/v5"
)

func notificationTestApp(t *testing.T) (*app, http.Handler) {
	t.Helper()
	a := adminTestApp(t)
	var err error
	a.notifications, err = initNotifications(a.db)
	if err != nil {
		t.Fatal(err)
	}
	a.contactsVault = newContactVault("contacts-test-private-password")
	r := chi.NewRouter()
	r.Get("/api/announcements", a.announcementStatus)
	r.Post("/api/announcements", a.sendAnnouncement)
	r.Group(func(r chi.Router) {
		r.Use(a.contactsVault.protect)
		r.Post("/api/announcements/custom", a.sendAnnouncement)
	})
	guard := newAccessGuard("leader", "private-admin-test-password")
	return a, secureHeaders(a.publicPush(publicHome(a.staticFS, guard.middleware(sameOriginAPI(r)))))
}
func testPushSubscription(t *testing.T) webpush.Subscription {
	t.Helper()
	key, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return webpush.Subscription{Endpoint: "https://fcm.googleapis.com/fcm/send/" + randomPushID(), Keys: webpush.Keys{P256dh: base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()), Auth: base64.RawURLEncoding.EncodeToString(make([]byte, 16))}}
}
func enrollPush(t *testing.T, h http.Handler, clan string) webpush.Subscription {
	t.Helper()
	s := testPushSubscription(t)
	res := adminTestRequest(h, "POST", "https://imperiobarbaro.com.br/api/push/subscription", map[string]any{"clanId": clan, "subscription": s}, false)
	if res.Code != 200 {
		t.Fatalf("enroll: %d %s", res.Code, res.Body.String())
	}
	return s
}
func pushCount(t *testing.T, a *app, query string) int {
	t.Helper()
	var n int
	if err := a.db.QueryRow(query).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestPushAccessAndOrigin(t *testing.T) {
	_, h := notificationTestApp(t)
	for _, path := range []string{"/instalar", "/app", "/member.webmanifest", "/pwa-sw.js", "/pwa-state-worker.js", "/app-icon-192.png", "/api/push/config"} {
		res := adminTestRequest(h, "GET", "https://imperiobarbaro.com.br"+path, nil, false)
		if res.Code != 200 {
			t.Fatalf("public %s: %d", path, res.Code)
		}
	}
	if res := adminTestRequest(h, "POST", "https://dashboard.imperiobarbaro.com.br/api/announcements", map[string]string{"clanId": "1", "kind": "raids"}, false); res.Code != 401 {
		t.Fatalf("anonymous publish %d", res.Code)
	}
	if res := adminTestRequest(h, "POST", "https://dashboard.imperiobarbaro.com.br/api/announcements/custom", map[string]string{"clanId": "1", "kind": "custom", "title": "T", "body": "B"}, true); res.Code != 423 {
		t.Fatalf("custom bypass %d", res.Code)
	}
	if res := adminTestRequest(h, "POST", "https://dashboard.imperiobarbaro.com.br/api/announcements", map[string]string{"clanId": "1", "kind": "custom", "title": "T", "body": "B"}, true); res.Code != 403 {
		t.Fatalf("preset custom bypass %d", res.Code)
	}
	for _, origin := range []string{"https://evil.test", "https://dashboard.imperiobarbaro.com.br"} {
		req := httptest.NewRequest("POST", "https://imperiobarbaro.com.br/api/push/subscription", strings.NewReader(`{}`))
		req.Header.Set("Origin", origin)
		req.Header.Set("X-Clanboard-Request", "1")
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		if res.Code != 403 {
			t.Fatalf("origin %s allowed", origin)
		}
	}
}

func TestSubscriptionValidationAndOwnership(t *testing.T) {
	a, h := notificationTestApp(t)
	s := testPushSubscription(t)
	for _, endpoint := range []string{"https://127.0.0.1/private", "https://fcm.googleapis.com.evil.test/path", "http://fcm.googleapis.com/send/x", "https://user:pass@fcm.googleapis.com/x", "https://fcm.googleapis.com:8443/x"} {
		bad := s
		bad.Endpoint = endpoint
		if validPushSubscription(bad) {
			t.Fatalf("unsafe endpoint %s", endpoint)
		}
	}
	for _, endpoint := range []string{"https://web.push.apple.com/Q/token", "https://updates.push.services.mozilla.com/wpush/v2/token", "https://fcm.googleapis.com/wp/token"} {
		good := s
		good.Endpoint = endpoint
		if !validPushSubscription(good) {
			t.Fatalf("valid service rejected %s", endpoint)
		}
	}
	bad := s
	bad.Keys.P256dh = base64.RawURLEncoding.EncodeToString(make([]byte, 65))
	if validPushSubscription(bad) {
		t.Fatal("invalid curve point accepted")
	}
	s = enrollPush(t, h, "1")
	res := adminTestRequest(h, "DELETE", "https://imperiobarbaro.com.br/api/push/subscription", map[string]string{"endpoint": s.Endpoint, "auth": "wrong"}, false)
	if res.Code != 204 || pushCount(t, a, `SELECT COUNT(*) FROM push_subscriptions`) != 1 {
		t.Fatal("unauthorized deletion")
	}
	res = adminTestRequest(h, "DELETE", "https://imperiobarbaro.com.br/api/push/subscription", map[string]string{"endpoint": s.Endpoint, "auth": s.Keys.Auth}, false)
	if res.Code != 204 || pushCount(t, a, `SELECT COUNT(*) FROM push_subscriptions`) != 0 {
		t.Fatal("unsubscribe failed")
	}
}

func TestAnnouncementCooldownAtomicAndPersisted(t *testing.T) {
	a, h := notificationTestApp(t)
	enrollPush(t, h, "1")
	enrollPush(t, h, "2")
	var group sync.WaitGroup
	codes := make(chan int, 8)
	for range 8 {
		group.Add(1)
		go func() {
			defer group.Done()
			codes <- adminTestRequest(h, "POST", "https://dashboard.imperiobarbaro.com.br/api/announcements", map[string]string{"clanId": "1", "kind": "raids", "body": "tampered"}, true).Code
		}()
	}
	group.Wait()
	close(codes)
	accepted := 0
	for code := range codes {
		if code == 202 {
			accepted++
		} else if code != 429 {
			t.Fatalf("unexpected concurrent status %d", code)
		}
	}
	if accepted != 1 {
		t.Fatalf("accepted %d sends", accepted)
	}
	var body string
	var next int64
	a.db.QueryRow(`SELECT body FROM announcements LIMIT 1`).Scan(&body)
	if strings.Contains(body, "tampered") {
		t.Fatal("preset text replaced")
	}
	a.db.QueryRow(`SELECT next_at FROM announcement_cooldowns WHERE clan_id='1' AND kind='raids'`).Scan(&next)
	if next < time.Now().Add(59*time.Minute).Unix() {
		t.Fatal("cooldown too short")
	}
	// Reinitializing the service preserves both the cooldown and VAPID identity.
	restarted, err := initNotifications(a.db)
	if err != nil || restarted.publicKey != a.notifications.publicKey {
		t.Fatal("VAPID key rotated")
	}
	a.notifications = restarted
	if res := adminTestRequest(h, "POST", "https://dashboard.imperiobarbaro.com.br/api/announcements", map[string]string{"clanId": "1", "kind": "raids"}, true); res.Code != 429 || res.Header().Get("Retry-After") == "" {
		t.Fatal("cooldown lost on restart")
	}
	for _, input := range []map[string]string{{"clanId": "2", "kind": "raids"}, {"clanId": "1", "kind": "games"}} {
		if res := adminTestRequest(h, "POST", "https://dashboard.imperiobarbaro.com.br/api/announcements", input, true); res.Code != 202 {
			t.Fatalf("independent button blocked %d", res.Code)
		}
	}
	if _, err := a.db.Exec(`UPDATE announcement_cooldowns SET next_at=0`); err != nil {
		t.Fatal(err)
	}
	if res := adminTestRequest(h, "POST", "https://dashboard.imperiobarbaro.com.br/api/announcements", map[string]string{"clanId": "1", "kind": "raids"}, true); res.Code != 202 {
		t.Fatal("expired cooldown stayed blocked")
	}
}

func TestEmptyAudienceDoesNotConsumeCooldown(t *testing.T) {
	a, h := notificationTestApp(t)
	res := adminTestRequest(h, "POST", "https://dashboard.imperiobarbaro.com.br/api/announcements", map[string]string{"clanId": "1", "kind": "raids"}, true)
	if res.Code != 409 || pushCount(t, a, `SELECT COUNT(*) FROM announcement_cooldowns`) != 0 {
		t.Fatal("empty audience consumed cooldown")
	}
}

func TestCustomPasswordAndPublicFeed(t *testing.T) {
	a, h := notificationTestApp(t)
	enrollPush(t, h, "1")
	unlock := httptest.NewRecorder()
	a.contactsVault.unlock(unlock, httptest.NewRequest("POST", "/api/contacts/unlock", strings.NewReader(`{"password":"contacts-test-private-password"}`)))
	var session struct {
		Token string `json:"token"`
	}
	json.Unmarshal(unlock.Body.Bytes(), &session)
	req := httptest.NewRequest("POST", "https://dashboard.imperiobarbaro.com.br/api/announcements/custom", strings.NewReader(`{"clanId":"1","kind":"custom","title":"Novo aviso","body":"Recado curto."}`))
	req.SetBasicAuth("leader", "private-admin-test-password")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Clanboard-Request", "1")
	req.Header.Set("X-Contacts-Session", session.Token)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != 202 {
		t.Fatalf("custom: %d %s", res.Code, res.Body.String())
	}
	feed := adminTestRequest(h, "GET", "https://imperiobarbaro.com.br/api/push/announcements?clan=1", nil, false)
	if !strings.Contains(feed.Body.String(), "Novo aviso") || strings.Contains(feed.Body.String(), "recipients") || strings.Contains(feed.Body.String(), "endpoint") {
		t.Fatal("incorrect public feed")
	}
	other := adminTestRequest(h, "GET", "https://imperiobarbaro.com.br/api/push/announcements?clan=2", nil, false)
	if strings.Contains(other.Body.String(), "Novo aviso") {
		t.Fatal("clan feed mixed")
	}
}

type pushTestClient struct {
	status int
	calls  int
	t      *testing.T
}

func (c *pushTestClient) Do(r *http.Request) (*http.Response, error) {
	c.calls++
	if r.Header.Get("Urgency") != "high" || r.Header.Get("TTL") == "" || !strings.HasPrefix(r.Header.Get("Authorization"), "vapid ") {
		c.t.Fatal("push priority or authentication missing")
	}
	return &http.Response{StatusCode: c.status, Body: io.NopCloser(strings.NewReader(""))}, nil
}

func TestPushDeliveryRetriesExpiredSubscriptionsAndRestart(t *testing.T) {
	a, h := notificationTestApp(t)
	enrollPush(t, h, "1")
	res := adminTestRequest(h, "POST", "https://dashboard.imperiobarbaro.com.br/api/announcements", map[string]string{"clanId": "1", "kind": "raids"}, true)
	if res.Code != 202 {
		t.Fatal(res.Body.String())
	}
	client := &pushTestClient{503, 0, t}
	a.notifications.client = client
	a.dispatchPush(context.Background())
	if client.calls != 1 || pushCount(t, a, `SELECT COUNT(*) FROM push_jobs WHERE state='pending' AND attempts=1 AND lease_until=0`) != 1 {
		t.Fatal("failed delivery not retried")
	}
	a.notifications, _ = initNotifications(a.db)
	a.notifications.client = client
	client.status = 201
	a.db.Exec(`UPDATE push_jobs SET next_at=0`)
	a.dispatchPush(context.Background())
	if client.calls != 2 || pushCount(t, a, `SELECT COUNT(*) FROM push_jobs WHERE state='accepted'`) != 1 {
		t.Fatal("restart failed to resume outbox")
	}
	a.dispatchPush(context.Background())
	if client.calls != 2 {
		t.Fatal("accepted notification redelivered")
	}
	a.db.Exec(`UPDATE announcement_cooldowns SET next_at=0`)
	adminTestRequest(h, "POST", "https://dashboard.imperiobarbaro.com.br/api/announcements", map[string]string{"clanId": "1", "kind": "raids"}, true)
	client.status = 410
	a.dispatchPush(context.Background())
	if pushCount(t, a, `SELECT COUNT(*) FROM push_subscriptions`) != 0 {
		t.Fatal("expired subscription not removed")
	}
}

func TestSecondWarAttackOnlyAfterTwelveHours(t *testing.T) {
	a, _ := notificationTestApp(t)
	a.token = "test-token"
	now := time.Now().UTC()
	start := now.Add(-11 * time.Hour)
	state := "inWar"
	a.clash = &http.Client{Transport: clashTransport(func(_ *http.Request) (*http.Response, error) {
		body, _ := json.Marshal(map[string]any{"state": state, "attacksPerMember": 2, "startTime": start.Format("20060102T150405.000Z"), "endTime": now.Add(12 * time.Hour).Format("20060102T150405.000Z")})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	})}
	if a.warReminderReady(context.Background(), "1", true, now) == nil {
		t.Fatal("second attack released before 12h")
	}
	if err := a.warReminderReady(context.Background(), "1", false, now); err != nil {
		t.Fatal("first attack incorrectly blocked", err)
	}
	a.cache = nil
	start = now.Add(-12 * time.Hour)
	if err := a.warReminderReady(context.Background(), "1", true, now); err != nil {
		t.Fatal("second attack not released", err)
	}
	a.cache = nil
	state = "preparation"
	if a.warReminderReady(context.Background(), "1", false, now) == nil {
		t.Fatal("preparation allowed")
	}
}
