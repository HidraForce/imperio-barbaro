package main

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
)

const announcementCooldown = time.Hour

type notificationService struct {
	publicKey, privateKey string
	client                webpush.HTTPClient
}

type announcement struct {
	ID         string `json:"id"`
	ClanID     string `json:"clanId"`
	Kind       string `json:"kind"`
	Title      string `json:"title"`
	Body       string `json:"body"`
	CreatedAt  int64  `json:"createdAt"`
	Recipients int    `json:"recipients"`
	Accepted   int    `json:"accepted"`
	Pending    int    `json:"pending"`
	Failed     int    `json:"failed"`
}

type announcementTemplate struct {
	Kind  string `json:"kind"`
	Label string `json:"label"`
	Title string `json:"title"`
	Body  string `json:"body"`
}

var announcementTemplates = []announcementTemplate{
	{"war-zero", "Guerra · 0 ataques", "⚔️ Guerra · primeiro ataque", "Está com 0 ataques? Faça seu primeiro ataque na guerra e siga a orientação da liderança."},
	{"war-second", "Guerra · 1 de 2 ataques", "⚔️ Guerra · segundo ataque liberado", "Já fez 1 de 2 ataques? Passaram 12 horas: use o segundo ataque em qualquer alvo!"},
	{"raids", "Raids do fim de semana", "🏰 Raids do fim de semana", "Não se esqueça dos ataques nas raids do fim de semana!"},
	{"games", "Jogos do clã", "🎯 Jogos do clã", "Não se esqueça de fazer seus pontos nos jogos do clã!"},
	{"cwl", "Liga de Guerra · CWL", "🏆 CWL · um ataque por rodada", "Está escalado na CWL? Não se esqueça do seu único ataque nesta rodada!"},
}

func announcementClanName(clanID string) string {
	if clan, valid := findCommunityClan(clanID); valid {
		if clan.AnnouncementLabel != "" {
			return clan.Name + " · " + clan.AnnouncementLabel
		}
		return clan.Name
	}
	return "Comunidade do Império"
}

func announcementPushPayload(item announcement) map[string]any {
	clan := announcementClanName(item.ClanID)
	return map[string]any{"id": item.ID, "clanId": item.ClanID, "clanName": clan, "title": clan + " · " + item.Title, "body": "Clã: " + clan + "\n" + item.Body, "url": "/app#" + item.ID}
}

func initNotifications(db *database) (*notificationService, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if db.postgres {
		if _, err = tx.Exec(`SELECT pg_advisory_xact_lock(629364820)`); err != nil {
			return nil, err
		}
	}
	for _, query := range []string{
		`CREATE TABLE IF NOT EXISTS push_keys (id INTEGER PRIMARY KEY, public_key TEXT NOT NULL, private_key TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS push_subscriptions (id TEXT PRIMARY KEY, endpoint TEXT NOT NULL UNIQUE, p256dh TEXT NOT NULL, auth TEXT NOT NULL, clan_id TEXT NOT NULL, updated_at BIGINT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS announcements (id TEXT PRIMARY KEY, clan_id TEXT NOT NULL, kind TEXT NOT NULL, title TEXT NOT NULL, body TEXT NOT NULL, created_at BIGINT NOT NULL, recipients INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS announcement_cooldowns (clan_id TEXT NOT NULL, kind TEXT NOT NULL, next_at BIGINT NOT NULL, PRIMARY KEY(clan_id,kind))`,
		`CREATE TABLE IF NOT EXISTS push_jobs (id TEXT PRIMARY KEY, announcement_id TEXT NOT NULL, subscription_id TEXT NOT NULL, state TEXT NOT NULL, attempts INTEGER NOT NULL DEFAULT 0, next_at BIGINT NOT NULL, lease_until BIGINT NOT NULL DEFAULT 0)`,
		`CREATE TABLE IF NOT EXISTS push_test_limits (subscription_id TEXT PRIMARY KEY, next_at BIGINT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS push_jobs_due ON push_jobs(state,next_at,lease_until)`,
		`CREATE INDEX IF NOT EXISTS push_jobs_announcement ON push_jobs(announcement_id,state)`,
		`CREATE INDEX IF NOT EXISTS announcements_recent ON announcements(clan_id,created_at DESC,id DESC)`,
		`CREATE INDEX IF NOT EXISTS push_subscriptions_clan ON push_subscriptions(clan_id)`,
	} {
		if _, err = tx.Exec(query); err != nil {
			return nil, err
		}
	}
	var public, private string
	err = tx.QueryRow(`SELECT public_key,private_key FROM push_keys WHERE id=1`).Scan(&public, &private)
	if err == sql.ErrNoRows {
		private, public, err = webpush.GenerateVAPIDKeys()
		if err == nil {
			_, err = tx.Exec(`INSERT INTO push_keys(id,public_key,private_key) VALUES(1,?,?)`, public, private)
		}
	}
	if err != nil {
		return nil, err
	}
	if err = maintainAnnouncementsTx(context.Background(), tx, time.Now().Unix()); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &notificationService{publicKey: public, privateKey: private, client: &http.Client{Timeout: 12 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func randomPushID() string {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(id[:])
}

// Subscription URLs are provided by visitors. Only known browser push services
// are allowed, so dispatch cannot be used to reach arbitrary hosts or the LAN.
func validPushSubscription(s webpush.Subscription) bool {
	u, err := url.Parse(s.Endpoint)
	if err != nil || len(s.Endpoint) > 2048 || u.Scheme != "https" || u.User != nil || u.Fragment != "" || (u.Port() != "" && u.Port() != "443") || u.Path == "" || u.Path == "/" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	allowed := host == "fcm.googleapis.com" || host == "updates.push.services.mozilla.com" || strings.HasSuffix(host, ".push.services.mozilla.com") || strings.HasSuffix(host, ".push.apple.com") || strings.HasSuffix(host, ".notify.windows.com")
	key, err := base64.RawURLEncoding.DecodeString(s.Keys.P256dh)
	if err != nil || len(key) != 65 {
		return false
	}
	if _, err = ecdh.P256().NewPublicKey(key); err != nil {
		return false
	}
	auth, err := base64.RawURLEncoding.DecodeString(s.Keys.Auth)
	return allowed && err == nil && len(auth) == 16
}

func (a *app) pushConfig(w http.ResponseWriter, _ *http.Request) {
	if a.notifications == nil {
		securityError(w, 503, "Os avisos estão indisponíveis. Tente novamente em instantes.")
		return
	}
	clans := make([]map[string]string, 0, len(communityClans))
	for _, clan := range communityClans {
		clans = append(clans, map[string]string{"id": clan.ID, "name": announcementClanName(clan.ID)})
	}
	jsonResponse(w, 200, map[string]any{"publicKey": a.notifications.publicKey, "clans": clans})
}

// Browsers include expirationTime (usually null) in PushSubscription.toJSON().
type browserPushSubscription struct {
	Endpoint       string       `json:"endpoint"`
	ExpirationTime *float64     `json:"expirationTime"`
	Keys           webpush.Keys `json:"keys"`
}

const pushClanMatch = `(clan_id NOT LIKE '%,%' OR clan_id LIKE ?)`

func pushClanPattern(clanID string) string { return "%," + clanID + ",%" }

func pushClanIDs(preference string) []string {
	ids := []string{}
	for _, clan := range communityClans {
		if !strings.Contains(preference, ",") || strings.Contains(preference, ","+clan.ID+",") {
			ids = append(ids, clan.ID)
		}
	}
	return ids
}

func (a *app) subscribePush(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ClanID       string                  `json:"clanId"`
		ClanIDs      []string                `json:"clanIds"`
		Subscription browserPushSubscription `json:"subscription"`
	}
	if !decodeAdminJSON(w, r, &input) {
		return
	}
	s := webpush.Subscription{Endpoint: input.Subscription.Endpoint, Keys: input.Subscription.Keys}
	if !validPushSubscription(s) {
		securityError(w, 400, "Ative as notificações pelo navegador.")
		return
	}
	preference := "all"
	if _, valid := findCommunityClan(input.ClanID); valid {
		preference = input.ClanID
	}
	if input.ClanIDs != nil {
		if len(input.ClanIDs) == 0 || len(input.ClanIDs) > len(communityClans) {
			securityError(w, 400, "Selecione pelo menos um clã válido.")
			return
		}
		seen := make(map[string]bool, len(input.ClanIDs))
		for _, id := range input.ClanIDs {
			if _, valid := findCommunityClan(id); !valid || seen[id] {
				securityError(w, 400, "Selecione clãs válidos sem repetir a seleção.")
				return
			}
			seen[id] = true
		}
		preference = "," + strings.Join(input.ClanIDs, ",") + ","
	}
	id := fmt.Sprintf("%x", sha256.Sum256([]byte(s.Endpoint)))
	// The id is a capability, never returned in a public list. Deletion requires
	// the complete subscription rather than a guessable member identifier.
	err := a.db.QueryRowContext(r.Context(), `INSERT INTO push_subscriptions(id,endpoint,p256dh,auth,clan_id,updated_at) VALUES(?,?,?,?,?,?)
	 ON CONFLICT(id) DO UPDATE SET p256dh=excluded.p256dh,auth=excluded.auth,
	 clan_id=CASE WHEN excluded.clan_id NOT LIKE '%,%' AND push_subscriptions.clan_id LIKE '%,%' THEN push_subscriptions.clan_id ELSE excluded.clan_id END,
	 updated_at=excluded.updated_at RETURNING clan_id`, id, s.Endpoint, s.Keys.P256dh, s.Keys.Auth, preference, time.Now().Unix()).Scan(&preference)
	if err != nil {
		securityError(w, 500, "Não foi possível ativar os avisos. Tente novamente.")
		return
	}
	jsonResponse(w, 200, map[string]any{"subscribed": true, "clanIds": pushClanIDs(preference)})
}

func (a *app) unsubscribePush(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Endpoint string `json:"endpoint"`
		Auth     string `json:"auth"`
	}
	if !decodeAdminJSON(w, r, &input) {
		return
	}
	if _, err := a.db.ExecContext(r.Context(), `DELETE FROM push_subscriptions WHERE endpoint=? AND auth=?`, input.Endpoint, input.Auth); err != nil {
		securityError(w, 500, "Não foi possível atualizar as notificações.")
		return
	}
	w.WriteHeader(204)
}

// Recover only this device's clan if its local preference was lost. The complete
// browser subscription is the capability; an endpoint alone cannot reveal it.
func (a *app) restorePush(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Subscription browserPushSubscription `json:"subscription"`
	}
	if !decodeAdminJSON(w, r, &input) {
		return
	}
	s := webpush.Subscription{Endpoint: input.Subscription.Endpoint, Keys: input.Subscription.Keys}
	if !validPushSubscription(s) {
		securityError(w, 400, "Não foi possível conferir os avisos deste celular.")
		return
	}
	var clan string
	err := a.db.QueryRowContext(r.Context(), `SELECT clan_id FROM push_subscriptions WHERE endpoint=? AND p256dh=? AND auth=?`, s.Endpoint, s.Keys.P256dh, s.Keys.Auth).Scan(&clan)
	if err == sql.ErrNoRows {
		jsonResponse(w, 200, map[string]any{"clanId": "", "clanIds": []string{}})
		return
	}
	if err != nil {
		securityError(w, 500, "Não foi possível recuperar seu clã. Tente novamente.")
		return
	}
	ids := pushClanIDs(clan)
	if strings.Contains(clan, ",") {
		clan = "all"
		if len(ids) == 1 {
			clan = ids[0]
		}
	}
	jsonResponse(w, 200, map[string]any{"clanId": clan, "clanIds": ids})
}

func (a *app) announcementList(ctx context.Context, clanID string) ([]announcement, error) {
	rows, err := a.db.QueryContext(ctx, `SELECT a.id,a.clan_id,a.kind,a.title,a.body,a.created_at,a.recipients,
	 (SELECT COUNT(*) FROM push_jobs j WHERE j.announcement_id=a.id AND j.state='accepted'),
	 (SELECT COUNT(*) FROM push_jobs j WHERE j.announcement_id=a.id AND j.state='pending'),
	 (SELECT COUNT(*) FROM push_jobs j WHERE j.announcement_id=a.id AND j.state='failed')
	 FROM announcements a WHERE a.clan_id=? ORDER BY a.created_at DESC,a.id DESC LIMIT ?`, clanID, announcementHistoryLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []announcement{}
	for rows.Next() {
		var item announcement
		if err = rows.Scan(&item.ID, &item.ClanID, &item.Kind, &item.Title, &item.Body, &item.CreatedAt, &item.Recipients, &item.Accepted, &item.Pending, &item.Failed); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (a *app) publicAnnouncements(w http.ResponseWriter, r *http.Request) {
	clanID := r.URL.Query().Get("clan")
	clans := make([]string, 0, len(communityClans))
	if clanID == "" || clanID == "all" {
		for _, clan := range communityClans {
			clans = append(clans, clan.ID)
		}
	} else if _, valid := findCommunityClan(clanID); valid {
		clans = append(clans, clanID)
	} else {
		securityError(w, 400, "Filtro de avisos inválido.")
		return
	}
	items := []announcement{}
	for _, clan := range clans {
		part, err := a.announcementList(r.Context(), clan)
		if err != nil {
			securityError(w, 500, "Não foi possível carregar os avisos.")
			return
		}
		items = append(items, part...)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt != items[j].CreatedAt {
			return items[i].CreatedAt > items[j].CreatedAt
		}
		return items[i].ID > items[j].ID
	})
	// Member feeds contain only intentional collective messages, never contacts,
	// disciplinary records, subscriptions or delivery metrics.
	public := []map[string]any{}
	for _, item := range items {
		public = append(public, map[string]any{"id": item.ID, "clanId": item.ClanID, "clanName": announcementClanName(item.ClanID), "title": item.Title, "body": item.Body, "createdAt": item.CreatedAt})
	}
	jsonResponse(w, 200, map[string]any{"items": public})
}

func (a *app) announcementStatus(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("clan")
	if _, valid := findCommunityClan(id); !valid {
		securityError(w, 400, "Escolha o clã.")
		return
	}
	items, err := a.announcementList(r.Context(), id)
	if err != nil {
		securityError(w, 500, "Não foi possível carregar os comunicados.")
		return
	}
	cooldowns := map[string]int64{}
	rows, err := a.db.QueryContext(r.Context(), `SELECT kind,next_at FROM announcement_cooldowns WHERE clan_id=?`, id)
	if err != nil {
		securityError(w, 500, "Não foi possível conferir os intervalos.")
		return
	}
	for rows.Next() {
		var kind string
		var next int64
		if err = rows.Scan(&kind, &next); err != nil {
			break
		}
		cooldowns[kind] = next
	}
	rowErr := rows.Err()
	rows.Close()
	if err != nil || rowErr != nil {
		securityError(w, 500, "Não foi possível conferir os intervalos.")
		return
	}
	var subscribers int
	if err = a.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM push_subscriptions WHERE `+pushClanMatch, pushClanPattern(id)).Scan(&subscribers); err != nil {
		securityError(w, 500, "Não foi possível conferir as inscrições.")
		return
	}
	jsonResponse(w, 200, map[string]any{"clanName": announcementClanName(id), "templates": announcementTemplates, "cooldowns": cooldowns, "items": items, "subscribers": subscribers, "serverTime": time.Now().Unix(), "customConfigured": a.customVault != nil && a.customVault.configured, "timerConfigured": a.timerVault != nil && a.timerVault.configured})
}

func (a *app) warReminderReady(ctx context.Context, clanID string, second bool, now time.Time) error {
	tag := a.clanTags[clanID]
	if tag == "" || a.token == "" {
		return fmt.Errorf("Configure a API do clã para conferir a guerra antes de enviar.")
	}
	body, _, err := a.fetchClash(ctx, "/clans/"+url.PathEscape("#"+tag)+"/currentwar")
	if err != nil {
		return fmt.Errorf("Não foi possível conferir a guerra. Atualize os dados e tente novamente.")
	}
	var war struct {
		State            string `json:"state"`
		StartTime        string `json:"startTime"`
		EndTime          string `json:"endTime"`
		AttacksPerMember int    `json:"attacksPerMember"`
	}
	if json.Unmarshal(body, &war) != nil || war.State != "inWar" || war.AttacksPerMember != 2 {
		return fmt.Errorf("Este aviso exige uma guerra em andamento com dois ataques por jogador.")
	}
	start, err := time.Parse("20060102T150405.000Z", war.StartTime)
	end, endErr := time.Parse("20060102T150405.000Z", war.EndTime)
	if err != nil || endErr != nil || now.Before(start) || !now.Before(end) {
		return fmt.Errorf("O horário desta guerra não pôde ser confirmado.")
	}
	if second && now.Before(start.Add(12*time.Hour)) {
		return fmt.Errorf("O segundo ataque será liberado em %s, após 12 horas de guerra.", start.Add(12*time.Hour).In(time.FixedZone("Brasília", -3*3600)).Format("02/01 às 15:04"))
	}
	return nil
}

func (a *app) sendAnnouncement(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ClanID             string                  `json:"clanId"`
		Kind               string                  `json:"kind"`
		Title              string                  `json:"title"`
		Body               string                  `json:"body"`
		SenderSubscription browserPushSubscription `json:"senderSubscription"`
	}
	if !decodeAdminJSON(w, r, &input) {
		return
	}
	if _, valid := findCommunityClan(input.ClanID); !valid {
		securityError(w, 400, "Escolha o clã.")
		return
	}
	if a.notifications == nil {
		securityError(w, 503, "Os avisos estão indisponíveis.")
		return
	}
	if input.Kind == "custom" {
		if !strings.HasSuffix(r.URL.Path, "/custom") {
			securityError(w, 403, "Desbloqueie o aviso personalizado com sua senha exclusiva.")
			return
		}
		input.Title = strings.TrimSpace(input.Title)
		input.Body = strings.TrimSpace(input.Body)
		if !validAdminText(input.Title, 70, true) || !validAdminText(input.Body, 240, true) {
			securityError(w, 400, "Use um título de até 70 caracteres e uma mensagem de até 240.")
			return
		}
	} else {
		found := false
		for _, item := range announcementTemplates {
			if item.Kind == input.Kind {
				input.Title = item.Title
				input.Body = item.Body
				found = true
				break
			}
		}
		if !found || strings.HasSuffix(r.URL.Path, "/custom") {
			securityError(w, 400, "Escolha um dos avisos disponíveis.")
			return
		}
	}
	now := time.Now().UTC()
	if strings.HasPrefix(input.Kind, "war-") {
		if err := a.warReminderReady(r.Context(), input.ClanID, input.Kind == "war-second", now); err != nil {
			securityError(w, 409, err.Error())
			return
		}
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		securityError(w, 500, "Não foi possível preparar o aviso.")
		return
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`INSERT INTO announcement_cooldowns(clan_id,kind,next_at) VALUES(?,?,0) ON CONFLICT(clan_id,kind) DO NOTHING`, input.ClanID, input.Kind); err != nil {
		securityError(w, 500, "Não foi possível preparar o intervalo.")
		return
	}
	result, err := tx.Exec(`UPDATE announcement_cooldowns SET next_at=? WHERE clan_id=? AND kind=? AND next_at<=?`, now.Add(announcementCooldown).Unix(), input.ClanID, input.Kind, now.Unix())
	if err != nil {
		securityError(w, 500, "Não foi possível reservar o envio.")
		return
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		var next int64
		_ = tx.QueryRow(`SELECT next_at FROM announcement_cooldowns WHERE clan_id=? AND kind=?`, input.ClanID, input.Kind).Scan(&next)
		w.Header().Set("Retry-After", strconv.FormatInt(next-now.Unix(), 10))
		securityError(w, 429, "Este aviso já foi enviado. Aguarde o intervalo de uma hora.")
		return
	}
	sender := input.SenderSubscription
	var senderID string
	if validPushSubscription(webpush.Subscription{Endpoint: sender.Endpoint, Keys: sender.Keys}) {
		senderID = fmt.Sprintf("%x", sha256.Sum256([]byte(sender.Endpoint)))
	}
	id := randomPushID()
	if _, err = tx.Exec(`INSERT INTO announcements(id,clan_id,kind,title,body,created_at,recipients) VALUES(?,?,?,?,?,?,0)`, id, input.ClanID, input.Kind, input.Title, input.Body, now.Unix()); err != nil {
		securityError(w, 500, "Não foi possível registrar o aviso.")
		return
	}
	jobs, err := tx.Exec(`INSERT INTO push_jobs(id,announcement_id,subscription_id,state,next_at)
	 SELECT ? || id,?,id,'pending',? FROM push_subscriptions WHERE `+pushClanMatch+` OR (id=? AND p256dh=? AND auth=?)`,
		id, id, now.Unix(), pushClanPattern(input.ClanID), senderID, sender.Keys.P256dh, sender.Keys.Auth)
	if err != nil {
		securityError(w, 500, "Não foi possível registrar o envio.")
		return
	}
	recipients, err := jobs.RowsAffected()
	if err != nil {
		securityError(w, 500, "Não foi possível conferir os inscritos.")
		return
	}
	if recipients == 0 {
		securityError(w, 409, "Nenhum celular está inscrito neste clã. Compartilhe a página de instalação.")
		return
	}
	if _, err = tx.Exec(`UPDATE announcements SET recipients=? WHERE id=?`, recipients, id); err != nil {
		securityError(w, 500, "Não foi possível registrar o total de inscritos.")
		return
	}
	if err = maintainAnnouncementsTx(r.Context(), tx, now.Unix()); err != nil {
		securityError(w, 500, "Não foi possível atualizar o histórico de avisos.")
		return
	}
	if err = tx.Commit(); err != nil {
		securityError(w, 500, "Não foi possível confirmar o envio.")
		return
	}
	jsonResponse(w, 202, map[string]any{"id": id, "queued": recipients, "nextAt": now.Add(announcementCooldown).Unix()})
}

func (a *app) publicPush(next http.Handler) http.Handler {
	routes := map[string]http.HandlerFunc{
		"POST /api/push/test": a.testPush, "GET /api/push/config": a.pushConfig, "POST /api/push/subscription": a.subscribePush, "POST /api/push/subscription/restore": a.restorePush, "DELETE /api/push/subscription": a.unsubscribePush, "GET /api/push/announcements": a.publicAnnouncements,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if handler, ok := routes[r.Method+" "+r.URL.Path]; ok {
			sameOriginAPI(handler).ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Durable outbox: each delivery is leased atomically so restarts and multiple
// processes cannot dispatch the same job concurrently. Providers may still
// redeliver after an ambiguous network failure; the client uses a stable tag.
func (a *app) dispatchPush(ctx context.Context) {
	now := time.Now().Unix()
	if err := maintainAnnouncements(ctx, a.db, now); err != nil {
		log.Printf("Não foi possível limpar o histórico de comunicados: %v", err)
		return
	}
	rows, err := a.db.QueryContext(ctx, `SELECT id FROM push_jobs WHERE state='pending' AND next_at<=? AND lease_until<=? ORDER BY next_at LIMIT 40`, now, now)
	if err != nil {
		return
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	var workers sync.WaitGroup
	limit := make(chan struct{}, 4)
	for _, id := range ids {
		limit <- struct{}{}
		workers.Add(1)
		go func(id string) { defer workers.Done(); defer func() { <-limit }(); a.deliverPush(ctx, id) }(id)
	}
	workers.Wait()
}

func (a *app) deliverPush(ctx context.Context, id string) {
	now := time.Now().Unix()
	result, err := a.db.ExecContext(ctx, `UPDATE push_jobs SET lease_until=?,attempts=attempts+1 WHERE id=? AND state='pending' AND next_at<=? AND lease_until<=?`, now+60, id, now, now)
	if err != nil {
		return
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return
	}
	var item announcement
	var s webpush.Subscription
	var subscriptionID string
	var attempts int
	err = a.db.QueryRowContext(ctx, `SELECT a.id,a.clan_id,a.title,a.body,a.created_at,s.id,s.endpoint,s.p256dh,s.auth,j.attempts FROM push_jobs j JOIN announcements a ON a.id=j.announcement_id JOIN push_subscriptions s ON s.id=j.subscription_id WHERE j.id=?`, id).Scan(&item.ID, &item.ClanID, &item.Title, &item.Body, &item.CreatedAt, &subscriptionID, &s.Endpoint, &s.Keys.P256dh, &s.Keys.Auth, &attempts)
	if err == sql.ErrNoRows {
		_, _ = a.db.ExecContext(ctx, `UPDATE push_jobs SET state='failed' WHERE id=?`, id)
		return
	}
	if err != nil {
		return
	}
	if !validPushSubscription(s) {
		_, _ = a.db.ExecContext(ctx, `UPDATE push_jobs SET state='failed' WHERE id=?`, id)
		return
	}
	status, sendErr := a.sendPushPayload(ctx, s, announcementPushPayload(item), max(1, int(item.CreatedAt+3600-now)))
	state := "pending"
	if sendErr == nil && status >= 200 && status < 300 {
		state = "accepted"
	} else if status == 404 || status == 410 {
		state = "failed"
		_, _ = a.db.ExecContext(ctx, `DELETE FROM push_subscriptions WHERE id=?`, subscriptionID)
	} else if (status >= 400 && status < 500 && status != 429) || attempts >= 6 {
		state = "failed"
	}
	_, _ = a.db.ExecContext(ctx, `UPDATE push_jobs SET state=?,lease_until=0,next_at=? WHERE id=?`, state, now+int64(min(600, 15*(1<<attempts))), id)
}

func (a *app) runPushDelivery() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		a.dispatchPush(context.Background())
		<-ticker.C
	}
}
