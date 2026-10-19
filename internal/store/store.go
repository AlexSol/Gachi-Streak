// Package store persists users, streaks, activity log and punishment events
// in SQLite through database/sql. The SQL is plain enough to port to
// PostgreSQL later (swap AUTOINCREMENT for IDENTITY and ? for $n).
package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/AlexSol/gachi-streak/internal/streak"
)

// ErrNotFound is returned when a row does not exist.
var ErrNotFound = errors.New("not found")

// Event kinds.
const (
	EventWarning = "warning"
	EventFailed  = "failed"
)

// Activity sources.
const (
	SourceManual   = "manual"
	SourceLocalGit = "local_git" // client saw a commit in a local repo
	SourceGitHub   = "github"
)

// User is a participant.
type User struct {
	ID          int64
	Name        string
	Timezone    string
	GitHubLogin string
	GitHubRepo  string // optional "owner/repo" filter for auto-check
	CreatedAt   time.Time
	CreatedDay  string // first day the user is accountable for
}

// Location returns the user's time zone, falling back to UTC.
func (u User) Location() *time.Location {
	loc, err := time.LoadLocation(u.Timezone)
	if err != nil {
		return time.UTC
	}
	return loc
}

// Streak is the current streak of a user.
type Streak struct {
	UserID        int64
	Current       int
	Best          int
	LastActiveDay string
	UpdatedAt     time.Time
}

// Event is a logged punishment.
type Event struct {
	ID        int64
	UserID    int64
	Kind      string
	Day       string
	CreatedAt time.Time
}

// Store wraps the database handle.
type Store struct {
	db *sql.DB
}

// Open opens (and creates) the SQLite database at path and runs migrations.
func Open(ctx context.Context, path string) (*Store, error) {
	db, err := sql.Open(driverName, dsn(path))
	if err != nil {
		return nil, err
	}
	// SQLite allows one writer at a time; one connection avoids SQLITE_BUSY
	// juggling and is more than enough for two users.
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate(ctx context.Context) error {
	for _, stmt := range strings.Split(schema, ";\n") {
		if strings.TrimSpace(stmt) == "" {
			continue
		}
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("%w\n%s", err, stmt)
		}
	}
	return nil
}

const schema = `
CREATE TABLE IF NOT EXISTS users (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	name         TEXT NOT NULL UNIQUE,
	token_hash   TEXT NOT NULL UNIQUE,
	timezone     TEXT NOT NULL DEFAULT 'UTC',
	github_login TEXT NOT NULL DEFAULT '',
	github_repo  TEXT NOT NULL DEFAULT '',
	created_at   TEXT NOT NULL,
	created_day  TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS streaks (
	user_id         INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
	current         INTEGER NOT NULL DEFAULT 0,
	best            INTEGER NOT NULL DEFAULT 0,
	last_active_day TEXT NOT NULL DEFAULT '',
	updated_at      TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS activities (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	day        TEXT NOT NULL,
	source     TEXT NOT NULL,
	note       TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_activities_user_day ON activities(user_id, day);
CREATE TABLE IF NOT EXISTS events (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	kind       TEXT NOT NULL,
	day        TEXT NOT NULL,
	created_at TEXT NOT NULL,
	UNIQUE(user_id, kind, day)
);
CREATE TABLE IF NOT EXISTS sessions (
	id_hash    TEXT PRIMARY KEY,
	user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	created_at TEXT NOT NULL,
	expires_at TEXT NOT NULL,
	user_agent TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);
CREATE TABLE IF NOT EXISTS app_usage (
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	day     TEXT NOT NULL,
	app     TEXT NOT NULL,
	seconds INTEGER NOT NULL,
	PRIMARY KEY (user_id, day, app)
);
CREATE TABLE IF NOT EXISTS presence (
	user_id    INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
	app        TEXT NOT NULL DEFAULT '',
	since      TEXT NOT NULL DEFAULT '',
	idle       INTEGER NOT NULL DEFAULT 0,
	updated_at TEXT NOT NULL
);
`

// Times are stored as RFC3339 UTC text: portable across drivers and readable.
func ts(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func parseTS(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}

// HashToken returns the stored form of an API token.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// NewToken generates a random API token.
func NewToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "gs_" + hex.EncodeToString(b), nil
}

// CreateUser adds a participant and returns its plain token (shown once).
func (s *Store) CreateUser(ctx context.Context, u User, now time.Time) (User, string, error) {
	if _, err := time.LoadLocation(u.Timezone); err != nil {
		return User{}, "", fmt.Errorf("bad timezone %q: %w", u.Timezone, err)
	}
	token, err := NewToken()
	if err != nil {
		return User{}, "", err
	}
	u.CreatedAt = now.UTC()
	u.CreatedDay = streak.DayOf(now, u.Location())

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, "", err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`INSERT INTO users(name, token_hash, timezone, github_login, github_repo, created_at, created_day)
		 VALUES(?, ?, ?, ?, ?, ?, ?)`,
		u.Name, HashToken(token), u.Timezone, u.GitHubLogin, u.GitHubRepo, ts(u.CreatedAt), u.CreatedDay)
	if err != nil {
		return User{}, "", err
	}
	if u.ID, err = res.LastInsertId(); err != nil {
		return User{}, "", err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO streaks(user_id, updated_at) VALUES(?, ?)`, u.ID, ts(now)); err != nil {
		return User{}, "", err
	}
	return u, token, tx.Commit()
}

// RotateToken issues a new token for a user, invalidating the old one.
func (s *Store) RotateToken(ctx context.Context, name string) (string, error) {
	token, err := NewToken()
	if err != nil {
		return "", err
	}
	res, err := s.db.ExecContext(ctx, `UPDATE users SET token_hash = ? WHERE name = ?`, HashToken(token), name)
	if err != nil {
		return "", err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return "", ErrNotFound
	}
	// A new invite token logs the user out of every browser.
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM sessions WHERE user_id = (SELECT id FROM users WHERE name = ?)`, name); err != nil {
		return "", err
	}
	return token, nil
}

// UpdateGitHub sets the GitHub login and optional repo filter for a user.
func (s *Store) UpdateGitHub(ctx context.Context, name, login, repo string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE users SET github_login = ?, github_repo = ? WHERE name = ?`, login, repo, name)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

const userCols = `id, name, timezone, github_login, github_repo, created_at, created_day`

func scanUser(row interface{ Scan(...any) error }) (User, error) {
	var u User
	var created string
	err := row.Scan(&u.ID, &u.Name, &u.Timezone, &u.GitHubLogin, &u.GitHubRepo, &created, &u.CreatedDay)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	u.CreatedAt = parseTS(created)
	return u, err
}

// UserByToken finds the user owning a plain token.
func (s *Store) UserByToken(ctx context.Context, token string) (User, error) {
	return scanUser(s.db.QueryRowContext(ctx,
		`SELECT `+userCols+` FROM users WHERE token_hash = ?`, HashToken(token)))
}

// ListUsers returns all users ordered by id.
func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+userCols+` FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// GetStreak returns the streak row of a user.
func (s *Store) GetStreak(ctx context.Context, userID int64) (Streak, error) {
	return getStreak(ctx, s.db, userID)
}

type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func getStreak(ctx context.Context, q querier, userID int64) (Streak, error) {
	st := Streak{UserID: userID}
	var updated string
	err := q.QueryRowContext(ctx,
		`SELECT current, best, last_active_day, updated_at FROM streaks WHERE user_id = ?`, userID).
		Scan(&st.Current, &st.Best, &st.LastActiveDay, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return st, ErrNotFound
	}
	st.UpdatedAt = parseTS(updated)
	return st, err
}

// RecordActivity logs an activity and advances the streak.
// counted is false if today was already counted before.
func (s *Store) RecordActivity(ctx context.Context, u User, source, note string, now time.Time) (st Streak, counted bool, err error) {
	today := streak.DayOf(now, u.Location())

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Streak{}, false, err
	}
	defer tx.Rollback()

	if _, err = tx.ExecContext(ctx,
		`INSERT INTO activities(user_id, day, source, note, created_at) VALUES(?, ?, ?, ?, ?)`,
		u.ID, today, source, note, ts(now)); err != nil {
		return Streak{}, false, err
	}
	st, err = getStreak(ctx, tx, u.ID)
	if err != nil {
		return Streak{}, false, err
	}
	st.Current, st.Best, counted = streak.ApplyPing(st.LastActiveDay, today, st.Current, st.Best)
	if counted {
		st.LastActiveDay = today
		st.UpdatedAt = now.UTC()
		if _, err = tx.ExecContext(ctx,
			`UPDATE streaks SET current = ?, best = ?, last_active_day = ?, updated_at = ? WHERE user_id = ?`,
			st.Current, st.Best, st.LastActiveDay, ts(now), u.ID); err != nil {
			return Streak{}, false, err
		}
	}
	return st, counted, tx.Commit()
}

// LogEvent inserts an event once per (user, kind, day).
// inserted is false if it already existed.
func (s *Store) LogEvent(ctx context.Context, userID int64, kind, day string, now time.Time) (inserted bool, err error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO events(user_id, kind, day, created_at) VALUES(?, ?, ?, ?)
		 ON CONFLICT(user_id, kind, day) DO NOTHING`,
		userID, kind, day, ts(now))
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// MarkFailed records a missed deadline and resets the streak to zero.
func (s *Store) MarkFailed(ctx context.Context, userID int64, missedDay string, now time.Time) (inserted bool, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`INSERT INTO events(user_id, kind, day, created_at) VALUES(?, ?, ?, ?)
		 ON CONFLICT(user_id, kind, day) DO NOTHING`,
		userID, EventFailed, missedDay, ts(now))
	if err != nil {
		return false, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return false, nil
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE streaks SET current = 0, updated_at = ? WHERE user_id = ?`, ts(now), userID); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// LastEvent returns the newest event of a kind for a user.
func (s *Store) LastEvent(ctx context.Context, userID int64, kind string) (Event, error) {
	e := Event{UserID: userID, Kind: kind}
	var created string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, day, created_at FROM events WHERE user_id = ? AND kind = ? ORDER BY id DESC LIMIT 1`,
		userID, kind).Scan(&e.ID, &e.Day, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return e, ErrNotFound
	}
	e.CreatedAt = parseTS(created)
	return e, err
}
