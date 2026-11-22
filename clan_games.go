package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

type clanGamesEvent struct {
	Season         string `json:"season"`
	Revision       int64  `json:"revision"`
	StartTime      string `json:"startTime"`
	EndTime        string `json:"endTime"`
	MemberTarget   int64  `json:"memberTarget"`
	ElderTarget    int64  `json:"elderTarget"`
	CoLeaderTarget int64  `json:"coLeaderTarget"`
	LeaderTarget   int64  `json:"leaderTarget"`
	ClanTarget     int64  `json:"clanTarget"`
	PlayerCap      int64  `json:"playerCap"`
	State          string `json:"state"`
	Finalized      bool   `json:"finalized"`
}

type clanGamesSnapshot struct {
	Tag           string `json:"tag"`
	Name          string `json:"name"`
	Role          string `json:"role"`
	TownHallLevel int    `json:"townHallLevel"`
	Current       bool   `json:"current"`
	Baseline      *int64 `json:"baseline"`
	Latest        *int64 `json:"latest"`
	BaselineAt    string `json:"baselineAt"`
	UpdatedAt     string `json:"updatedAt"`
	FirstSeen     string `json:"firstSeen"`
	ReviewReason  string `json:"reviewReason"`
	Failure       string `json:"failure"`
	Final         bool   `json:"final"`
}

type clanGamesDocument struct {
	Event    clanGamesEvent                `json:"event"`
	Members  map[string]*clanGamesSnapshot `json:"members"`
	RosterAt string                        `json:"rosterAt"`
	SyncedAt string                        `json:"syncedAt"`
	Problems []string                      `json:"problems"`
}

type clanGamesRow struct {
	Tag           string `json:"tag"`
	Name          string `json:"name"`
	Role          string `json:"role"`
	TownHallLevel int    `json:"townHallLevel"`
	Current       bool   `json:"current"`
	Points        *int64 `json:"points"`
	Target        int64  `json:"target"`
	Status        string `json:"status"`
	Reason        string `json:"reason"`
	BaselineAt    string `json:"baselineAt"`
	UpdatedAt     string `json:"updatedAt"`
	Partial       bool   `json:"partial"`
}

type clanGamesSummary struct {
	Points       int64 `json:"points"`
	Participants int   `json:"participants"`
	Completed    int   `json:"completed"`
	Zero         int   `json:"zero"`
	Review       int   `json:"review"`
	Eligible     int   `json:"eligible"`
}

type clanGamesResponse struct {
	Event    clanGamesEvent   `json:"event"`
	Seasons  []string         `json:"seasons"`
	Items    []clanGamesRow   `json:"items"`
	Summary  clanGamesSummary `json:"summary"`
	SyncedAt string           `json:"syncedAt"`
	Problems []string         `json:"problems"`
}

type clanGamesRosterMember struct {
	Tag           string `json:"tag"`
	Name          string `json:"name"`
	Role          string `json:"role"`
	TownHallLevel int    `json:"townHallLevel"`
}

type clanGamesProfile struct {
	Tag           string `json:"tag"`
	Name          string `json:"name"`
	TownHallLevel int    `json:"townHallLevel"`
	Clan          struct {
		Tag string `json:"tag"`
	} `json:"clan"`
	Achievements []struct {
		Name  string `json:"name"`
		Value *int64 `json:"value"`
	} `json:"achievements"`
}

var errClanGamesConflict = errors.New("Os jogos foram atualizados por outra consulta. Recarregue os dados e tente novamente.")

func initClanGames(db *database) error {
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
	if _, err = tx.Exec(`CREATE TABLE IF NOT EXISTS clan_games_events(clan_tag TEXT NOT NULL,season TEXT NOT NULL,revision BIGINT NOT NULL,payload TEXT NOT NULL,PRIMARY KEY(clan_tag,season))`); err != nil {
		return err
	}
	return tx.Commit()
}

func clanGamesSeason(value string) (time.Time, error) {
	month, err := time.Parse("2006-01", value)
	if err != nil || month.Year() < 2020 || month.Year() > 2100 {
		return time.Time{}, fmt.Errorf("Escolha uma temporada no formato AAAA-MM, entre 2020 e 2100.")
	}
	return month, nil
}

func defaultClanGamesEvent(season string) (clanGamesEvent, error) {
	month, err := clanGamesSeason(season)
	if err != nil {
		return clanGamesEvent{}, err
	}
	return clanGamesEvent{
		Season: season, Revision: 1,
		StartTime:    time.Date(month.Year(), month.Month(), 22, 8, 0, 0, 0, time.UTC).Format(time.RFC3339),
		EndTime:      time.Date(month.Year(), month.Month(), 28, 8, 0, 0, 0, time.UTC).Format(time.RFC3339),
		MemberTarget: 4000, ElderTarget: 4000, CoLeaderTarget: 4000, LeaderTarget: 4000, ClanTarget: 50000, PlayerCap: 4000,
	}, nil
}

func readClanGamesDocument(ctx context.Context, db *database, tag, season string) (clanGamesDocument, int64, error) {
	event, err := defaultClanGamesEvent(season)
	if err != nil {
		return clanGamesDocument{}, 0, err
	}
	doc := clanGamesDocument{Event: event, Members: map[string]*clanGamesSnapshot{}, Problems: []string{}}
	var payload string
	var revision int64
	err = db.QueryRowContext(ctx, `SELECT revision,payload FROM clan_games_events WHERE clan_tag=? AND season=?`, tag, season).Scan(&revision, &payload)
	if err == sql.ErrNoRows {
		return doc, 0, nil
	}
	if err != nil {
		return doc, 0, err
	}
	if err = json.Unmarshal([]byte(payload), &doc); err != nil {
		return doc, 0, err
	}
	doc.Event.Revision = revision
	if doc.Members == nil {
		doc.Members = map[string]*clanGamesSnapshot{}
	}
	if doc.Problems == nil {
		doc.Problems = []string{}
	}
	return doc, revision, nil
}

func writeClanGamesDocument(ctx context.Context, db *database, tag string, doc *clanGamesDocument, expected int64) error {
	if expected < 0 || expected == math.MaxInt64 {
		return errClanGamesConflict
	}
	doc.Event.Revision = expected + 1
	payload, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	var result sql.Result
	if expected == 0 {
		result, err = db.ExecContext(ctx, `INSERT INTO clan_games_events(clan_tag,season,revision,payload) VALUES(?,?,1,?) ON CONFLICT(clan_tag,season) DO NOTHING`, tag, doc.Event.Season, string(payload))
	} else {
		result, err = db.ExecContext(ctx, `UPDATE clan_games_events SET revision=?,payload=? WHERE clan_tag=? AND season=? AND revision=?`, doc.Event.Revision, string(payload), tag, doc.Event.Season, expected)
	}
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err == nil && changed != 1 {
		return errClanGamesConflict
	}
	return err
}

func clanGamesTimes(event clanGamesEvent) (time.Time, time.Time) {
	start, _ := time.Parse(time.RFC3339, event.StartTime)
	end, _ := time.Parse(time.RFC3339, event.EndTime)
	return start, end
}

func clanGamesTarget(event clanGamesEvent, role string) int64 {
	switch role {
	case "leader":
		return event.LeaderTarget
	case "coLeader":
		return event.CoLeaderTarget
	case "admin":
		return event.ElderTarget
	default:
		return event.MemberTarget
	}
}

func clanGamesMemberRow(event clanGamesEvent, member *clanGamesSnapshot, now time.Time) clanGamesRow {
	row := clanGamesRow{Tag: member.Tag, Name: member.Name, Role: member.Role, TownHallLevel: member.TownHallLevel, Current: member.Current, Target: clanGamesTarget(event, member.Role), Status: "review", BaselineAt: member.BaselineAt, UpdatedAt: member.UpdatedAt}
	start, end := clanGamesTimes(event)
	baselineAt, _ := time.Parse(time.RFC3339Nano, member.BaselineAt)
	updatedAt, _ := time.Parse(time.RFC3339Nano, member.UpdatedAt)
	if member.Baseline != nil && member.Latest != nil && *member.Latest >= *member.Baseline {
		points := *member.Latest - *member.Baseline
		row.Points = &points
	}
	if member.TownHallLevel > 0 && member.TownHallLevel < 6 {
		row.Status, row.Reason, row.Points = "ineligible", "Centro de Vila abaixo do nível 6.", nil
		return row
	}
	if now.Before(start) {
		row.Points = nil
		row.Status = "upcoming"
		row.Reason = "Aguardando o início dos jogos. A referência é atualizada automaticamente."
		if member.Baseline == nil || member.Failure != "" || !member.Current {
			row.Status, row.Partial = "review", true
			row.Reason = "Referência inicial ainda não confirmada para este clã."
			if member.Failure != "" {
				row.Reason = member.Failure
			}
		}
		return row
	}
	row.Partial = true
	switch {
	case member.Failure != "":
		row.Reason = member.Failure
	case member.ReviewReason != "":
		row.Reason = member.ReviewReason
	case member.TownHallLevel < 6:
		row.Reason = "Elegibilidade ainda não confirmada."
	case !member.Current && !member.Final:
		row.Reason = "Membro fora do clã; confira em qual clã participou dos jogos."
	case member.Baseline == nil || baselineAt.IsZero():
		row.Reason = "Sem referência inicial. Não é possível reconstruir os pontos anteriores."
	case !baselineAt.Before(start):
		row.Reason = "Acompanhamento iniciado após o início; os pontos são somente o aumento observado."
	case member.Latest == nil || updatedAt.Before(start):
		row.Reason = "Aguardando a primeira consulta após o início dos jogos."
	case row.Points == nil:
		row.Reason = "O contador diminuiu; confira a referência antes de atribuir pontos."
	case *row.Points > event.PlayerCap:
		row.Reason = "Aumento acima do limite configurado; confira o limite e a participação no período."
	case now.Before(end) && now.Sub(updatedAt) > 45*time.Minute:
		row.Reason = "Dados desatualizados; a ausência de novos pontos ainda não foi confirmada."
	case !now.Before(end) && !member.Final:
		row.Reason = "Resultado provisório; falta uma consulta válida após o encerramento."
	default:
		row.Partial = false
		row.Status = "zero"
		row.Reason = "Nenhum aumento de pontos registrado neste período."
		if *row.Points >= row.Target {
			row.Status, row.Reason = "completed", "Meta atingida."
		} else if *row.Points > 0 {
			row.Status, row.Reason = "contributing", "Contribuiu e ainda não atingiu a meta."
		}
	}
	return row
}

func clanGamesView(doc clanGamesDocument, now time.Time) clanGamesResponse {
	response := clanGamesResponse{Event: doc.Event, Items: []clanGamesRow{}, Seasons: []string{doc.Event.Season}, SyncedAt: doc.SyncedAt, Problems: doc.Problems}
	start, end := clanGamesTimes(doc.Event)
	response.Event.State = "active"
	if now.Before(start) {
		response.Event.State = "upcoming"
	} else if !now.Before(end) {
		response.Event.State = "ended"
	}
	response.Event.Finalized = !now.Before(end) && len(doc.Members) > 0
	for _, member := range doc.Members {
		row := clanGamesMemberRow(doc.Event, member, now)
		response.Items = append(response.Items, row)
		if row.Current && row.TownHallLevel >= 6 {
			response.Summary.Eligible++
		}
		if row.Status == "review" {
			response.Summary.Review++
		}
		if !member.Final && row.Status != "ineligible" {
			response.Event.Finalized = false
		}
		if row.Status == "completed" || row.Status == "contributing" || row.Status == "zero" {
			response.Summary.Points += *row.Points
			if *row.Points > 0 {
				response.Summary.Participants++
			}
			if row.Status == "completed" {
				response.Summary.Completed++
			} else if row.Status == "zero" {
				response.Summary.Zero++
			}
		}
	}
	sort.Slice(response.Items, func(i, j int) bool {
		if response.Items[i].Current != response.Items[j].Current {
			return response.Items[i].Current
		}
		return strings.ToLower(response.Items[i].Name) < strings.ToLower(response.Items[j].Name)
	})
	return response
}

func (a *app) clanGamesResponse(ctx context.Context, tag, season string, now time.Time) (clanGamesResponse, error) {
	doc, _, err := readClanGamesDocument(ctx, a.db, tag, season)
	if err != nil {
		return clanGamesResponse{}, err
	}
	response := clanGamesView(doc, now)
	rows, err := a.db.QueryContext(ctx, `SELECT season FROM clan_games_events WHERE clan_tag=? ORDER BY season DESC LIMIT 24`, tag)
	if err != nil {
		return response, err
	}
	defer rows.Close()
	seasons := map[string]bool{season: true}
	for rows.Next() {
		var value string
		if err = rows.Scan(&value); err != nil {
			return response, err
		}
		seasons[value] = true
	}
	response.Seasons = []string{}
	for value := range seasons {
		response.Seasons = append(response.Seasons, value)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(response.Seasons)))
	return response, rows.Err()
}

func (a *app) clanGames(w http.ResponseWriter, r *http.Request) {
	tag, ok := a.selectedClanTag(w, r)
	if !ok {
		return
	}
	season := r.URL.Query().Get("season")
	if season == "" {
		season = time.Now().UTC().Format("2006-01")
	}
	if _, err := clanGamesSeason(season); err != nil {
		jsonResponse(w, 400, map[string]string{"error": err.Error()})
		return
	}
	response, err := a.clanGamesResponse(r.Context(), tag, season, time.Now().UTC())
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": "Não foi possível carregar os jogos do clã."})
		return
	}
	jsonResponse(w, 200, response)
}

func validateClanGamesSettings(event clanGamesEvent) error {
	month, err := clanGamesSeason(event.Season)
	if err != nil {
		return err
	}
	start, startErr := time.Parse(time.RFC3339, event.StartTime)
	end, endErr := time.Parse(time.RFC3339, event.EndTime)
	if startErr != nil || endErr != nil || start.Before(month) || !end.Before(month.AddDate(0, 1, 0)) || !end.After(start) || end.Sub(start) > 14*24*time.Hour {
		return fmt.Errorf("Informe início e fim válidos na mesma temporada, com duração máxima de 14 dias.")
	}
	if event.Revision < 1 || event.Revision == math.MaxInt64 || event.PlayerCap < 1 || event.PlayerCap > 100000 || event.ClanTarget < 1 || event.ClanTarget > event.PlayerCap*50 {
		return fmt.Errorf("Informe um limite individual de 1 a 100.000 e uma meta do clã de até 50 vezes esse limite.")
	}
	for _, target := range []int64{event.MemberTarget, event.ElderTarget, event.CoLeaderTarget, event.LeaderTarget} {
		if target < 1 || target > event.PlayerCap {
			return fmt.Errorf("As metas por cargo devem estar entre 1 e o limite individual do evento.")
		}
	}
	return nil
}

func (a *app) saveClanGamesSettings(w http.ResponseWriter, r *http.Request) {
	tag, ok := a.selectedClanTag(w, r)
	if !ok {
		return
	}
	var event clanGamesEvent
	if !decodeAdminJSON(w, r, &event) {
		return
	}
	if err := validateClanGamesSettings(event); err != nil {
		jsonResponse(w, 400, map[string]string{"error": err.Error()})
		return
	}
	if !a.clanGamesMu.TryLock() {
		jsonResponse(w, 409, map[string]string{"error": "A sincronização dos jogos está em andamento. Aguarde."})
		return
	}
	defer a.clanGamesMu.Unlock()
	doc, revision, err := readClanGamesDocument(r.Context(), a.db, tag, event.Season)
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": "Não foi possível ler a configuração dos jogos."})
		return
	}
	if event.Revision != doc.Event.Revision {
		jsonResponse(w, 409, map[string]string{"error": errClanGamesConflict.Error()})
		return
	}
	oldStart, oldEnd := clanGamesTimes(doc.Event)
	newStart, newEnd := clanGamesTimes(event)
	if (!newStart.Equal(oldStart) || !newEnd.Equal(oldEnd)) && (!time.Now().UTC().Before(oldStart) || !time.Now().UTC().Before(newStart)) {
		jsonResponse(w, 400, map[string]string{"error": "As datas só podem mudar antes do início dos jogos. O histórico registrado foi preservado."})
		return
	}
	event.StartTime = newStart.UTC().Format(time.RFC3339)
	event.EndTime = newEnd.UTC().Format(time.RFC3339)
	event.State, event.Finalized = "", false
	doc.Event = event
	if err = writeClanGamesDocument(r.Context(), a.db, tag, &doc, revision); err != nil {
		status := 500
		if errors.Is(err, errClanGamesConflict) {
			status = 409
		}
		jsonResponse(w, status, map[string]string{"error": "Não foi possível salvar. " + err.Error()})
		return
	}
	jsonResponse(w, 200, clanGamesView(doc, time.Now().UTC()))
}

func applyClanGamesRoster(doc *clanGamesDocument, roster []clanGamesRosterMember, at time.Time) {
	previous, _ := time.Parse(time.RFC3339Nano, doc.RosterAt)
	if !previous.IsZero() && !at.After(previous) {
		return
	}
	start, _ := clanGamesTimes(doc.Event)
	seen := map[string]bool{}
	for _, person := range roster {
		person.Tag = "#" + normalizeTag(strings.ToUpper(person.Tag))
		seen[person.Tag] = true
		member := doc.Members[person.Tag]
		if member == nil {
			member = &clanGamesSnapshot{Tag: person.Tag, FirstSeen: at.UTC().Format(time.RFC3339Nano)}
			if !at.Before(start) {
				member.ReviewReason = "Entrada observada após o início; confira a participação e os pontos anteriores."
			}
			doc.Members[person.Tag] = member
		} else if !member.Current && !at.Before(start) && !member.Final {
			member.ReviewReason = "Retorno ao clã durante o período; confira onde participou dos jogos."
		}
		if !member.Final {
			member.Name, member.Role, member.TownHallLevel = person.Name, person.Role, person.TownHallLevel
		}
		member.Current = true
	}
	for tag, member := range doc.Members {
		if seen[tag] {
			continue
		}
		if member.Current && !at.Before(start) && !member.Final {
			member.ReviewReason = "Saída do clã observada durante o período; confira a participação."
		}
		if at.Before(start) {
			member.Baseline, member.Latest = nil, nil
			member.BaselineAt, member.UpdatedAt = "", ""
		}
		member.Current = false
	}
	doc.RosterAt = at.UTC().Format(time.RFC3339Nano)
}

func applyClanGamesProfile(doc *clanGamesDocument, tag string, profile clanGamesProfile, clanTag string, at, floor, cutoff time.Time) error {
	member := doc.Members[tag]
	if member == nil || member.Final {
		return nil
	}
	if normalizeTag(strings.ToUpper(profile.Tag)) != normalizeTag(strings.ToUpper(tag)) || at.IsZero() || at.Before(floor) || !at.Before(cutoff) {
		return fmt.Errorf("Perfil ou horário da consulta inválido; os pontos anteriores foram preservados.")
	}
	previous, _ := time.Parse(time.RFC3339Nano, member.UpdatedAt)
	if !previous.IsZero() && !at.After(previous) {
		return nil
	}
	var value *int64
	for _, achievement := range profile.Achievements {
		if achievement.Name == "Games Champion" {
			if value != nil || achievement.Value == nil || *achievement.Value < 0 {
				return fmt.Errorf("Contador Games Champion inválido; a referência foi preservada.")
			}
			value = achievement.Value
		}
	}
	if value == nil {
		return fmt.Errorf("Contador Games Champion indisponível no perfil.")
	}
	start, end := clanGamesTimes(doc.Event)
	if normalizeTag(strings.ToUpper(profile.Clan.Tag)) != normalizeTag(strings.ToUpper(clanTag)) {
		if !at.Before(start) {
			member.ReviewReason = "O perfil indica outro clã ou nenhum clã; confira a participação."
		}
		return fmt.Errorf("O perfil não confirma a presença neste clã.")
	}
	if member.Latest != nil && *value < *member.Latest && !at.Before(start) {
		member.ReviewReason = "O contador diminuiu; confira a referência antes de atribuir pontos."
	}
	if at.Before(start) || member.Baseline == nil {
		baseline := *value
		member.Baseline = &baseline
		member.BaselineAt = at.UTC().Format(time.RFC3339Nano)
		rosterAt, _ := time.Parse(time.RFC3339Nano, doc.RosterAt)
		if at.Before(start) && rosterAt.Before(start) {
			member.ReviewReason = ""
		}
	}
	latest := *value
	member.Latest = &latest
	member.UpdatedAt = at.UTC().Format(time.RFC3339Nano)
	member.TownHallLevel = profile.TownHallLevel
	member.Failure = ""
	if profile.Name != "" {
		member.Name = profile.Name
	}
	if !at.Before(end.Add(10 * time.Minute)) {
		member.Final = true
	}
	return nil
}

func (a *app) clanGamesWindow(ctx context.Context, tag string, event clanGamesEvent) (time.Time, time.Time, error) {
	month, _ := clanGamesSeason(event.Season)
	previous, _, err := readClanGamesDocument(ctx, a.db, tag, month.AddDate(0, -1, 0).Format("2006-01"))
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	next, _, err := readClanGamesDocument(ctx, a.db, tag, month.AddDate(0, 1, 0).Format("2006-01"))
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	_, floor := clanGamesTimes(previous.Event)
	cutoff, _ := clanGamesTimes(next.Event)
	return floor.Add(10 * time.Minute), cutoff, nil
}

func (a *app) saveCollectedClanGames(ctx context.Context, tag string, doc *clanGamesDocument, revision int64) error {
	if ctx.Err() != nil {
		doc.Problems = append(doc.Problems, "A consulta atingiu o prazo ou foi interrompida. Os dados incompletos precisam de nova sincronização.")
	}
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return writeClanGamesDocument(saveCtx, a.db, tag, doc, revision)
}

func (a *app) collectClanGames(ctx context.Context, tag, season string, now time.Time) error {
	doc, revision, err := readClanGamesDocument(ctx, a.db, tag, season)
	if err != nil {
		return err
	}
	floor, cutoff, err := a.clanGamesWindow(ctx, tag, doc.Event)
	if err != nil {
		return err
	}
	if now.Before(floor) || !now.Before(cutoff) {
		return nil
	}
	_, eventEnd := clanGamesTimes(doc.Event)
	if !now.Before(eventEnd) && len(doc.Members) > 0 {
		complete := true
		for _, member := range doc.Members {
			if !member.Final && !(member.TownHallLevel > 0 && member.TownHallLevel < 6) {
				complete = false
			}
		}
		if complete {
			return nil
		}
	}
	doc.Problems = []string{}
	doc.SyncedAt = now.UTC().Format(time.RFC3339Nano)
	endpoint := "/clans/" + url.PathEscape("#"+tag)
	body, _, err := a.fetchClash(ctx, endpoint)
	var clan struct {
		Tag        string                   `json:"tag"`
		MemberList *[]clanGamesRosterMember `json:"memberList"`
	}
	if err == nil {
		if json.Unmarshal(body, &clan) != nil || normalizeTag(strings.ToUpper(clan.Tag)) != tag || clan.MemberList == nil || len(*clan.MemberList) > 50 {
			err = fmt.Errorf("Lista completa de membros indisponível; a presença anterior foi preservada.")
		} else {
			seen := map[string]bool{}
			for _, member := range *clan.MemberList {
				key := "#" + normalizeTag(strings.ToUpper(member.Tag))
				if !playerTagPattern.MatchString(key) || seen[key] {
					err = fmt.Errorf("Lista de membros inválida; a presença anterior foi preservada.")
					break
				}
				seen[key] = true
			}
		}
	}
	if err != nil {
		doc.Problems = append(doc.Problems, "Lista de membros: "+err.Error())
		for _, member := range doc.Members {
			if !member.Final {
				member.Failure = "Lista de membros indisponível; a participação ainda não foi confirmada."
			}
		}
		return a.saveCollectedClanGames(ctx, tag, &doc, revision)
	}
	applyClanGamesRoster(&doc, *clan.MemberList, a.clashFetchedAt(endpoint))
	if len(doc.Members) > 200 {
		return fmt.Errorf("O limite de membros acompanhados neste período foi atingido.")
	}
	tags := []string{}
	start, _ := clanGamesTimes(doc.Event)
	for key, member := range doc.Members {
		if !member.Final && (member.Current || !now.Before(start)) && !(member.TownHallLevel > 0 && member.TownHallLevel < 6) {
			tags = append(tags, key)
		}
	}
	sort.Strings(tags)
	type profileResult struct {
		tag     string
		profile clanGamesProfile
		at      time.Time
		err     error
	}
	jobs := make(chan string)
	results := make(chan profileResult, len(tags))
	var workers sync.WaitGroup
	for i := 0; i < 4; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for key := range jobs {
				path := "/players/" + url.PathEscape(key)
				body, _, fetchErr := a.fetchClash(ctx, path)
				result := profileResult{tag: key, err: fetchErr}
				if fetchErr == nil {
					result.at = a.clashFetchedAt(path)
					if json.Unmarshal(body, &result.profile) != nil {
						result.err = fmt.Errorf("Perfil inválido recebido da API.")
					}
				}
				results <- result
			}
		}()
	}
	for _, key := range tags {
		jobs <- key
	}
	close(jobs)
	workers.Wait()
	close(results)
	for result := range results {
		if result.err == nil {
			result.err = applyClanGamesProfile(&doc, result.tag, result.profile, tag, result.at, floor, cutoff)
		}
		if result.err != nil {
			doc.Members[result.tag].Failure = result.err.Error()
			doc.Problems = append(doc.Problems, result.tag+": "+result.err.Error())
		}
	}
	sort.Strings(doc.Problems)
	return a.saveCollectedClanGames(ctx, tag, &doc, revision)
}

func (a *app) syncClanGames(w http.ResponseWriter, r *http.Request) {
	tag, ok := a.selectedClanTag(w, r)
	if !ok {
		return
	}
	var input struct {
		Season string `json:"season"`
	}
	if !decodeAdminJSON(w, r, &input) {
		return
	}
	if input.Season == "" {
		input.Season = r.URL.Query().Get("season")
	}
	if input.Season == "" {
		input.Season = time.Now().UTC().Format("2006-01")
	}
	if _, err := clanGamesSeason(input.Season); err != nil {
		jsonResponse(w, 400, map[string]string{"error": err.Error()})
		return
	}
	if !a.clanGamesMu.TryLock() {
		jsonResponse(w, 409, map[string]string{"error": "Uma sincronização dos jogos já está em andamento. Aguarde."})
		return
	}
	defer a.clanGamesMu.Unlock()
	ctx, cancel := context.WithTimeout(r.Context(), 55*time.Second)
	defer cancel()
	if err := a.collectClanGames(ctx, tag, input.Season, time.Now().UTC()); err != nil {
		status := 500
		if errors.Is(err, errClanGamesConflict) {
			status = 409
		}
		jsonResponse(w, status, map[string]string{"error": "Não foi possível sincronizar os jogos. " + err.Error()})
		return
	}
	responseCtx, responseCancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer responseCancel()
	response, err := a.clanGamesResponse(responseCtx, tag, input.Season, time.Now().UTC())
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": "A consulta terminou, mas não foi possível carregar os jogos."})
		return
	}
	jsonResponse(w, 200, response)
}

func (a *app) trackClanGamesOnce(now time.Time) {
	if !a.clanGamesMu.TryLock() {
		return
	}
	defer a.clanGamesMu.Unlock()
	for _, tag := range a.clanTags {
		if tag == "" || a.token == "" {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		current, _, err := readClanGamesDocument(ctx, a.db, tag, now.UTC().Format("2006-01"))
		if err == nil {
			seasons := []string{current.Event.Season}
			start, end := clanGamesTimes(current.Event)
			month, _ := clanGamesSeason(current.Event.Season)
			if now.Before(start) {
				previousSeason := month.AddDate(0, -1, 0).Format("2006-01")
				_, previousRevision, readErr := readClanGamesDocument(ctx, a.db, tag, previousSeason)
				if readErr == nil && previousRevision > 0 {
					seasons = append(seasons, previousSeason)
				}
			} else if !now.Before(end) {
				seasons = append(seasons, month.AddDate(0, 1, 0).Format("2006-01"))
			}
			for _, season := range seasons {
				a.collectClanGames(ctx, tag, season, now)
			}
		}
		cancel()
	}
}

func (a *app) runClanGamesTracking() {
	timer := time.NewTimer(20 * time.Second)
	defer timer.Stop()
	for {
		<-timer.C
		a.trackClanGamesOnce(time.Now().UTC())
		timer.Reset(15 * time.Minute)
	}
}
