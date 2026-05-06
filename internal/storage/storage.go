package storage

import (
	"context"
	"database/sql"
	"fmt"
	"go-notification-tg-bot/internal/alteg"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// Storage persists activities to a PostgreSQL database.
type Storage struct {
	db *sql.DB
}

// New opens the PostgreSQL database using the given DSN and runs migrations.
// The DSN must be in the libpq/URL format accepted by pgx, for example:
//
//	postgres://user:pass@host:5432/dbname?sslmode=disable
func New(dsn string) (*Storage, error) {
	if dsn == "" {
		return nil, fmt.Errorf("postgres DSN is empty")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres db: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres db: %w", err)
	}

	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate postgres db: %w", err)
	}

	return &Storage{db: db}, nil
}

// Close closes the underlying database connection.
func (s *Storage) Close() error {
	return s.db.Close()
}

// migrate creates the activities table if it does not exist.
func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS activities (
			id            BIGINT  PRIMARY KEY,
			date          TEXT    NOT NULL,
			capacity      INTEGER NOT NULL,
			records_count INTEGER NOT NULL,
			staff_id      BIGINT  NOT NULL,
			staff_name    TEXT    NOT NULL,
			service_id    BIGINT  NOT NULL,
			service_title TEXT    NOT NULL
		)
	`)
	return err
}

// Save upserts the given activities into the database.
// Existing rows are updated if their available places changed; new rows are inserted.
// Activities that are no longer present in the slice are NOT deleted — they remain
// as historical records.
func (s *Storage) Save(activities []alteg.Activity) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(`
		INSERT INTO activities (id, date, capacity, records_count, staff_id, staff_name, service_id, service_title)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (id) DO UPDATE SET
			date          = EXCLUDED.date,
			capacity      = EXCLUDED.capacity,
			records_count = EXCLUDED.records_count,
			staff_id      = EXCLUDED.staff_id,
			staff_name    = EXCLUDED.staff_name,
			service_id    = EXCLUDED.service_id,
			service_title = EXCLUDED.service_title
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, a := range activities {
		if _, err := stmt.Exec(
			a.ID, a.Date, a.Capacity, a.RecordsCount,
			a.Staff.ID, a.Staff.Name,
			a.Service.ID, a.Service.Title,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// LoadBetween returns activities whose date falls within [from, to] (inclusive).
// Dates are compared as strings in the format stored by the API (e.g. "2026-03-04 10:00:00").
func (s *Storage) LoadBetween(from, to time.Time) ([]alteg.Activity, error) {
	const layout = "2006-01-02 15:04:05"
	rows, err := s.db.Query(`
		SELECT id, date, capacity, records_count, staff_id, staff_name, service_id, service_title
		FROM activities
		WHERE date >= $1 AND date <= $2
		ORDER BY date
	`, from.Format(layout), to.Format(layout))
	if err != nil {
		return nil, err
	}
	return scanActivities(rows)
}

// Load returns all activities currently stored in the database.
func (s *Storage) Load() ([]alteg.Activity, error) {
	rows, err := s.db.Query(`
		SELECT id, date, capacity, records_count, staff_id, staff_name, service_id, service_title
		FROM activities
		ORDER BY date
	`)
	if err != nil {
		return nil, err
	}
	return scanActivities(rows)
}

// scanActivities reads all rows from a *sql.Rows result set into a slice of Activity.
func scanActivities(rows *sql.Rows) ([]alteg.Activity, error) {
	defer rows.Close()

	var activities []alteg.Activity
	for rows.Next() {
		var a alteg.Activity
		if err := rows.Scan(
			&a.ID, &a.Date, &a.Capacity, &a.RecordsCount,
			&a.Staff.ID, &a.Staff.Name,
			&a.Service.ID, &a.Service.Title,
		); err != nil {
			return nil, err
		}
		activities = append(activities, a)
	}
	return activities, rows.Err()
}
