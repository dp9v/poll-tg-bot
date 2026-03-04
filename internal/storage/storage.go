package storage

import (
	"database/sql"
	"fmt"
	"go-notification-tg-bot/internal/alteg"
	"time"

	_ "modernc.org/sqlite"
)

const defaultFilePath = "activities.db"

// Storage persists activities to a SQLite database.
type Storage struct {
	db *sql.DB
}

// New opens (or creates) the SQLite database at the given path and runs migrations.
// If filePath is empty, defaultFilePath is used.
func New(filePath string) (*Storage, error) {
	if filePath == "" {
		filePath = defaultFilePath
	}

	db, err := sql.Open("sqlite", filePath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}

	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate sqlite db: %w", err)
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
			id            INTEGER PRIMARY KEY,
			DATE          TEXT    NOT NULL,
			capacity      INTEGER NOT NULL,
			records_count INTEGER NOT NULL,
			staff_id      INTEGER NOT NULL,
			staff_name    TEXT    NOT NULL,
			service_id    INTEGER NOT NULL,
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
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			DATE          = excluded.date,
			capacity      = excluded.capacity,
			records_count = excluded.records_count,
			staff_id      = excluded.staff_id,
			staff_name    = excluded.staff_name,
			service_id    = excluded.service_id,
			service_title = excluded.service_title
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
		SELECT id, DATE, capacity, records_count, staff_id, staff_name, service_id, service_title
		FROM activities
		WHERE DATE >= ? AND DATE <= ?
		ORDER BY DATE
	`, from.Format(layout), to.Format(layout))
	if err != nil {
		return nil, err
	}
	return scanActivities(rows)
}

// Load returns all activities currently stored in the database.
func (s *Storage) Load() ([]alteg.Activity, error) {
	rows, err := s.db.Query(`
		SELECT id, DATE, capacity, records_count, staff_id, staff_name, service_id, service_title
		FROM activities
		ORDER BY DATE
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
