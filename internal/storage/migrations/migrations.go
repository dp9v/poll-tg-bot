// Package migrations embeds and runs PostgreSQL schema migrations using goose.
//
// Migration files live next to this file as plain `.sql` scripts and follow the
// goose convention (`-- +goose Up` / `-- +goose Down`). They are baked into the
// binary via `embed.FS`, so deployments don't need any extra files.
//
// To add a new migration, drop a file named like
// `0000N_short_description.sql` into this directory — it will be picked up
// automatically on the next start.
package migrations

import (
	"context"
	"database/sql"
	"embed"
	"fmt"

	"github.com/pressly/goose/v3"
)

//go:embed *.sql
var fs embed.FS

// Up applies all pending migrations to the given database.
// It is safe to call on every application start: already-applied migrations are skipped.
func Up(ctx context.Context, db *sql.DB) error {
	provider, err := newProvider(db)
	if err != nil {
		return err
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

// Down rolls back the most recently applied migration. Intended for tooling/tests.
func Down(ctx context.Context, db *sql.DB) error {
	provider, err := newProvider(db)
	if err != nil {
		return err
	}
	if _, err := provider.Down(ctx); err != nil {
		return fmt.Errorf("rollback migration: %w", err)
	}
	return nil
}

func newProvider(db *sql.DB) (*goose.Provider, error) {
	provider, err := goose.NewProvider(
		goose.DialectPostgres,
		db,
		fs,
		goose.WithVerbose(false),
	)
	if err != nil {
		return nil, fmt.Errorf("init goose provider: %w", err)
	}
	return provider, nil
}


