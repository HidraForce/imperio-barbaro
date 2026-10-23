package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
)

// Tests and clan announcements use the same transport, priority and worker.
func (a *app) sendPushPayload(ctx context.Context, subscription webpush.Subscription, payload any, ttl int) (int, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}
	sendCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	response, err := webpush.SendNotificationWithContext(sendCtx, body, &subscription, &webpush.Options{HTTPClient: a.notifications.client, Subscriber: "https://imperiobarbaro.com.br", VAPIDPublicKey: a.notifications.publicKey, VAPIDPrivateKey: a.notifications.privateKey, TTL: ttl, Urgency: webpush.UrgencyHigh})
	if response == nil {
		return 0, err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	return response.StatusCode, err
}

// Possession of the enrolled subscription's secret is required. This route can
// only test that device; it cannot broadcast or send to a supplied arbitrary URL.
func (a *app) testPush(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Subscription browserPushSubscription `json:"subscription"`
	}
	if !decodeAdminJSON(w, r, &input) {
		return
	}
	if a.notifications == nil {
		securityError(w, 503, "Os avisos estão indisponíveis.")
		return
	}
	s := webpush.Subscription{Endpoint: input.Subscription.Endpoint, Keys: input.Subscription.Keys}
	if !validPushSubscription(s) {
		securityError(w, 400, "Ative os avisos neste celular antes de testar.")
		return
	}
	hash := sha256.Sum256([]byte(s.Endpoint))
	id := hex.EncodeToString(hash[:])
	var count int
	if err := a.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM push_subscriptions WHERE id=? AND endpoint=? AND auth=? AND p256dh=?`, id, s.Endpoint, s.Keys.Auth, s.Keys.P256dh).Scan(&count); err != nil {
		securityError(w, 500, "Não foi possível conferir este celular.")
		return
	}
	if count != 1 {
		securityError(w, 409, "A inscrição deste celular mudou. Verifique novamente a ativação dos avisos.")
		return
	}
	now := time.Now().Unix()
	// A single atomic upsert limits tests per device, including concurrent taps.
	result, err := a.db.ExecContext(r.Context(), `INSERT INTO push_test_limits(subscription_id,next_at) VALUES(?,?) ON CONFLICT(subscription_id) DO UPDATE SET next_at=excluded.next_at WHERE push_test_limits.next_at<=?`, id, now+60, now)
	if err != nil {
		securityError(w, 500, "Não foi possível preparar o teste.")
		return
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		w.Header().Set("Retry-After", strconv.Itoa(60))
		securityError(w, 429, "Aguarde um minuto antes de testar novamente neste celular.")
		return
	}
	status, err := a.sendPushPayload(r.Context(), s, map[string]string{"id": "test-" + randomPushID(), "title": "🛡️ Império Bárbaro", "body": "Tudo pronto! Este é um teste de aviso neste celular.", "url": "/app"}, 60)
	if status == 404 || status == 410 {
		_, _ = a.db.ExecContext(r.Context(), `DELETE FROM push_subscriptions WHERE id=?`, id)
		securityError(w, 409, "A inscrição deste celular expirou. Ative os avisos novamente.")
		return
	}
	if err != nil || status < 200 || status >= 300 {
		securityError(w, 502, "O serviço de push não aceitou o teste. Verifique a ativação e tente novamente em um minuto.")
		return
	}
	jsonResponse(w, 202, map[string]bool{"accepted": true})
}
