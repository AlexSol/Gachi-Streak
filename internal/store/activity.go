package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Presence is the stored current program of a user.
type Presence struct {
	App       string
	Since     time.Time
	Idle      bool
	UpdatedAt time.Time
}

// SavePresence stores the current program and the day's totals in one go.
// Totals only grow (MAX): a client that lost its local file can't erase time.
func (s *Store) SavePresence(ctx context.Context, userID int64, p Presence, day string, usage map[string]int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	since := ""
	if !p.Since.IsZero() {
		since = ts(p.Since)
	}
	idle := 0
	if p.Idle {
		idle = 1
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO presence(user_id, app, since, idle, updated_at) VALUES(?, ?, ?, ?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET app = excluded.app, since = excluded.since,
		   idle = excluded.idle, updated_at = excluded.updated_at`,
		userID, p.App, since, idle, ts(p.UpdatedAt)); err != nil {
		return err
	}
	for app, sec := range usage {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO app_usage(user_id, day, app, seconds) VALUES(?, ?, ?, ?)
			 ON CONFLICT(user_id, day, app) DO UPDATE SET seconds = MAX(seconds, excluded.seconds)`,
			userID, day, app, sec); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetPresence returns the last reported presence of a user.
func (s *Store) GetPresence(ctx context.Context, userID int64) (Presence, error) {
	var p Presence
	var since, updated string
	var idle int
	err := s.db.QueryRowContext(ctx,
		`SELECT app, since, idle, updated_at FROM presence WHERE user_id = ?`, userID).
		Scan(&p.App, &since, &idle, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrNotFound
	}
	if since != "" {
		p.Since = parseTS(since)
	}
	p.Idle = idle == 1
	p.UpdatedAt = parseTS(updated)
	return p, err
}

// AppUsage is time in one program.
type AppUsage struct {
	Day     string
	App     string
	Seconds int64
}

// UsageRange returns per-day per-app rows for fromDay..toDay (inclusive).
func (s *Store) UsageRange(ctx context.Context, userID int64, fromDay, toDay string) ([]AppUsage, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT day, app, seconds FROM app_usage
		 WHERE user_id = ? AND day >= ? AND day <= ? ORDER BY day, seconds DESC`,
		userID, fromDay, toDay)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AppUsage
	for rows.Next() {
		var u AppUsage
		if err := rows.Scan(&u.Day, &u.App, &u.Seconds); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
