package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

type memberObservation struct {
	FirstSeen    string `json:"firstSeen"`
	JoinAfter    string `json:"joinAfter"`
	DetectedJoin bool   `json:"detectedJoin"`
	NewMember    bool   `json:"newMember"`
	present      bool
}

func initMembers(db *database) error {
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
		`CREATE TABLE IF NOT EXISTS member_snapshots(clan_tag TEXT PRIMARY KEY,observed_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS member_observations(clan_tag TEXT NOT NULL,player_tag TEXT NOT NULL,first_seen TEXT NOT NULL,join_after TEXT NOT NULL,detected_join INTEGER NOT NULL,present INTEGER NOT NULL,PRIMARY KEY(clan_tag,player_tag))`,
	} {
		if _, err = tx.Exec(q); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (a *app) observeMembers(ctx context.Context, tag string, body []byte, at time.Time) error {
	var clan struct {
		Tag        string             `json:"tag"`
		MemberList []raidRosterMember `json:"memberList"`
	}
	if json.Unmarshal(body, &clan) != nil || normalizeTag(clan.Tag) != tag || clan.MemberList == nil || len(clan.MemberList) > 50 || at.IsZero() {
		return fmt.Errorf("Lista completa de membros indisponível; o histórico de entradas foi preservado.")
	}
	seen := map[string]bool{}
	for _, m := range clan.MemberList {
		key := "#" + normalizeTag(strings.ToUpper(m.Tag))
		if !playerTagPattern.MatchString(key) || seen[key] {
			return fmt.Errorf("Lista de membros inválida.")
		}
		seen[key] = true
	}
	a.membersMu.Lock()
	defer a.membersMu.Unlock()
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if a.db.postgres {
		if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(629364821)`); err != nil {
			return err
		}
	}
	var previous string
	err = tx.QueryRowContext(ctx, `SELECT observed_at FROM member_snapshots WHERE clan_tag=?`, tag).Scan(&previous)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if previous != "" {
		last, err := time.Parse(time.RFC3339Nano, previous)
		if err != nil {
			return err
		}
		if !at.After(last) {
			return nil
		}
	}
	for key := range seen {
		_, err = tx.ExecContext(ctx, `INSERT INTO member_observations(clan_tag,player_tag,first_seen,join_after,detected_join,present) VALUES(?,?,?,?,?,1)
 ON CONFLICT(clan_tag,player_tag) DO UPDATE SET first_seen=CASE WHEN member_observations.present=0 THEN excluded.first_seen ELSE member_observations.first_seen END,join_after=CASE WHEN member_observations.present=0 THEN excluded.join_after ELSE member_observations.join_after END,detected_join=CASE WHEN member_observations.present=0 THEN excluded.detected_join ELSE member_observations.detected_join END,present=1`, tag, key, at.UTC().Format(time.RFC3339Nano), previous, boolInt(previous != ""))
		if err != nil {
			return err
		}
	}
	rows, err := tx.Tx.QueryContext(ctx, sqlParameters(`SELECT player_tag FROM member_observations WHERE clan_tag=? AND present=1`, a.db.postgres), tag)
	if err != nil {
		return err
	}
	absent := []string{}
	for rows.Next() {
		var key string
		if err = rows.Scan(&key); err != nil {
			rows.Close()
			return err
		}
		if !seen[key] {
			absent = append(absent, key)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, key := range absent {
		if _, err = tx.ExecContext(ctx, `UPDATE member_observations SET present=0 WHERE clan_tag=? AND player_tag=?`, tag, key); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO member_snapshots(clan_tag,observed_at) VALUES(?,?) ON CONFLICT(clan_tag) DO UPDATE SET observed_at=excluded.observed_at`, tag, at.UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	return tx.Commit()
}

func (a *app) memberObservations(ctx context.Context, tag string, now time.Time) (map[string]memberObservation, error) {
	rows, err := a.db.QueryContext(ctx, `SELECT player_tag,first_seen,join_after,detected_join,present FROM member_observations WHERE clan_tag=?`, tag)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]memberObservation{}
	for rows.Next() {
		var key string
		var item memberObservation
		var detected, present int
		if err = rows.Scan(&key, &item.FirstSeen, &item.JoinAfter, &detected, &present); err != nil {
			return nil, err
		}
		item.DetectedJoin, item.present = detected == 1, present == 1
		first, err := time.Parse(time.RFC3339Nano, item.FirstSeen)
		item.NewMember = err == nil && item.DetectedJoin && item.present && !now.Before(first) && now.Sub(first) < 7*24*time.Hour
		result[key] = item
	}
	return result, rows.Err()
}

func (a *app) trackClanMembers(ctx context.Context, tag string) ([]byte, int, error) {
	endpoint := "/clans/" + url.PathEscape("#"+tag)
	body, status, err := a.fetchClash(ctx, endpoint)
	if err != nil {
		return nil, status, err
	}
	if err = a.observeMembers(ctx, tag, body, a.clashFetchedAt(endpoint)); err != nil {
		return nil, 503, err
	}
	return body, status, nil
}

func membershipWarnings(records []moderationRecord, members map[string]memberObservation, available bool) []moderationRecord {
	result := []moderationRecord{}
	for _, record := range records {
		member := members[record.PlayerTag]
		end, endErr := time.Parse("20060102T150405.000Z", record.EventEnd)
		lower, lowerErr := time.Parse(time.RFC3339Nano, member.JoinAfter)
		if member.DetectedJoin && endErr == nil && lowerErr == nil && !lower.Before(end) {
			continue
		}
		first, firstErr := time.Parse(time.RFC3339Nano, member.FirstSeen)
		start, startErr := time.Parse("20060102T150405.000Z", record.EventStart)
		uncertain := member.DetectedJoin && firstErr == nil && ((endErr == nil && first.After(end)) || startErr != nil || first.After(start))
		if !available || uncertain {
			record.Status = "pending"
			record.Notes = "Conferir entrada/retorno ao clã e elegibilidade no período do evento. Este registro não conta como aviso ativo."
		}
		result = append(result, record)
	}
	return result
}
