package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

func privateStateKey(prefix string, secret [32]byte, value string) string {
	hash := hmac.New(sha256.New, secret[:])
	hash.Write([]byte(value))
	return prefix + hex.EncodeToString(hash.Sum(nil))
}
func securityTransaction(ctx context.Context, db *database) (*databaseTx, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(629364822)`); err != nil {
		tx.Rollback()
		return nil, err
	}
	return tx, nil
}
func sharedAttempt(ctx context.Context, db *database, key, prefix string, incorrect bool, limit int, now time.Time) (time.Time, error) {
	tx, err := securityTransaction(ctx, db)
	if err != nil {
		return time.Time{}, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `DELETE FROM crm_state WHERE expires<=?`, now.UnixMilli()); err != nil {
		return time.Time{}, err
	}
	var payload string
	var expiry int64
	count := 0
	err = tx.QueryRowContext(ctx, `SELECT payload,expires FROM crm_state WHERE key=?`, key).Scan(&payload, &expiry)
	if err != nil && err != sql.ErrNoRows {
		return time.Time{}, err
	}
	if err == nil {
		if err = json.Unmarshal([]byte(payload), &count); err != nil {
			return time.Time{}, err
		}
	}
	if count >= limit {
		return time.UnixMilli(expiry), tx.Commit()
	}
	if incorrect {
		if count == 0 {
			var total int
			if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM crm_state WHERE key LIKE ?`, prefix+"%").Scan(&total); err != nil {
				return time.Time{}, err
			}
			if total >= 2048 {
				return now.Add(time.Minute), tx.Commit()
			}
			expiry = now.Add(5 * time.Minute).UnixMilli()
		}
		count++
		body, _ := json.Marshal(count)
		if _, err = tx.ExecContext(ctx, `INSERT INTO crm_state(key,payload,expires) VALUES (?,?,?) ON CONFLICT(key) DO UPDATE SET payload=excluded.payload,expires=excluded.expires`, key, string(body), expiry); err != nil {
			return time.Time{}, err
		}
	}
	return time.Time{}, tx.Commit()
}
func sharedUnavailable(w http.ResponseWriter) {
	securityError(w, 503, "Proteção temporariamente indisponível. Tente novamente.")
}
func sharedLimited(w http.ResponseWriter, until, now time.Time) {
	w.Header().Set("Retry-After", strconv.Itoa(int(until.Sub(now).Seconds())+1))
	securityError(w, 429, "Muitas tentativas. Aguarde antes de tentar novamente.")
}
func (g *accessGuard) sharedAccess(w http.ResponseWriter, r *http.Request, next http.Handler, ip string) {
	username, password, ok := r.BasicAuth()
	userHash, passwordHash := sha256.Sum256([]byte(username)), sha256.Sum256([]byte(password))
	valid := ok && (subtle.ConstantTimeCompare(userHash[:], g.username[:])&subtle.ConstantTimeCompare(passwordHash[:], g.password[:]) == 1)
	now := g.now()
	key := privateStateKey("auth:", g.password, ip)
	until, err := sharedAttempt(r.Context(), g.store, key, "auth:", !valid && r.Header.Get("Authorization") != "", 10, now)
	if err != nil {
		sharedUnavailable(w)
		return
	}
	if !until.IsZero() {
		sharedLimited(w, until, now)
		return
	}
	if !valid {
		w.Header().Set("WWW-Authenticate", `Basic realm="Império Bárbaro CRM", charset="UTF-8"`)
		securityError(w, 401, "Informe o usuário e a senha do painel.")
		return
	}
	next.ServeHTTP(w, r)
}
func (v *contactVault) unlockShared(w http.ResponseWriter, r *http.Request, password, ip string) {
	guess := sha256.Sum256([]byte(password))
	valid := subtle.ConstantTimeCompare(guess[:], v.password[:]) == 1
	now := v.now()
	until, err := sharedAttempt(r.Context(), v.store, privateStateKey("contacts-fail:", v.password, ip), "contacts-fail:", !valid, 5, now)
	if err != nil {
		sharedUnavailable(w)
		return
	}
	if !until.IsZero() {
		sharedLimited(w, until, now)
		return
	}
	if !valid {
		securityError(w, 403, "Senha de contatos incorreta.")
		return
	}
	var bytes [32]byte
	if _, err = rand.Read(bytes[:]); err != nil {
		sharedUnavailable(w)
		return
	}
	token := base64.RawURLEncoding.EncodeToString(bytes[:])
	expires := now.Add(contactSessionTTL)
	tx, err := securityTransaction(r.Context(), v.store)
	if err != nil {
		sharedUnavailable(w)
		return
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(r.Context(), `DELETE FROM crm_state WHERE expires<=?`, now.UnixMilli()); err != nil {
		sharedUnavailable(w)
		return
	}
	var count int
	if err = tx.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM crm_state WHERE key LIKE 'contacts-session:%'`).Scan(&count); err != nil {
		sharedUnavailable(w)
		return
	}
	if count >= 256 {
		sharedLimited(w, now.Add(time.Minute), now)
		return
	}
	if _, err = tx.ExecContext(r.Context(), `INSERT INTO crm_state(key,payload,expires) VALUES (?,?,?)`, privateStateKey("contacts-session:", v.password, token), "{}", expires.UnixMilli()); err != nil {
		sharedUnavailable(w)
		return
	}
	if err = tx.Commit(); err != nil {
		sharedUnavailable(w)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]any{"token": token, "expiresAt": expires.UTC().Format(time.RFC3339)})
}
func (v *contactVault) protectShared(w http.ResponseWriter, r *http.Request, next http.Handler) {
	token := r.Header.Get("X-Contacts-Session")
	if len(token) != 43 {
		securityError(w, 423, "Contatos bloqueados. Informe a senha exclusiva para continuar.")
		return
	}
	var expiry int64
	err := v.store.QueryRowContext(r.Context(), `SELECT expires FROM crm_state WHERE key=?`, privateStateKey("contacts-session:", v.password, token)).Scan(&expiry)
	if err != nil && err != sql.ErrNoRows {
		sharedUnavailable(w)
		return
	}
	if err == sql.ErrNoRows || v.now().UnixMilli() >= expiry {
		securityError(w, 423, "Contatos bloqueados. Informe a senha exclusiva para continuar.")
		return
	}
	next.ServeHTTP(w, r)
}
func (v *contactVault) lockShared(w http.ResponseWriter, r *http.Request) {
	if _, err := v.store.ExecContext(r.Context(), `DELETE FROM crm_state WHERE key=?`, privateStateKey("contacts-session:", v.password, r.Header.Get("X-Contacts-Session"))); err != nil {
		sharedUnavailable(w)
		return
	}
	w.WriteHeader(204)
}
