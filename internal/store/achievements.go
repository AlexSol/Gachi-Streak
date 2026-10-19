package store

import (
	"context"
	"database/sql"
	"time"
)

// Unlock is an achievement a user has earned. Once stored it stays
// unlocked, even if the rule that granted it changes later.
type Unlock struct {
	UserID        int64
	AchievementID string
	Day           string    // day it was earned (user's time zone)
	UnlockedAt    time.Time // when the server noticed it
	SeenAt        time.Time // zero until the user has been notified
}

// AchievementRepo is the repository of the achievements_unlocked table.
type AchievementRepo struct {
	db *sql.DB
}

// Achievements returns the achievement repository.
func (s *Store) Achievements() *AchievementRepo { return &AchievementRepo{db: s.db} }

// ListByUser returns every unlock of a user, oldest day first.
func (r *AchievementRepo) ListByUser(ctx context.Context, userID int64) ([]Unlock, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT achievement_id, day, unlocked_at, seen_at FROM achievements_unlocked
		 WHERE user_id = ? ORDER BY day, achievement_id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Unlock
	for rows.Next() {
		u := Unlock{UserID: userID}
		var unlocked, seen string
		if err := rows.Scan(&u.AchievementID, &u.Day, &unlocked, &seen); err != nil {
			return nil, err
		}
		u.UnlockedAt = parseTS(unlocked)
		if seen != "" {
			u.SeenAt = parseTS(seen)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// Unlock stores an achievement once per user.
// inserted is false if the user already had it.
func (r *AchievementRepo) Unlock(ctx context.Context, userID int64, achievementID, day string, now time.Time) (inserted bool, err error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO achievements_unlocked(user_id, achievement_id, day, unlocked_at) VALUES(?, ?, ?, ?)
		 ON CONFLICT(user_id, achievement_id) DO NOTHING`,
		userID, achievementID, day, ts(now))
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// MarkSeen marks every unseen unlock of a user as seen and returns how many.
func (r *AchievementRepo) MarkSeen(ctx context.Context, userID int64, now time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE achievements_unlocked SET seen_at = ? WHERE user_id = ? AND seen_at = ''`, ts(now), userID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// DeleteByUser removes every unlock of a user and returns how many.
func (r *AchievementRepo) DeleteByUser(ctx context.Context, userID int64) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM achievements_unlocked WHERE user_id = ?`, userID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
