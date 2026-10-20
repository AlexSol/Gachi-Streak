package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Relation is how another user relates to the viewer.
type Relation string

const (
	RelNone     Relation = ""         // strangers
	RelFriend   Relation = "friend"   // request accepted (either side sent it)
	RelIncoming Relation = "incoming" // they asked the viewer, waiting for an answer
	RelOutgoing Relation = "outgoing" // the viewer asked them, waiting for an answer
)

// Friendship statuses as stored. A declined or cancelled request is
// deleted, so the same person can ask again later.
const (
	friendPending  = "pending"
	friendAccepted = "accepted"
)

// FriendRequest is a pending request as one side sees it: the other
// person and when the request was sent.
type FriendRequest struct {
	UserID    int64
	Name      string
	Streak    int
	Best      int
	CreatedAt time.Time
}

// FriendRepo is the repository of the friendships table.
//
// One row per pair of users, whichever direction the request went: the
// unique index on (min(a, b), max(a, b)) forbids a second row for the
// same pair. (PostgreSQL: LEAST / GREATEST.)
type FriendRepo struct {
	db *sql.DB
}

// Friends returns the friendships repository.
func (s *Store) Friends() *FriendRepo { return &FriendRepo{db: s.db} }

// pairRow loads the row of a pair in either direction.
func pairRow(ctx context.Context, tx *sql.Tx, a, b int64) (requester int64, status string, err error) {
	err = tx.QueryRowContext(ctx,
		`SELECT requester_id, status FROM friendships
		 WHERE (requester_id = ? AND addressee_id = ?) OR (requester_id = ? AND addressee_id = ?)`,
		a, b, b, a).Scan(&requester, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", ErrNotFound
	}
	return requester, status, err
}

// Request sends a friend request from one user to another and returns the
// resulting relation (as from sees to). It is idempotent: asking an
// existing friend or asking twice changes nothing. If to has already
// asked from, the two requests meet and become a friendship.
func (r *FriendRepo) Request(ctx context.Context, from, to int64, now time.Time) (Relation, error) {
	if from == to {
		return RelNone, errors.New("cannot befriend yourself")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return RelNone, err
	}
	defer tx.Rollback()

	requester, status, err := pairRow(ctx, tx, from, to)
	switch {
	case errors.Is(err, ErrNotFound):
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO friendships(requester_id, addressee_id, status, created_at) VALUES(?, ?, ?, ?)`,
			from, to, friendPending, ts(now)); err != nil {
			return RelNone, err
		}
		return RelOutgoing, tx.Commit()
	case err != nil:
		return RelNone, err
	case status == friendAccepted:
		return RelFriend, nil
	case requester == from:
		return RelOutgoing, nil
	}
	// Their request is waiting for us: asking back means yes.
	if _, err := tx.ExecContext(ctx,
		`UPDATE friendships SET status = ?, responded_at = ? WHERE requester_id = ? AND addressee_id = ?`,
		friendAccepted, ts(now), to, from); err != nil {
		return RelNone, err
	}
	return RelFriend, tx.Commit()
}

// Accept accepts the pending request from someone to me.
func (r *FriendRepo) Accept(ctx context.Context, me, from int64, now time.Time) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE friendships SET status = ?, responded_at = ?
		 WHERE requester_id = ? AND addressee_id = ? AND status = ?`,
		friendAccepted, ts(now), from, me, friendPending)
	return affected(res, err)
}

// Decline drops the pending request from someone to me.
func (r *FriendRepo) Decline(ctx context.Context, me, from int64) error {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM friendships WHERE requester_id = ? AND addressee_id = ? AND status = ?`,
		from, me, friendPending)
	return affected(res, err)
}

// Cancel withdraws my pending request to someone.
func (r *FriendRepo) Cancel(ctx context.Context, me, to int64) error {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM friendships WHERE requester_id = ? AND addressee_id = ? AND status = ?`,
		me, to, friendPending)
	return affected(res, err)
}

// Remove ends a friendship, whoever sent the original request.
func (r *FriendRepo) Remove(ctx context.Context, me, other int64) error {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM friendships
		 WHERE status = ? AND ((requester_id = ? AND addressee_id = ?) OR (requester_id = ? AND addressee_id = ?))`,
		friendAccepted, me, other, other, me)
	return affected(res, err)
}

// affected turns "no row changed" into ErrNotFound.
func affected(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Relations returns how every user I have a row with relates to me,
// keyed by their user ID. Users missing from the map are strangers.
func (r *FriendRepo) Relations(ctx context.Context, me int64) (map[int64]Relation, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT requester_id, addressee_id, status FROM friendships
		 WHERE requester_id = ? OR addressee_id = ?`, me, me)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]Relation{}
	for rows.Next() {
		var from, to int64
		var status string
		if err := rows.Scan(&from, &to, &status); err != nil {
			return nil, err
		}
		switch {
		case status == friendAccepted && from == me:
			out[to] = RelFriend
		case status == friendAccepted:
			out[from] = RelFriend
		case from == me:
			out[to] = RelOutgoing
		default:
			out[from] = RelIncoming
		}
	}
	return out, rows.Err()
}

// Requests returns my pending requests, newest first: the ones sent to me
// and the ones I sent.
func (r *FriendRepo) Requests(ctx context.Context, me int64) (incoming, outgoing []FriendRequest, err error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT f.requester_id = ?, u.id, u.name, COALESCE(s.current, 0), COALESCE(s.best, 0), f.created_at
		FROM friendships f
		JOIN users u ON u.id = CASE WHEN f.requester_id = ? THEN f.addressee_id ELSE f.requester_id END
		LEFT JOIN streaks s ON s.user_id = u.id
		WHERE f.status = ? AND (f.requester_id = ? OR f.addressee_id = ?)
		ORDER BY f.created_at DESC, u.name`,
		me, me, friendPending, me, me)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var mine bool
		var fr FriendRequest
		var created string
		if err := rows.Scan(&mine, &fr.UserID, &fr.Name, &fr.Streak, &fr.Best, &created); err != nil {
			return nil, nil, err
		}
		fr.CreatedAt = parseTS(created)
		if mine {
			outgoing = append(outgoing, fr)
		} else {
			incoming = append(incoming, fr)
		}
	}
	return incoming, outgoing, rows.Err()
}
