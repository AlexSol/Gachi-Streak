package store

import (
	"context"
	"time"
)

// CreateSession stores a new browser session for a user and returns the
// plain session id (goes into the cookie; only its hash is stored).
func (s *Store) CreateSession(ctx context.Context, userID int64, ttl time.Duration, userAgent string, now time.Time) (string, error) {
	id, err := NewToken()
	if err != nil {
		return "", err
	}
	if len(userAgent) > 300 {
		userAgent = userAgent[:300]
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO sessions(id_hash, user_id, created_at, expires_at, user_agent) VALUES(?, ?, ?, ?, ?)`,
		HashToken(id), userID, ts(now), ts(now.Add(ttl)), userAgent)
	return id, err
}

// UserBySession returns the user of a live (not expired) session.
func (s *Store) UserBySession(ctx context.Context, sessionID string, now time.Time) (User, error) {
	return scanUser(s.db.QueryRowContext(ctx,
		`SELECT u.id, u.name, u.timezone, u.github_login, u.github_repo, u.created_at, u.created_day
		 FROM sessions s JOIN users u ON u.id = s.user_id
		 WHERE s.id_hash = ? AND s.expires_at > ?`, HashToken(sessionID), ts(now)))
}

// DeleteSession removes one session (logout).
func (s *Store) DeleteSession(ctx context.Context, sessionID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id_hash = ?`, HashToken(sessionID))
	return err
}

// DeleteUserSessions logs a user out everywhere (e.g. after a token rotation).
func (s *Store) DeleteUserSessions(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID)
	return err
}

// PurgeExpiredSessions deletes expired sessions; returns how many.
func (s *Store) PurgeExpiredSessions(ctx context.Context, now time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, ts(now))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
