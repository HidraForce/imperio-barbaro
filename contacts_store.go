package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

const contactsSchema = `
CREATE TABLE IF NOT EXISTS contact_people (
 id INTEGER PRIMARY KEY AUTOINCREMENT, real_name TEXT NOT NULL,
 phone TEXT NOT NULL DEFAULT '', notes TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS contact_accounts (
 player_tag TEXT PRIMARY KEY COLLATE NOCASE, contact_id INTEGER NOT NULL,
 player_name TEXT NOT NULL, clan_id TEXT NOT NULL DEFAULT '' CHECK (clan_id IN ('','1','2')),
 FOREIGN KEY(contact_id) REFERENCES contact_people(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS contact_accounts_person ON contact_accounts(contact_id);
CREATE TABLE IF NOT EXISTS contact_migrations (name TEXT PRIMARY KEY);
CREATE TABLE IF NOT EXISTS contacts (
 id INTEGER PRIMARY KEY AUTOINCREMENT, player_tag TEXT NOT NULL UNIQUE,
 player_name TEXT NOT NULL, real_name TEXT NOT NULL DEFAULT '',
 phone TEXT NOT NULL DEFAULT '', notes TEXT NOT NULL DEFAULT '',
 created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);`

const migrateContactPeople = `INSERT INTO contact_people (id,real_name,phone,notes)
 SELECT id,CASE WHEN TRIM(real_name)='' THEN player_name ELSE real_name END,phone,notes FROM contacts`
const migrateContactAccounts = `INSERT INTO contact_accounts (player_tag,contact_id,player_name,clan_id)
 SELECT '#' || UPPER(LTRIM(TRIM(player_tag),'#')),id,player_name,'' FROM contacts`

func initContacts(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(contactsSchema); err != nil {
		return err
	}
	var count int
	if err = tx.QueryRow(`SELECT COUNT(*) FROM contact_migrations WHERE name='people-v1'`).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		if _, err = tx.Exec(migrateContactPeople); err != nil {
			return err
		}
		if _, err = tx.Exec(migrateContactAccounts); err != nil {
			return fmt.Errorf("não foi possível migrar as contas; verifique tags duplicadas nos contatos antigos: %w", err)
		}
		if _, err = tx.Exec(`INSERT INTO contact_migrations(name) VALUES ('people-v1')`); err != nil {
			return err
		}
	}
	// A tabela antiga só é removida após a cópia completa, na mesma transação.
	if _, err = tx.Exec(`DROP TABLE contacts`); err != nil {
		return err
	}
	return tx.Commit()
}

func listContacts(ctx context.Context, db *sql.DB) ([]contact, error) {
	rows, err := db.QueryContext(ctx, `SELECT p.id,p.real_name,p.phone,p.notes,a.player_tag,a.player_name,a.clan_id
 FROM contact_people p JOIN contact_accounts a ON a.contact_id=p.id ORDER BY p.real_name,p.id,a.player_name,a.player_tag`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []contact{}
	indices := map[int64]int{}
	for rows.Next() {
		var c contact
		var account contactAccount
		if err = rows.Scan(&c.ID, &c.RealName, &c.Phone, &c.Notes, &account.PlayerTag, &account.PlayerName, &account.ClanID); err != nil {
			return nil, err
		}
		index, exists := indices[c.ID]
		if !exists {
			index = len(items)
			indices[c.ID] = index
			c.Accounts = []contactAccount{}
			items = append(items, c)
		}
		items[index].Accounts = append(items[index].Accounts, account)
	}
	return items, rows.Err()
}

var errContactNotFound = errors.New("Contato não encontrado.")
var errMergeTooMany = errors.New("Um contato pode ter no máximo 20 contas.")
var errMergeNotes = errors.New("As observações reunidas excedem 1.000 caracteres. Revise os contatos antes de reunir.")
var errInvalidMerge = errors.New("Escolha dois contatos diferentes da mesma pessoa.")
var errAccountInUse = errors.New("Uma das contas já pertence a outro contato. Edite esse contato ou reúna os contatos da mesma pessoa.")

func writeContact(ctx context.Context, db *sql.DB, c *contact) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if c.ID == 0 {
		result, err := tx.ExecContext(ctx, `INSERT INTO contact_people(real_name,phone,notes) VALUES (?,?,?)`, c.RealName, c.Phone, c.Notes)
		if err != nil {
			return err
		}
		if c.ID, err = result.LastInsertId(); err != nil {
			return err
		}
	} else {
		result, err := tx.ExecContext(ctx, `UPDATE contact_people SET real_name=?,phone=?,notes=? WHERE id=?`, c.RealName, c.Phone, c.Notes, c.ID)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count == 0 {
			return errContactNotFound
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM contact_accounts WHERE contact_id=?`, c.ID); err != nil {
			return err
		}
	}
	for _, account := range c.Accounts {
		if _, err = tx.ExecContext(ctx, `INSERT INTO contact_accounts(player_tag,contact_id,player_name,clan_id) VALUES (?,?,?,?)`, account.PlayerTag, c.ID, account.PlayerName, account.ClanID); err != nil {
			if strings.Contains(err.Error(), "UNIQUE constraint failed") {
				return errAccountInUse
			}
			return err
		}
	}
	return tx.Commit()
}

func eraseContact(ctx context.Context, db *sql.DB, id int64) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `DELETE FROM contact_accounts WHERE contact_id=?`, id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM contact_people WHERE id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func mergeContacts(ctx context.Context, db *sql.DB, targetID, sourceID int64) error {
	if targetID <= 0 || sourceID <= 0 || targetID == sourceID {
		return errInvalidMerge
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var target, source contact
	for id, c := range map[int64]*contact{targetID: &target, sourceID: &source} {
		if err = tx.QueryRowContext(ctx, `SELECT real_name,phone,notes FROM contact_people WHERE id=?`, id).Scan(&c.RealName, &c.Phone, &c.Notes); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errContactNotFound
			}
			return err
		}
	}
	var count int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM contact_accounts WHERE contact_id IN (?,?)`, targetID, sourceID).Scan(&count); err != nil {
		return err
	}
	if count > 20 {
		return errMergeTooMany
	}
	notes := target.Notes
	if source.RealName != target.RealName {
		notes += "\nContato reunido: " + source.RealName
	}
	if target.Phone == "" {
		target.Phone = source.Phone
	} else if source.Phone != "" && source.Phone != target.Phone {
		notes += "\nTelefone adicional: " + source.Phone
	}
	if source.Notes != "" && source.Notes != target.Notes {
		notes += "\n" + source.Notes
	}
	target.Notes = strings.TrimSpace(notes)
	if len([]rune(target.Notes)) > 1000 {
		return errMergeNotes
	}
	if _, err = tx.ExecContext(ctx, `UPDATE contact_people SET phone=?,notes=? WHERE id=?`, target.Phone, target.Notes, targetID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE contact_accounts SET contact_id=? WHERE contact_id=?`, targetID, sourceID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM contact_people WHERE id=?`, sourceID); err != nil {
		return err
	}
	return tx.Commit()
}
