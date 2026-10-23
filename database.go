package main

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
)

// As consultas deste projeto não possuem '?' dentro de literais SQL.
// Mantemos parâmetros SQLite no código e convertemos somente para PostgreSQL.
type database struct {
	*sql.DB
	postgres bool
}
type databaseTx struct {
	*sql.Tx
	postgres bool
}

func sqlParameters(query string, postgres bool) string {
	if !postgres {
		return query
	}
	var result strings.Builder
	parameter := 0
	quoted := false
	for i := 0; i < len(query); i++ {
		ch := query[i]
		if ch == '\'' {
			if quoted && i+1 < len(query) && query[i+1] == '\'' {
				result.WriteString("''")
				i++
				continue
			}
			quoted = !quoted
		}
		if ch == '?' && !quoted {
			parameter++
			result.WriteByte('$')
			result.WriteString(strconv.Itoa(parameter))
		} else {
			result.WriteByte(ch)
		}
	}
	return result.String()
}
func (d *database) ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error) {
	return d.DB.ExecContext(ctx, sqlParameters(q, d.postgres), args...)
}
func (d *database) QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	return d.DB.QueryContext(ctx, sqlParameters(q, d.postgres), args...)
}
func (d *database) QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row {
	return d.DB.QueryRowContext(ctx, sqlParameters(q, d.postgres), args...)
}
func (d *database) Exec(q string, args ...any) (sql.Result, error) {
	return d.ExecContext(context.Background(), q, args...)
}
func (d *database) QueryRow(q string, args ...any) *sql.Row {
	return d.QueryRowContext(context.Background(), q, args...)
}
func (d *database) BeginTx(ctx context.Context, opts *sql.TxOptions) (*databaseTx, error) {
	tx, err := d.DB.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &databaseTx{tx, d.postgres}, nil
}
func (d *database) Begin() (*databaseTx, error) { return d.BeginTx(context.Background(), nil) }
func (t *databaseTx) ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error) {
	return t.Tx.ExecContext(ctx, sqlParameters(q, t.postgres), args...)
}
func (t *databaseTx) QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	return t.Tx.QueryContext(ctx, sqlParameters(q, t.postgres), args...)
}
func (t *databaseTx) QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row {
	return t.Tx.QueryRowContext(ctx, sqlParameters(q, t.postgres), args...)
}
func (t *databaseTx) Exec(q string, args ...any) (sql.Result, error) {
	return t.ExecContext(context.Background(), q, args...)
}
func (t *databaseTx) QueryRow(q string, args ...any) *sql.Row {
	return t.QueryRowContext(context.Background(), q, args...)
}

const postgresContactsSchema = `
CREATE TABLE IF NOT EXISTS contact_people (
 id BIGSERIAL PRIMARY KEY, real_name TEXT NOT NULL,
 phone TEXT NOT NULL DEFAULT '', notes TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS contact_accounts (
 player_tag TEXT PRIMARY KEY CHECK(player_tag=UPPER(player_tag)), contact_id BIGINT NOT NULL,
 player_name TEXT NOT NULL, clan_id TEXT NOT NULL DEFAULT '' CHECK(clan_id IN ('','1','2')),
 FOREIGN KEY(contact_id) REFERENCES contact_people(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS contact_accounts_person ON contact_accounts(contact_id);
CREATE TABLE IF NOT EXISTS contact_migrations(name TEXT PRIMARY KEY);
CREATE TABLE IF NOT EXISTS crm_state(key TEXT PRIMARY KEY,payload TEXT NOT NULL,expires BIGINT NOT NULL);
CREATE INDEX IF NOT EXISTS crm_state_expiry ON crm_state(expires);
CREATE TABLE IF NOT EXISTS crm_cache(key TEXT PRIMARY KEY,body BYTEA NOT NULL,status INTEGER NOT NULL,error TEXT NOT NULL,fetched_at BIGINT NOT NULL,expires BIGINT NOT NULL);
CREATE INDEX IF NOT EXISTS crm_cache_expiry ON crm_cache(expires);
`

func initPostgresContacts(d *database) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`SELECT pg_advisory_xact_lock(629364820)`); err != nil {
		return err
	}
	for _, q := range strings.Split(postgresContactsSchema, ";") {
		if strings.TrimSpace(q) != "" {
			if _, err = tx.Exec(q); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}
func isUniqueViolation(err error) bool {
	type sqlState interface{ SQLState() string }
	if state, ok := err.(sqlState); ok && state.SQLState() == "23505" {
		return true
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}
