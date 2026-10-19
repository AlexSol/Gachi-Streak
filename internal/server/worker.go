package server

import (
	"context"
	"time"

	"github.com/AlexSol/gachi-streak/internal/store"
	"github.com/AlexSol/gachi-streak/internal/streak"
)

// RunDeadlineWorker checks deadlines every interval until ctx is done:
// logs the warning event when a user enters the warning window and
// resets the streak (failed event) when a deadline was missed.
func (s *Server) RunDeadlineWorker(ctx context.Context, interval time.Duration) {
	s.CheckDeadlines(ctx)
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.CheckDeadlines(ctx)
		}
	}
}

// CheckDeadlines runs one pass over all users.
func (s *Server) CheckDeadlines(ctx context.Context) {
	now := s.Now()
	users, err := s.Store.ListUsers(ctx)
	if err != nil {
		s.Log.Error("worker: list users", "err", err)
		return
	}
	for _, u := range users {
		if err := s.checkUser(ctx, u, now); err != nil {
			s.Log.Error("worker", "user", u.Name, "err", err)
		}
	}
	if _, err := s.Store.PurgeExpiredSessions(ctx, now); err != nil {
		s.Log.Error("worker: purge sessions", "err", err)
	}
}

func (s *Server) checkUser(ctx context.Context, u store.User, now time.Time) error {
	st, err := s.Store.GetStreak(ctx, u.ID)
	if err != nil {
		return err
	}
	loc := u.Location()

	if day, missed := streak.MissedDay(now, loc, st.LastActiveDay, u.CreatedDay); missed {
		inserted, err := s.Store.MarkFailed(ctx, u.ID, day, now)
		if err != nil {
			return err
		}
		if inserted {
			s.Log.Warn("deadline missed, streak reset", "user", u.Name, "day", day, "lost", st.Current)
		}
	}

	ev := streak.Evaluate(now, loc, st.LastActiveDay, u.CreatedDay, s.Warn)
	if ev.State == streak.StateWarning {
		inserted, err := s.Store.LogEvent(ctx, u.ID, store.EventWarning, ev.Today, now)
		if err != nil {
			return err
		}
		if inserted {
			s.Log.Info("warning window entered", "user", u.Name, "left", ev.Left.Round(time.Minute))
		}
	}
	return nil
}
