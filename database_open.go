package main

import (
	"context"
	"database/sql"
	"fmt"
	_ "github.com/jackc/pgx/v5/stdlib"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

func openDatabase() (*database, error) {
	remote := os.Getenv("DATABASE_URL")
	driver, dsn := "sqlite", env("DATABASE_PATH", "./data/clan.db")
	if remote != "" {
		parsed, err := url.Parse(remote)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
			return nil, fmt.Errorf("DATABASE_URL deve ser uma conexão PostgreSQL válida")
		}
		if os.Getenv("VERCEL") == "1" && parsed.Query().Get("sslmode") != "verify-full" {
			return nil, fmt.Errorf("use sslmode=verify-full em DATABASE_URL para verificar o certificado do banco")
		}
		driver, dsn = "pgx", remote
	} else {
		if os.Getenv("VERCEL") == "1" {
			return nil, fmt.Errorf("configure DATABASE_URL: SQLite local não é persistente na Vercel")
		}
		if err := os.MkdirAll(filepath.Dir(dsn), 0700); err != nil {
			return nil, fmt.Errorf("não foi possível criar o diretório do banco")
		}
	}
	connection, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("não foi possível configurar o banco")
	}
	db := &database{connection, remote != ""}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(5 * time.Minute)
	if !db.postgres {
		db.SetMaxOpenConns(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("não foi possível conectar ao banco; confira a configuração e o acesso de rede")
	}
	if !db.postgres {
		if _, err = db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
			db.Close()
			return nil, err
		}
	}
	return db, nil
}
