package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type raidRosterMember struct {
	Tag           string `json:"tag"`
	Name          string `json:"name"`
	TownHallLevel int    `json:"townHallLevel"`
}
type raidRecord struct {
	Season           json.RawMessage    `json:"season"`
	Roster           []raidRosterMember `json:"roster"`
	RosterCapturedAt string             `json:"rosterCapturedAt"`
	RosterDuringRaid bool               `json:"rosterDuringRaid"`
	SyncedAt         string             `json:"syncedAt"`
}

func initRaids(db *database) error {
	query := `CREATE TABLE IF NOT EXISTS raid_reports (
 clan_tag TEXT NOT NULL, start_time TEXT NOT NULL, season TEXT NOT NULL,
 roster TEXT NOT NULL, roster_captured_at TEXT NOT NULL,
 roster_during_raid INTEGER NOT NULL, synced_at TEXT NOT NULL,
 PRIMARY KEY (clan_tag, start_time)
 )`
	if db.postgres {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err = tx.Exec(`SELECT pg_advisory_xact_lock(629364820)`); err != nil {
			return err
		}
		if _, err = tx.Exec(query); err != nil {
			return err
		}
		return tx.Commit()
	}
	_, err := db.Exec(query)
	return err
}

func (a *app) fetchClashUncached(ctx context.Context, endpoint string) ([]byte, int, error) {
	if a.token == "" {
		return nil, 503, fmt.Errorf("Configure o token da API do Clash e reinicie o aplicativo.")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.clashofclans.com/v1"+endpoint, nil)
	if err != nil {
		return nil, 500, fmt.Errorf("Não foi possível criar a consulta à API do Clash.")
	}
	req.Header.Set("Authorization", "Bearer "+a.token)
	resp, err := a.clash.Do(req)
	if err != nil {
		return nil, 502, fmt.Errorf("Não foi possível acessar a API do Clash of Clans.")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, (8<<20)+1))
	if err != nil || len(body) > 8<<20 {
		return nil, 502, fmt.Errorf("Não foi possível ler a resposta da API do Clash.")
	}
	if resp.StatusCode != http.StatusOK {
		var detail struct {
			Reason  string `json:"reason"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(body, &detail)
		message := "Falha na consulta à API do Clash."
		switch resp.StatusCode {
		case 403:
			if strings.Contains(endpoint, "currentwar") {
				message = "Acesso à guerra negado. Verifique o registro público de guerras, o token e o IP público autorizado."
			} else {
				message = "Acesso à API do Clash negado. Verifique o token e o IP público autorizado."
			}
		case 404:
			if strings.Contains(endpoint, "leaguegroup") {
				message = "Nenhum grupo atual da Liga de Guerra de Clãs está disponível."
			} else if strings.Contains(endpoint, "/clanwarleagues/wars/") {
				message = "Esta guerra da liga ainda não está disponível."
			} else if strings.Contains(endpoint, "currentwar") {
				message = "Nenhuma guerra atual está disponível para este clã."
			} else {
				message = "Clã não encontrado. Verifique a tag configurada."
			}
		case 429:
			a.pauseClash(resp.Header.Get("Retry-After"))
			message = "Limite de consultas da API atingido. Aguarde antes de sincronizar novamente."
		case 503:
			message = "A API do Clash está em manutenção. Tente novamente mais tarde."
		}
		return nil, resp.StatusCode, fmt.Errorf("%s", message)
	}
	if !json.Valid(body) {
		return nil, 502, fmt.Errorf("A API do Clash retornou uma resposta JSON inválida.")
	}
	return body, resp.StatusCode, nil
}

func (a *app) savedRaids(ctx context.Context, tag string) ([]raidRecord, error) {
	rows, err := a.db.QueryContext(ctx, `SELECT season, roster, roster_captured_at, roster_during_raid, synced_at FROM raid_reports WHERE clan_tag=? ORDER BY start_time DESC LIMIT 52`, tag)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := []raidRecord{}
	for rows.Next() {
		var rec raidRecord
		var season, roster string
		if err := rows.Scan(&season, &roster, &rec.RosterCapturedAt, &rec.RosterDuringRaid, &rec.SyncedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(roster), &rec.Roster); err != nil {
			return nil, err
		}
		rec.Season = json.RawMessage(season)
		records = append(records, rec)
	}
	return records, rows.Err()
}

func (a *app) raids(w http.ResponseWriter, r *http.Request) {
	tag, ok := a.selectedClanTag(w, r)
	if !ok {
		return
	}
	base := "/clans/" + url.PathEscape("#"+tag)
	body, _, fetchErr := a.fetchClash(r.Context(), base+"/capitalraidseasons?limit=10")
	warning := ""
	if fetchErr == nil {
		var response struct {
			Items []json.RawMessage `json:"items"`
		}
		if err := json.Unmarshal(body, &response); err != nil {
			jsonResponse(w, 502, map[string]string{"error": "Resposta inválida da API do Clash para raids."})
			return
		}
		if len(response.Items) > 0 {
			clanBody, _, rosterErr := a.fetchClash(r.Context(), base)
			if rosterErr != nil {
				fetchErr = rosterErr
			} else {
				var clan struct {
					MemberList []raidRosterMember `json:"memberList"`
				}
				if err := json.Unmarshal(clanBody, &clan); err != nil {
					jsonResponse(w, 502, map[string]string{"error": "Resposta inválida para a lista de membros do clã."})
					return
				}
				roster, _ := json.Marshal(clan.MemberList)
				now := a.clashFetchedAt(base + "/capitalraidseasons?limit=10")
				rosterAt := a.clashFetchedAt(base)
				if now.IsZero() || rosterAt.IsZero() {
					jsonResponse(w, 503, map[string]string{"error": "Não foi possível confirmar a data da consulta. Sincronize novamente."})
					return
				}
				for _, raw := range response.Items {
					var season struct {
						StartTime string `json:"startTime"`
						EndTime   string `json:"endTime"`
					}
					if err := json.Unmarshal(raw, &season); err != nil || season.StartTime == "" {
						jsonResponse(w, 502, map[string]string{"error": "Temporada de raids inválida."})
						return
					}
					start, startErr := time.Parse("20060102T150405.000Z", season.StartTime)
					end, endErr := time.Parse("20060102T150405.000Z", season.EndTime)
					during := startErr == nil && endErr == nil && !rosterAt.Before(start) && rosterAt.Before(end)
					_, err := a.db.ExecContext(r.Context(), `INSERT INTO raid_reports (clan_tag,start_time,season,roster,roster_captured_at,roster_during_raid,synced_at) VALUES (?,?,?,?,?,?,?)
      ON CONFLICT(clan_tag,start_time) DO UPDATE SET season=excluded.season, synced_at=excluded.synced_at`, tag, season.StartTime, string(raw), string(roster), rosterAt.Format(time.RFC3339), boolInt(during), now.Format(time.RFC3339))
					if err != nil {
						jsonResponse(w, 500, map[string]string{"error": "Não foi possível salvar os registros semanais de raids."})
						return
					}
				}
			}
		}
	}
	if fetchErr != nil {
		warning = fetchErr.Error()
	}
	records, err := a.savedRaids(r.Context(), tag)
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": "Não foi possível ler os registros salvos de raids."})
		return
	}
	if warning != "" && len(records) == 0 {
		jsonResponse(w, 502, map[string]string{"error": warning})
		return
	}
	jsonResponse(w, 200, map[string]any{"items": records, "warning": warning})
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
