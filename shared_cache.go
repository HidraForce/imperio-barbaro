package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"time"
)

func (a *app) cacheNamespace() string {
	sum := sha256.Sum256([]byte(a.token))
	return fmt.Sprintf("%x:", sum[:])
}
func (a *app) fetchClashShared(ctx context.Context, endpoint string) ([]byte, int, error) {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, 503, fmt.Errorf("Cache compartilhado indisponível.")
	}
	defer tx.Rollback()
	namespace := a.cacheNamespace()
	lock := int64(629364823)
	// Lock transacional: funciona também com o pool de conexões do Neon.
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(?)`, lock); err != nil {
		return nil, 503, fmt.Errorf("Cache compartilhado indisponível.")
	}
	now := time.Now().UTC()
	key := namespace + endpoint
	var body []byte
	var status int
	var message string
	var fetched, expires int64
	err = tx.QueryRowContext(ctx, `SELECT body,status,error,fetched_at,expires FROM crm_cache WHERE key=?`, key).Scan(&body, &status, &message, &fetched, &expires)
	if err != nil && err != sql.ErrNoRows {
		return nil, 503, fmt.Errorf("Cache compartilhado indisponível.")
	}
	if err == nil && now.UnixMilli() < expires {
		if message != "" {
			return body, status, fmt.Errorf("%s", message)
		}
		return body, status, nil
	}
	var cooldown int64
	err = tx.QueryRowContext(ctx, `SELECT expires FROM crm_state WHERE key=?`, namespace+"clash-cooldown").Scan(&cooldown)
	if err != nil && err != sql.ErrNoRows {
		return nil, 503, fmt.Errorf("Cache compartilhado indisponível.")
	}
	if err == nil && now.UnixMilli() < cooldown {
		return nil, 429, fmt.Errorf("Limite de consultas da API atingido. Aguarde antes de atualizar novamente.")
	}
	body, status, fetchErr := a.fetchClashUncached(ctx, endpoint)
	at := time.Now().UTC()
	ttl := clashTTL(endpoint)
	message = ""
	if fetchErr != nil {
		ttl = 30 * time.Second
		message = fetchErr.Error()
	}
	if body == nil {
		body = []byte{}
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM crm_cache WHERE expires<=?`, at.UnixMilli()); err != nil {
		return nil, 503, fmt.Errorf("Cache compartilhado indisponível.")
	}
	var count int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM crm_cache`).Scan(&count); err != nil {
		return nil, 503, fmt.Errorf("Cache compartilhado indisponível.")
	}
	if count >= 128 {
		if _, err = tx.ExecContext(ctx, `DELETE FROM crm_cache WHERE key=(SELECT key FROM crm_cache ORDER BY expires LIMIT 1)`); err != nil {
			return nil, 503, fmt.Errorf("Cache compartilhado indisponível.")
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO crm_cache(key,body,status,error,fetched_at,expires) VALUES (?,?,?,?,?,?) ON CONFLICT(key) DO UPDATE SET body=excluded.body,status=excluded.status,error=excluded.error,fetched_at=excluded.fetched_at,expires=excluded.expires`, key, body, status, message, at.UnixMilli(), at.Add(ttl).UnixMilli()); err != nil {
		return nil, 503, fmt.Errorf("Cache compartilhado indisponível.")
	}
	if status == 429 {
		a.cacheMu.Lock()
		until := a.retryAt
		a.cacheMu.Unlock()
		if _, err = tx.ExecContext(ctx, `INSERT INTO crm_state(key,payload,expires) VALUES (?,'{}',?) ON CONFLICT(key) DO UPDATE SET expires=GREATEST(crm_state.expires,excluded.expires)`, namespace+"clash-cooldown", until.UnixMilli()); err != nil {
			return nil, 503, fmt.Errorf("Cache compartilhado indisponível.")
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, 503, fmt.Errorf("Cache compartilhado indisponível.")
	}
	return body, status, fetchErr
}
