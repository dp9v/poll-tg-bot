package storage

import (
	"context"
	"database/sql"
	"fmt"
	"go-notification-tg-bot/internal/alteg"
	"go-notification-tg-bot/internal/storage/migrations"
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

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres db: %w", err)
	}

	if err := migrations.Up(ctx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate postgres db: %w", err)
	}

	return &Storage{db: db}, nil
}

// Close closes the underlying database connection.
func (s *Storage) Close() error {
	return s.db.Close()
}

// Save upserts the given activities, along with the staff, categories and
// services they reference, into the database in a single transaction.
//
// Inserts are performed in dependency order: categories → services → staff →
// activities. Existing rows are updated by id (upsert). Activities that are no
// longer returned by the API are NOT removed; they remain as historical records.
func (s *Storage) Save(activities []alteg.Activity) error {
	if len(activities) == 0 {
		return nil
	}
	return s.saveInTx(activities, nil, nil)
}

// SaveWindow replaces all activities in the given date range [from, to] with
// the provided set. Activities that previously existed in the window but are
// absent from the new batch are deleted. This is the correct method when
// syncing a complete time window from the API.
func (s *Storage) SaveWindow(activities []alteg.Activity, from, to time.Time) error {
	return s.saveInTx(activities, &from, &to)
}

// saveInTx performs the actual upsert (and optional window-based delete) in a
// single transaction.
func (s *Storage) saveInTx(activities []alteg.Activity, from, to *time.Time) error {
	// Deduplicate referenced metadata so we touch each row at most once.
	staffByID := make(map[int]alteg.Staff)
	servicesByID := make(map[int]alteg.Service)
	categoriesByID := make(map[int]alteg.Category)
	for _, a := range activities {
		if a.Staff.ID != 0 {
			staffByID[a.Staff.ID] = a.Staff
		}
		if a.Service.ID != 0 {
			servicesByID[a.Service.ID] = a.Service
		}
		if a.Service.Category.ID != 0 {
			categoriesByID[a.Service.Category.ID] = a.Service.Category
		}
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err := upsertCategories(tx, categoriesByID); err != nil {
		return fmt.Errorf("upsert categories: %w", err)
	}
	if err := upsertServices(tx, servicesByID); err != nil {
		return fmt.Errorf("upsert services: %w", err)
	}
	if err := upsertStaff(tx, staffByID); err != nil {
		return fmt.Errorf("upsert staff: %w", err)
	}
	if err := upsertActivities(tx, activities); err != nil {
		return fmt.Errorf("upsert activities: %w", err)
	}

	// Delete activities that are in the window but not in the new batch.
	if from != nil && to != nil {
		if err := deleteStaleActivities(tx, activities, *from, *to); err != nil {
			return fmt.Errorf("delete stale activities: %w", err)
		}
	}

	return tx.Commit()
}

func upsertCategories(tx *sql.Tx, items map[int]alteg.Category) error {
	if len(items) == 0 {
		return nil
	}
	stmt, err := tx.Prepare(`
		INSERT INTO categories (id, title, parent_id, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (id) DO UPDATE SET
			title      = EXCLUDED.title,
			parent_id  = EXCLUDED.parent_id,
			updated_at = NOW()
	`)
	if err != nil {
		return err
	}
	defer func() { _ = stmt.Close() }()

	for _, c := range items {
		if _, err := stmt.Exec(c.ID, c.Title, c.ParentID); err != nil {
			return err
		}
	}
	return nil
}

func upsertServices(tx *sql.Tx, items map[int]alteg.Service) error {
	if len(items) == 0 {
		return nil
	}
	stmt, err := tx.Prepare(`
		INSERT INTO services (id, title, category_id, price_min, price_max, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (id) DO UPDATE SET
			title       = EXCLUDED.title,
			category_id = EXCLUDED.category_id,
			price_min   = EXCLUDED.price_min,
			price_max   = EXCLUDED.price_max,
			updated_at  = NOW()
	`)
	if err != nil {
		return err
	}
	defer func() { _ = stmt.Close() }()

	for _, sv := range items {
		categoryID := sv.CategoryID
		if categoryID == 0 {
			categoryID = sv.Category.ID
		}
		if _, err := stmt.Exec(sv.ID, sv.Title, categoryID, sv.PriceMin, sv.PriceMax); err != nil {
			return err
		}
	}
	return nil
}

func upsertStaff(tx *sql.Tx, items map[int]alteg.Staff) error {
	if len(items) == 0 {
		return nil
	}
	stmt, err := tx.Prepare(`
		INSERT INTO staff (id, name, specialization, avatar, rating, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (id) DO UPDATE SET
			name           = EXCLUDED.name,
			specialization = EXCLUDED.specialization,
			avatar         = EXCLUDED.avatar,
			rating         = EXCLUDED.rating,
			updated_at     = NOW()
	`)
	if err != nil {
		return err
	}
	defer func() { _ = stmt.Close() }()

	for _, st := range items {
		if _, err := stmt.Exec(st.ID, st.Name, st.Specialization, st.Avatar, st.Rating); err != nil {
			return err
		}
	}
	return nil
}

func upsertActivities(tx *sql.Tx, activities []alteg.Activity) error {
	if len(activities) == 0 {
		return nil
	}
	stmt, err := tx.Prepare(`
		INSERT INTO activities (id, date, capacity, records_count, staff_id, service_id)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (id) DO UPDATE SET
			date          = EXCLUDED.date,
			capacity      = EXCLUDED.capacity,
			records_count = EXCLUDED.records_count,
			staff_id      = EXCLUDED.staff_id,
			service_id    = EXCLUDED.service_id
	`)
	if err != nil {
		return err
	}
	defer func() { _ = stmt.Close() }()

	for _, a := range activities {
		if _, err := stmt.Exec(
			a.ID, a.Date, a.Capacity, a.RecordsCount,
			a.Staff.ID, a.Service.ID,
		); err != nil {
			return err
		}
	}
	return nil
}

// deleteStaleActivities removes activities within the given date window that
// are not present in the new batch.
func deleteStaleActivities(tx *sql.Tx, activities []alteg.Activity, from, to time.Time) error {
	const layout = "2006-01-02 15:04:05"
	if len(activities) == 0 {
		// Delete everything in the window.
		_, err := tx.Exec(`
			DELETE FROM activities
			WHERE date >= $1 AND date <= $2
		`, from.Format(layout), to.Format(layout))
		return err
	}

	// Collect IDs to keep.
	ids := make([]any, len(activities))
	placeholders := ""
	for i, a := range activities {
		ids[i] = a.ID
		if i > 0 {
			placeholders += ","
		}
		placeholders += fmt.Sprintf("$%d", i+3)
	}

	query := fmt.Sprintf(`
		DELETE FROM activities
		WHERE date >= $1 AND date <= $2
		  AND id NOT IN (%s)
	`, placeholders)

	args := make([]any, 0, 2+len(ids))
	args = append(args, from.Format(layout), to.Format(layout))
	args = append(args, ids...)
	_, err := tx.Exec(query, args...)
	return err
}

const selectActivitiesSQL = `
	SELECT a.id, a.date, a.capacity, a.records_count,
	       a.staff_id,   COALESCE(s.name,  ''),
	       a.service_id, COALESCE(sv.title, '')
	FROM activities a
	LEFT JOIN staff    s  ON s.id  = a.staff_id
	LEFT JOIN services sv ON sv.id = a.service_id
`

// LoadBetween returns activities whose date falls within [from, to] (inclusive).
// Dates are compared as strings in the format stored by the API (e.g. "2026-03-04 10:00:00").
func (s *Storage) LoadBetween(from, to time.Time) ([]alteg.Activity, error) {
	const layout = "2006-01-02 15:04:05"
	rows, err := s.db.Query(selectActivitiesSQL+`
		WHERE a.date >= $1 AND a.date <= $2
		ORDER BY a.date
	`, from.Format(layout), to.Format(layout))
	if err != nil {
		return nil, err
	}
	return scanActivities(rows)
}

// LoadBetweenForService returns activities for a specific service whose date
// falls within [from, to] (inclusive).
func (s *Storage) LoadBetweenForService(from, to time.Time, serviceID int) ([]alteg.Activity, error) {
	const layout = "2006-01-02 15:04:05"
	rows, err := s.db.Query(selectActivitiesSQL+`
		WHERE a.date >= $1 AND a.date <= $2 AND a.service_id = $3
		ORDER BY a.date
	`, from.Format(layout), to.Format(layout), serviceID)
	if err != nil {
		return nil, err
	}
	return scanActivities(rows)
}

// Load returns all activities currently stored in the database.
func (s *Storage) Load() ([]alteg.Activity, error) {
	rows, err := s.db.Query(selectActivitiesSQL + ` ORDER BY a.date`)
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
