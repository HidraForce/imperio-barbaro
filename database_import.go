package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

type sqliteExport struct {
	Version  int       `json:"version"`
	Contacts []contact `json:"contacts"`
	Raids    []struct {
		ClanTag   string          `json:"clanTag"`
		StartTime string          `json:"startTime"`
		Season    json.RawMessage `json:"season"`
		Roster    json.RawMessage `json:"roster"`
		Captured  string          `json:"rosterCapturedAt"`
		During    bool            `json:"rosterDuringRaid"`
		Synced    string          `json:"syncedAt"`
	} `json:"raids"`
}

func importSQLiteExport(ctx context.Context, db *database, path string) error {
	if !db.postgres {
		return fmt.Errorf("a importação exige DATABASE_URL apontando para PostgreSQL")
	}
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("não foi possível abrir a exportação privada")
	}
	defer file.Close()
	var data sqliteExport
	decoder := json.NewDecoder(io.LimitReader(file, 64<<20))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&data); err != nil {
		return fmt.Errorf("exportação inválida ou maior que 64 MiB")
	}
	if err = decoder.Decode(&struct{}{}); err != io.EOF || data.Version != 1 {
		return fmt.Errorf("formato de exportação inválido")
	}
	for i := range data.Contacts {
		if data.Contacts[i].ID <= 0 {
			return fmt.Errorf("identificador inválido na exportação")
		}
		if err = validateContact(&data.Contacts[i]); err != nil {
			return fmt.Errorf("contato %d: %w", data.Contacts[i].ID, err)
		}
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `LOCK TABLE contact_people,contact_accounts,raid_reports IN ACCESS EXCLUSIVE MODE`); err != nil {
		return err
	}
	var count int
	if err = tx.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM contact_people)+(SELECT COUNT(*) FROM raid_reports)`).Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return fmt.Errorf("o destino já possui dados; use um banco vazio para evitar sobrescrever cadastros")
	}
	for _, c := range data.Contacts {
		if _, err = tx.ExecContext(ctx, `INSERT INTO contact_people(id,real_name,phone,notes) VALUES (?,?,?,?)`, c.ID, c.RealName, c.Phone, c.Notes); err != nil {
			return fmt.Errorf("não foi possível importar o contato %d", c.ID)
		}
		for _, a := range c.Accounts {
			if _, err = tx.ExecContext(ctx, `INSERT INTO contact_accounts(player_tag,contact_id,player_name,clan_id) VALUES (?,?,?,?)`, a.PlayerTag, c.ID, a.PlayerName, a.ClanID); err != nil {
				return fmt.Errorf("conta duplicada ou inválida no contato %d", c.ID)
			}
		}
	}
	for _, raid := range data.Raids {
		if raid.ClanTag == "" || raid.StartTime == "" || !json.Valid(raid.Season) || !json.Valid(raid.Roster) {
			return fmt.Errorf("raid inválida na exportação")
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO raid_reports(clan_tag,start_time,season,roster,roster_captured_at,roster_during_raid,synced_at) VALUES (?,?,?,?,?,?,?)`, raid.ClanTag, raid.StartTime, string(raid.Season), string(raid.Roster), raid.Captured, boolInt(raid.During), raid.Synced); err != nil {
			return fmt.Errorf("raid duplicada ou inválida na exportação")
		}
	}
	if _, err = tx.ExecContext(ctx, `SELECT setval(pg_get_serial_sequence('contact_people','id'),COALESCE((SELECT MAX(id) FROM contact_people),1),(SELECT COUNT(*)>0 FROM contact_people))`); err != nil {
		return err
	}
	return tx.Commit()
}
