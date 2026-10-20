package store

import (
	"context"
	"database/sql"
)

// LeaderUser is everything the leaderboard needs about one user, loaded
// for all users at once (a few grouped queries instead of N per user).
type LeaderUser struct {
	User
	Streak     Streak
	Presence   *Presence // nil = the desktop client never reported
	ActiveDays int       // distinct counted days
	Failures   int       // missed deadlines
}

// LeaderUsers returns every user with streak, presence and the counters
// XP is derived from, ordered by id.
func (s *Store) LeaderUsers(ctx context.Context) ([]LeaderUser, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT u.id, u.name, u.timezone, u.github_login, u.github_repo, u.created_at, u.created_day,
		       COALESCE(s.current, 0), COALESCE(s.best, 0), COALESCE(s.last_active_day, ''),
		       p.app, p.since, p.idle, p.updated_at,
		       (SELECT COUNT(DISTINCT a.day) FROM activities a WHERE a.user_id = u.id),
		       (SELECT COUNT(*) FROM events e WHERE e.user_id = u.id AND e.kind = ?)
		FROM users u
		LEFT JOIN streaks s ON s.user_id = u.id
		LEFT JOIN presence p ON p.user_id = u.id
		ORDER BY u.id`, EventFailed)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LeaderUser
	for rows.Next() {
		var lu LeaderUser
		var created string
		var app, since, updated sql.NullString
		var idle sql.NullInt64
		if err := rows.Scan(&lu.ID, &lu.Name, &lu.Timezone, &lu.GitHubLogin, &lu.GitHubRepo, &created, &lu.CreatedDay,
			&lu.Streak.Current, &lu.Streak.Best, &lu.Streak.LastActiveDay,
			&app, &since, &idle, &updated,
			&lu.ActiveDays, &lu.Failures); err != nil {
			return nil, err
		}
		lu.CreatedAt = parseTS(created)
		lu.Streak.UserID = lu.ID
		if updated.Valid {
			p := &Presence{App: app.String, Idle: idle.Int64 == 1, UpdatedAt: parseTS(updated.String)}
			if since.String != "" {
				p.Since = parseTS(since.String)
			}
			lu.Presence = p
		}
		out = append(out, lu)
	}
	return out, rows.Err()
}

// UnlockedIDs returns the stored achievement IDs of every user.
func (s *Store) UnlockedIDs(ctx context.Context) (map[int64][]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT user_id, achievement_id FROM achievements_unlocked`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64][]string{}
	for rows.Next() {
		var uid int64
		var id string
		if err := rows.Scan(&uid, &id); err != nil {
			return nil, err
		}
		out[uid] = append(out[uid], id)
	}
	return out, rows.Err()
}

// DailyUsage returns the total tracked seconds per user per day for
// fromDay..toDay (inclusive): user ID → day → seconds.
func (s *Store) DailyUsage(ctx context.Context, fromDay, toDay string) (map[int64]map[string]int64, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT user_id, day, SUM(seconds) FROM app_usage
		 WHERE day >= ? AND day <= ? GROUP BY user_id, day`, fromDay, toDay)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]map[string]int64{}
	for rows.Next() {
		var uid, sec int64
		var day string
		if err := rows.Scan(&uid, &day, &sec); err != nil {
			return nil, err
		}
		if out[uid] == nil {
			out[uid] = map[string]int64{}
		}
		out[uid][day] = sec
	}
	return out, rows.Err()
}
