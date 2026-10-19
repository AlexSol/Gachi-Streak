package store

import (
	"context"
	"database/sql"
	"time"
)

// CodeFinding is how many times a construct (codescan kind) appeared in
// the code a user pushed on a day.
type CodeFinding struct {
	Kind  string
	Day   string
	Count int
}

// CodeRepo is the repository of the code_scans and code_findings tables:
// which pushes were analyzed and what was found in them.
type CodeRepo struct {
	db *sql.DB
}

// Code returns the code analysis repository.
func (s *Store) Code() *CodeRepo { return &CodeRepo{db: s.db} }

// Scanned reports whether ref (e.g. "owner/repo@sha") was analyzed already.
func (r *CodeRepo) Scanned(ctx context.Context, userID int64, ref string) (bool, error) {
	var n int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM code_scans WHERE user_id = ? AND ref = ?`, userID, ref).Scan(&n)
	return n > 0, err
}

// SaveScan records an analyzed ref with its findings, once per ref.
// inserted is false if the ref was saved before (nothing is changed then).
func (r *CodeRepo) SaveScan(ctx context.Context, userID int64, ref, day string, found map[string]int, now time.Time) (inserted bool, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`INSERT INTO code_scans(user_id, ref, day, scanned_at) VALUES(?, ?, ?, ?)
		 ON CONFLICT(user_id, ref) DO NOTHING`, userID, ref, day, ts(now))
	if err != nil {
		return false, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return false, nil
	}
	for kind, count := range found {
		if count <= 0 {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO code_findings(user_id, ref, kind, day, count) VALUES(?, ?, ?, ?, ?)`,
			userID, ref, kind, day, count); err != nil {
			return false, err
		}
	}
	return true, tx.Commit()
}

// Findings returns a user's findings summed per kind and day, by day.
func (r *CodeRepo) Findings(ctx context.Context, userID int64) ([]CodeFinding, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT kind, day, SUM(count) FROM code_findings WHERE user_id = ?
		 GROUP BY kind, day ORDER BY day, kind`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CodeFinding
	for rows.Next() {
		var f CodeFinding
		if err := rows.Scan(&f.Kind, &f.Day, &f.Count); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
