// Package config loads connection settings from the environment.
// In local development, .env.local (git-ignored) is loaded first via
// godotenv, then real environment variables are read the same way
// they would be in any deployed environment (Docker, systemd, Lambda).
package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

// Postgres holds everything needed to open a database/sql connection.
type Postgres struct {
	Host     string
	Port     string
	User     string
	Password string
	Database string
}

// LoadPostgres reads PG* environment variables, loading .env.local
// first if present. A missing .env.local is not an error — in
// production, real environment variables are expected to already be
// set, and there is no file to load.
func LoadPostgres() (Postgres, error) {
	_ = godotenv.Load(".env.local")

	cfg := Postgres{
		Host:     getenv("PGHOST", "localhost"),
		Port:     getenv("PGPORT", "5432"),
		User:     os.Getenv("PGUSER"),
		Password: os.Getenv("PGPASSWORD"),
		Database: getenv("PGDATABASE", "olist_etl"),
	}

	if cfg.User == "" {
		return Postgres{}, fmt.Errorf("config: PGUSER is required")
	}
	if cfg.Password == "" {
		return Postgres{}, fmt.Errorf("config: PGPASSWORD is required")
	}

	return cfg, nil
}

// DSN returns a connection string in URL form, as expected by
// pgx/v5/stdlib.
func (p Postgres) DSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s",
		p.User, p.Password, p.Host, p.Port, p.Database)
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
