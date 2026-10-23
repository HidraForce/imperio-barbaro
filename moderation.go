package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

type moderationRecord struct {
	ID           string `json:"id"`
	Revision     int64  `json:"revision"`
	AutoRevision int64  `json:"autoRevision,omitempty"`
	PlayerTag    string `json:"playerTag"`
	PlayerName   string `json:"playerName"`
	Kind         string `json:"kind"`
	Sector       string `json:"sector"`
	Sanction     string `json:"sanction"`
	Status       string `json:"status"`
	Reason       string `json:"reason"`
	Action       string `json:"action"`
	Notes        string `json:"notes"`
	Source       string `json:"source"`
	EventKey     string `json:"eventKey"`
	EventStart   string `json:"eventStart,omitempty"`
	EventEnd     string `json:"eventEnd,omitempty"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
	IssuedAt     string `json:"issuedAt,omitempty"`
	AppliedAt    string `json:"appliedAt,omitempty"`
	ExpiresAt    string `json:"expiresAt,omitempty"`
}
type moderationSettings struct {
	Revision  int64    `json:"revision"`
	Automatic bool     `json:"automatic"`
	Since     string   `json:"since"`
	LastCheck string   `json:"lastCheck"`
	Problems  []string `json:"problems"`
}

func initModeration(db *database) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if db.postgres {
		if _, err = tx.Exec(`SELECT pg_advisory_xact_lock(629364820)`); err != nil {
			return err
		}
	}
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS moderation_records(id TEXT PRIMARY KEY,clan_tag TEXT NOT NULL,source_key TEXT NOT NULL,revision BIGINT NOT NULL,payload TEXT NOT NULL,created_at TEXT NOT NULL,UNIQUE(clan_tag,source_key))`,
		`CREATE INDEX IF NOT EXISTS moderation_clan_date ON moderation_records(clan_tag,created_at)`,
		`CREATE TABLE IF NOT EXISTS moderation_settings(clan_tag TEXT PRIMARY KEY,revision BIGINT NOT NULL,payload TEXT NOT NULL)`,
	} {
		if _, err = tx.Exec(q); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func readModerationSettings(ctx context.Context, db *database, tag string) (moderationSettings, error) {
	var s moderationSettings
	var payload string
	var revision int64
	err := db.QueryRowContext(ctx, `SELECT revision,payload FROM moderation_settings WHERE clan_tag=?`, tag).Scan(&revision, &payload)
	if err == sql.ErrNoRows {
		return moderationSettings{Revision: 1, Since: time.Now().UTC().Add(-7 * 24 * time.Hour).Format(time.RFC3339), Problems: []string{}}, nil
	}
	if err != nil {
		return s, err
	}
	err = json.Unmarshal([]byte(payload), &s)
	s.Revision = revision
	return s, err
}
func (a *app) moderation(w http.ResponseWriter, r *http.Request) {
	tag, ok := a.selectedClanTag(w, r)
	if !ok {
		return
	}
	settings, err := readModerationSettings(r.Context(), a.db, tag)
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": "Não foi possível ler a configuração dos avisos."})
		return
	}
	records, err := readModerationRecords(r.Context(), a.db, tag)
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": "Não foi possível carregar o histórico."})
		return
	}
	jsonResponse(w, 200, map[string]any{"items": evaluateModeration(records, time.Now().UTC()), "settings": settings})
}
func readModerationRecords(ctx context.Context, db *database, tag string) ([]moderationRecord, error) {
	rows, err := db.QueryContext(ctx, `SELECT revision,payload FROM moderation_records WHERE clan_tag=? ORDER BY created_at DESC,id`, tag)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := []moderationRecord{}
	for rows.Next() {
		var record moderationRecord
		var payload string
		var revision int64
		if err = rows.Scan(&revision, &payload); err != nil {
			break
		}
		if err = json.Unmarshal([]byte(payload), &record); err != nil {
			break
		}
		record.Revision = revision
		records = append(records, record)
	}
	if err != nil {
		return nil, err
	}
	return records, rows.Err()
}
func validModerationRecord(record *moderationRecord) bool {
	record.PlayerTag = "#" + normalizeTag(strings.TrimSpace(record.PlayerTag))
	record.PlayerName = strings.TrimSpace(record.PlayerName)
	record.Reason = strings.TrimSpace(record.Reason)
	if record.Sector == "" {
		record.Sector = automaticSector(record.Source)
	}
	if record.Sector != "" && !validModerationSector(record.Sector) {
		return false
	}
	return playerTagPattern.MatchString(record.PlayerTag) && validAdminText(record.PlayerName, 100, true) && validAdminText(record.Reason, 1500, true) && validAdminText(record.Action, 500, record.Kind == "punishment") && validAdminText(record.Notes, 2000, false) && (record.Kind == "warning" || record.Kind == "punishment") && (record.Status == "pending" || record.Status == "active" || record.Status == "justified" || record.Status == "resolved" || record.Status == "dismissed")
}
func insertModeration(ctx context.Context, db *database, tag string, record *moderationRecord) (bool, error) {
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return false, err
	}
	record.ID = hex.EncodeToString(id)
	record.Revision = 1
	if record.Source != "manual" {
		record.AutoRevision = 1
	}
	record.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	record.UpdatedAt = record.CreatedAt
	if record.Kind == "warning" && record.Status == "active" {
		record.IssuedAt = record.CreatedAt
	}
	key := record.EventKey
	if key == "" {
		key = "manual:" + record.ID
	}
	payload, _ := json.Marshal(record)
	result, err := db.ExecContext(ctx, `INSERT INTO moderation_records(id,clan_tag,source_key,revision,payload,created_at) VALUES(?,?,?,1,?,?) ON CONFLICT(clan_tag,source_key) DO NOTHING`, record.ID, tag, key, string(payload), record.CreatedAt)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}
func (a *app) saveModeration(w http.ResponseWriter, r *http.Request) {
	tag, ok := a.selectedClanTag(w, r)
	if !ok {
		return
	}
	var input struct {
		Revision   int64  `json:"revision"`
		PlayerTag  string `json:"playerTag"`
		PlayerName string `json:"playerName"`
		Kind       string `json:"kind"`
		Sector     string `json:"sector"`
		Sanction   string `json:"sanction"`
		Status     string `json:"status"`
		Reason     string `json:"reason"`
		Action     string `json:"action"`
		Notes      string `json:"notes"`
	}
	if !decodeAdminJSON(w, r, &input) {
		return
	}
	record := moderationRecord{Revision: input.Revision, PlayerTag: input.PlayerTag, PlayerName: input.PlayerName, Kind: input.Kind, Sector: input.Sector, Sanction: input.Sanction, Status: input.Status, Reason: input.Reason, Action: input.Action, Notes: input.Notes, Source: "manual"}
	if action, ok := moderationSanctions[record.Sanction]; ok && record.Kind == "punishment" {
		record.Action = action
	}
	if !validModerationRecord(&record) {
		jsonResponse(w, 400, map[string]string{"error": "Informe jogador, tag, motivo e situação válidos. Punições precisam da medida definida pela liderança."})
		return
	}
	id := chi.URLParam(r, "id")
	if id == "" {
		if !a.validateModerationPolicy(w, r, tag, &record, nil) {
			return
		}
		if _, err := insertModeration(r.Context(), a.db, tag, &record); err != nil {
			jsonResponse(w, 500, map[string]string{"error": "Não foi possível registrar o aviso."})
			return
		}
		jsonResponse(w, 201, record)
		return
	}
	var payload string
	var savedRevision int64
	if err := a.db.QueryRowContext(r.Context(), `SELECT revision,payload FROM moderation_records WHERE id=? AND clan_tag=?`, id, tag).Scan(&savedRevision, &payload); err != nil {
		jsonResponse(w, 404, map[string]string{"error": "Registro não encontrado neste clã."})
		return
	}
	if input.Revision != savedRevision {
		jsonResponse(w, 409, map[string]string{"error": "O registro mudou em outra sessão ou na conferência automática. Recarregue o histórico antes de salvar."})
		return
	}
	var saved moderationRecord
	if json.Unmarshal([]byte(payload), &saved) != nil {
		jsonResponse(w, 500, map[string]string{"error": "Registro indisponível."})
		return
	}
	record.ID = id
	record.Source = saved.Source
	record.EventKey = saved.EventKey
	record.EventStart = saved.EventStart
	record.EventEnd = saved.EventEnd
	record.CreatedAt = saved.CreatedAt
	record.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	record.IssuedAt, record.AppliedAt = saved.IssuedAt, saved.AppliedAt
	if record.Kind == "warning" && record.Status == "active" && record.IssuedAt == "" {
		record.IssuedAt = saved.CreatedAt
		if saved.Status == "pending" {
			record.IssuedAt = record.UpdatedAt
		}
	}
	if record.Sector == "" {
		record.Sector = automaticSector(saved.Source)
	}
	if saved.Source != "manual" && (saved.PlayerTag != record.PlayerTag || saved.PlayerName != record.PlayerName || saved.Reason != record.Reason || saved.Kind != record.Kind || record.Sector != automaticSector(saved.Source)) {
		jsonResponse(w, 400, map[string]string{"error": "A evidência automática é preservada. Use observações para registrar correções e justificativas."})
		return
	}
	if !a.validateModerationPolicy(w, r, tag, &record, &saved) {
		return
	}
	payloadBytes, _ := json.Marshal(record)
	result, err := a.db.ExecContext(r.Context(), `UPDATE moderation_records SET payload=?,revision=revision+1 WHERE id=? AND clan_tag=? AND revision=?`, string(payloadBytes), id, tag, input.Revision)
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": "Não foi possível atualizar o registro."})
		return
	}
	changed, err := result.RowsAffected()
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": "Não foi possível confirmar a atualização."})
		return
	}
	if changed != 1 {
		jsonResponse(w, 409, map[string]string{"error": "O registro mudou em outra sessão. Recarregue o histórico antes de salvar."})
		return
	}
	record.Revision++
	jsonResponse(w, 200, record)
}
func (a *app) saveModerationSettings(w http.ResponseWriter, r *http.Request) {
	tag, ok := a.selectedClanTag(w, r)
	if !ok {
		return
	}
	var input struct {
		Revision  int64 `json:"revision"`
		Automatic bool  `json:"automatic"`
	}
	if !decodeAdminJSON(w, r, &input) {
		return
	}
	if !a.moderationMu.TryLock() {
		jsonResponse(w, 409, map[string]string{"error": "Há uma consulta automática em andamento. Tente novamente em instantes."})
		return
	}
	defer a.moderationMu.Unlock()
	s, err := readModerationSettings(r.Context(), a.db, tag)
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": "Não foi possível ler a configuração."})
		return
	}
	if s.Revision != input.Revision {
		jsonResponse(w, 409, map[string]string{"error": "A configuração mudou em outra sessão. Recarregue antes de salvar."})
		return
	}

	s.Automatic = input.Automatic
	payload, _ := json.Marshal(s)
	result, err := a.db.ExecContext(r.Context(), `INSERT INTO moderation_settings(clan_tag,revision,payload) VALUES(?,?,?) ON CONFLICT(clan_tag) DO UPDATE SET revision=moderation_settings.revision+1,payload=excluded.payload WHERE moderation_settings.revision=?`, tag, 2, string(payload), input.Revision)
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": "Não foi possível salvar a automação."})
		return
	}
	changed, err := result.RowsAffected()
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": "Não foi possível confirmar a configuração."})
		return
	}
	if changed != 1 {
		jsonResponse(w, 409, map[string]string{"error": "A configuração mudou em outra sessão. Recarregue antes de salvar."})
		return
	}
	jsonResponse(w, 200, map[string]bool{"saved": true})
}

type moderationMember struct {
	Tag     string            `json:"tag"`
	Name    string            `json:"name"`
	Attacks []json.RawMessage `json:"attacks"`
}
type moderationSide struct {
	Tag     string             `json:"tag"`
	Attacks *int               `json:"attacks"`
	Members []moderationMember `json:"members"`
}
type moderationWar struct {
	State            string         `json:"state"`
	TeamSize         int            `json:"teamSize"`
	StartTime        string         `json:"startTime"`
	EndTime          string         `json:"endTime"`
	AttacksPerMember int            `json:"attacksPerMember"`
	Clan             moderationSide `json:"clan"`
	Opponent         moderationSide `json:"opponent"`
}

func closedSince(end string, since, now time.Time) bool {
	date, err := time.Parse("20060102T150405.000Z", end)
	return err == nil && !date.Before(since) && !date.After(now)
}
func warWarnings(body []byte, tag, source, event string, since, now time.Time) []moderationRecord {
	var war moderationWar
	if json.Unmarshal(body, &war) != nil || war.State != "warEnded" || !finishedPeriod(war.StartTime, war.EndTime, since, now) {
		return nil
	}
	side, limit, complete := finalWarSide(war, tag, source)
	if !complete {
		return nil
	}
	records := []moderationRecord{}
	for _, m := range side.Members {
		if len(m.Attacks) != 0 {
			continue
		}
		record := moderationRecord{PlayerTag: m.Tag, PlayerName: m.Name, Kind: "warning", Status: "active", Source: source, EventKey: source + ":" + event + ":" + normalizeTag(m.Tag), Reason: fmt.Sprintf("%s encerrada em %s: %d/%d ataques usados.", map[string]string{"war": "Guerra", "cwl": "CWL"}[source], war.EndTime, len(m.Attacks), limit)}
		record.EventStart, record.EventEnd = war.StartTime, war.EndTime
		if validModerationRecord(&record) {
			records = append(records, record)
		}
	}
	return records
}
func raidWarnings(body []byte, rosters map[string][]raidRosterMember, since, now time.Time) []moderationRecord {
	var response struct {
		Items []moderationRaidSeason `json:"items"`
	}
	if json.Unmarshal(body, &response) != nil {
		return nil
	}
	records := []moderationRecord{}
	for _, season := range response.Items {
		if season.State != "ended" || !finishedPeriod(season.StartTime, season.EndTime, since, now) || !finalRaidMembers(season) {
			continue
		}
		seen := map[string]bool{}
		for _, m := range season.Members {
			seen[normalizeTag(m.Tag)] = true
		}
		for _, member := range rosters[season.StartTime] {
			if seen[normalizeTag(member.Tag)] {
				continue
			}
			record := moderationRecord{PlayerTag: member.Tag, PlayerName: member.Name, Kind: "warning", Status: "pending", Source: "raids", EventKey: "raids:" + season.StartTime + ":" + normalizeTag(member.Tag), Reason: "Nenhum ataque registrado nas raids de " + season.StartTime + ". Membro na lista capturada durante o evento; confirmar elegibilidade e participação em outro clã."}
			record.EventStart, record.EventEnd = season.StartTime, season.EndTime
			if validModerationRecord(&record) {
				records = append(records, record)
			}
		}
		for _, m := range season.Members {
			if m.AttackLimit == nil || *m.AttackLimit < 1 || *m.AttackLimit > 6 || m.Bonus < 0 || m.Bonus > 1 || m.Attacks == nil || *m.Attacks < 0 {
				continue
			}
			limit := *m.AttackLimit + m.Bonus
			if limit > 6 || *m.Attacks >= limit {
				continue
			}
			record := moderationRecord{PlayerTag: m.Tag, PlayerName: m.Name, Kind: "warning", Status: "pending", Source: "raids", EventKey: "raids:" + season.StartTime + ":" + normalizeTag(m.Tag), Reason: fmt.Sprintf("Raids encerradas em %s: %d/%d ataques disponíveis usados. Conferir elegibilidade e participação em outro clã.", season.EndTime, *m.Attacks, limit)}
			record.EventStart, record.EventEnd = season.StartTime, season.EndTime
			if validModerationRecord(&record) {
				records = append(records, record)
			}
		}
	}
	return records
}
func (a *app) collectWarnings(ctx context.Context, tag string) (int, []string, error) {
	s, err := readModerationSettings(ctx, a.db, tag)
	if err != nil {
		return 0, nil, err
	}
	since, err := time.Parse(time.RFC3339, s.Since)
	if err != nil {
		return 0, nil, err
	}
	now := time.Now().UTC()
	records := []moderationRecord{}
	counts := map[string]finalAttackCount{}
	problems := []string{}
	_, _, memberErr := a.trackClanMembers(ctx, tag)
	members, historyErr := a.memberObservations(ctx, tag, now)
	if memberErr != nil || historyErr != nil {
		problems = append(problems, "Entradas de membros indisponíveis: novas ocorrências ficam a conferir, sem contar como avisos ativos.")
	}
	base := "/clans/" + url.PathEscape("#"+tag)
	body, status, err := a.fetchClash(ctx, base+"/currentwar")
	if err == nil {
		var w moderationWar
		if json.Unmarshal(body, &w) == nil && w.State != "" {
			checkedAt := minSnapshotTime(now, a.clashFetchedAt(base+"/currentwar"))
			records = append(records, warWarnings(body, tag, "war", w.EndTime, since, checkedAt)...)
			finalWarCounts(body, tag, "war", w.EndTime, since, checkedAt, counts)
			if w.State == "warEnded" && finishedPeriod(w.StartTime, w.EndTime, since, now) {
				if _, _, ok := finalWarSide(w, tag, "war"); !ok {
					problems = append(problems, "Guerra encerrada: escalação ou contagem final incompleta; nenhum aviso gerado.")
				}
			}
		} else {
			problems = append(problems, "Resposta inválida da guerra.")
		}
	} else if status != 404 {
		problems = append(problems, "Guerra: "+err.Error())
	}
	body, _, err = a.fetchClash(ctx, base+"/capitalraidseasons?limit=10")
	if err == nil {
		a.captureModerationRaidRoster(ctx, tag, body)
		saved, savedErr := a.savedRaids(ctx, tag)
		rosters := map[string][]raidRosterMember{}
		if savedErr == nil {
			for _, r := range saved {
				if r.RosterDuringRaid {
					var season struct {
						StartTime string `json:"startTime"`
						EndTime   string `json:"endTime"`
					}
					if json.Unmarshal(r.Season, &season) == nil {
						start, e1 := time.Parse("20060102T150405.000Z", season.StartTime)
						end, e2 := time.Parse("20060102T150405.000Z", season.EndTime)
						captured, e3 := time.Parse(time.RFC3339, r.RosterCapturedAt)
						if e1 == nil && e2 == nil && e3 == nil && !captured.Before(start) && captured.Before(end) {
							rosters[season.StartTime] = r.Roster
						}
					}
				}
			}
		} else {
			problems = append(problems, "Não foi possível ler as listas históricas de raids.")
		}
		var raidData struct {
			Items []moderationRaidSeason `json:"items"`
		}
		if json.Unmarshal(body, &raidData) != nil || raidData.Items == nil {
			problems = append(problems, "Resposta inválida das raids.")
		} else {
			for _, season := range raidData.Items {
				if season.State == "ended" && finishedPeriod(season.StartTime, season.EndTime, since, now) {
					if !finalRaidMembers(season) {
						problems = append(problems, "Raid "+season.StartTime+": contagem final incompleta; nenhum aviso gerado.")
						continue
					}
					if rosters[season.StartTime] == nil {
						problems = append(problems, "Raid "+season.StartTime+": sem lista capturada durante o evento; ausentes precisam ser conferidos manualmente.")
					}
				}
			}
		}
		checkedAt := minSnapshotTime(now, a.clashFetchedAt(base+"/capitalraidseasons?limit=10"))
		records = append(records, raidWarnings(body, rosters, since, checkedAt)...)
		finalRaidCounts(body, since, checkedAt, counts)
	} else {
		problems = append(problems, "Raids: "+err.Error())
	}
	_, group, status, err := a.fetchLeagueGroup(ctx, tag)
	if err != nil && status != 404 {
		problems = append(problems, "CWL: "+err.Error())
	}
	if err == nil {
		tags := map[string]bool{}
		for _, round := range group.Rounds {
			for _, warTag := range round.WarTags {
				key := normalizeTag(warTag)
				if key != "" && key != "0" {
					tags[key] = true
				}
			}
		}
		if len(tags) > 64 {
			problems = append(problems, "Quantidade inválida de guerras da CWL.")
		} else {
			for warTag := range tags {
				body, _, err := a.fetchClash(ctx, "/clanwarleagues/wars/"+url.PathEscape("#"+warTag))
				if err != nil {
					problems = append(problems, "Não foi possível consultar uma rodada da CWL.")
					continue
				}
				checkedAt := minSnapshotTime(now, a.clashFetchedAt("/clanwarleagues/wars/"+url.PathEscape("#"+warTag)))
				records = append(records, warWarnings(body, tag, "cwl", warTag, since, checkedAt)...)
				finalWarCounts(body, tag, "cwl", warTag, since, checkedAt, counts)
			}
		}
	}
	records = membershipWarnings(records, members, memberErr == nil && historyErr == nil)
	conflicts, err := reconcileAutomaticWarnings(ctx, a.db, tag, records, counts)
	if err != nil {
		return 0, problems, err
	}
	if conflicts > 0 {
		problems = append(problems, fmt.Sprintf("%d ocorrência(s) já revisada(s) pela liderança divergem da contagem final. Confira o histórico; as decisões foram preservadas.", conflicts))
	}
	count := 0
	for i := range records {
		added, err := insertModeration(ctx, a.db, tag, &records[i])
		if err != nil {
			return count, problems, err
		}
		if added {
			count++
		}
	}
	s.LastCheck = now.Format(time.RFC3339)
	s.Problems = problems
	payload, _ := json.Marshal(s)
	_, err = a.db.ExecContext(ctx, `INSERT INTO moderation_settings(clan_tag,revision,payload) VALUES(?,1,?) ON CONFLICT(clan_tag) DO UPDATE SET payload=excluded.payload WHERE moderation_settings.revision=?`, tag, string(payload), s.Revision)
	return count, problems, err
}
func (a *app) syncModeration(w http.ResponseWriter, r *http.Request) {
	tag, ok := a.selectedClanTag(w, r)
	if !ok {
		return
	}
	var input struct{}
	if !decodeAdminJSON(w, r, &input) {
		return
	}
	if !a.moderationMu.TryLock() {
		jsonResponse(w, 409, map[string]string{"error": "Já existe uma consulta em andamento. Aguarde."})
		return
	}
	defer a.moderationMu.Unlock()
	count, problems, err := a.collectWarnings(r.Context(), tag)
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": "Não foi possível concluir a geração de avisos."})
		return
	}
	jsonResponse(w, 200, map[string]any{"created": count, "problems": problems})
}
func (a *app) runAutomaticWarnings() {
	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()
	for {
		<-timer.C
		if a.moderationMu.TryLock() {
			for _, tag := range a.clanTags {
				if tag == "" || a.token == "" {
					continue
				}
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				s, err := readModerationSettings(ctx, a.db, tag)
				if err == nil && s.Automatic {
					a.collectWarnings(ctx, tag)
				} else {
					a.trackClanMembers(ctx, tag)
				}
				cancel()
			}
			a.moderationMu.Unlock()
		}
		timer.Reset(15 * time.Minute)
	}
}

func (a *app) captureModerationRaidRoster(ctx context.Context, tag string, body []byte) {
	var response struct {
		Items []json.RawMessage `json:"items"`
	}
	if json.Unmarshal(body, &response) != nil {
		return
	}
	base := "/clans/" + url.PathEscape("#"+tag)
	for _, raw := range response.Items {
		var season struct {
			StartTime string `json:"startTime"`
			EndTime   string `json:"endTime"`
		}
		if json.Unmarshal(raw, &season) != nil {
			continue
		}
		start, e1 := time.Parse("20060102T150405.000Z", season.StartTime)
		end, e2 := time.Parse("20060102T150405.000Z", season.EndTime)
		if e1 != nil || e2 != nil || time.Now().Before(start) || !time.Now().Before(end) {
			continue
		}
		clanBody, _, err := a.fetchClash(ctx, base)
		if err != nil {
			return
		}
		var clan struct {
			MemberList []raidRosterMember `json:"memberList"`
		}
		if json.Unmarshal(clanBody, &clan) != nil {
			return
		}
		at := a.clashFetchedAt(base)
		if at.Before(start) || !at.Before(end) {
			continue
		}
		roster, _ := json.Marshal(clan.MemberList)
		a.db.ExecContext(ctx, `INSERT INTO raid_reports(clan_tag,start_time,season,roster,roster_captured_at,roster_during_raid,synced_at) VALUES(?,?,?,?,?,1,?) ON CONFLICT(clan_tag,start_time) DO UPDATE SET season=excluded.season,synced_at=excluded.synced_at,roster=CASE WHEN raid_reports.roster_during_raid=0 AND excluded.roster_during_raid=1 THEN excluded.roster ELSE raid_reports.roster END,roster_captured_at=CASE WHEN raid_reports.roster_during_raid=0 AND excluded.roster_during_raid=1 THEN excluded.roster_captured_at ELSE raid_reports.roster_captured_at END,roster_during_raid=CASE WHEN excluded.roster_during_raid=1 THEN 1 ELSE raid_reports.roster_during_raid END`, tag, season.StartTime, string(raw), string(roster), at.Format(time.RFC3339), at.Format(time.RFC3339))
	}
}
