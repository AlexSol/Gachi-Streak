package server

import (
	"context"
	"net/http"

	"github.com/AlexSol/gachi-streak/internal/api"
	"github.com/AlexSol/gachi-streak/internal/progress"
	"github.com/AlexSol/gachi-streak/internal/store"
)

// handleProgress returns the rank ladder, the achievement catalog and the
// XP / rank / achievements of every participant, derived from history.
func (s *Server) handleProgress(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	users, err := s.Store.ListUsers(r.Context())
	if err != nil {
		s.Log.Error("progress", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	hist := make([]progress.History, len(users))
	for i, u := range users {
		if hist[i], err = s.history(r.Context(), u); err != nil {
			s.Log.Error("progress", "user", u.Name, "err", err)
			writeErr(w, http.StatusInternalServerError, "internal error")
			return
		}
	}
	resp := api.ProgressResponse{
		Me:           me.Name,
		XPPerDay:     progress.XPPerDay,
		XPPerFailure: progress.XPPerFailure,
		Ranks:        progress.Ranks,
		Achievements: progress.Achievements,
	}
	for i, u := range users {
		// Duo achievements are counted against the other participant
		// (the service is built for two; with more users, the first other one).
		var partner *progress.History
		for j := range users {
			if j != i {
				partner = &hist[j]
				break
			}
		}
		resp.Users = append(resp.Users, progress.Compute(u.Name, hist[i], partner))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) history(ctx context.Context, u store.User) (progress.History, error) {
	days, times, err := s.Store.ActivityHistory(ctx, u.ID)
	if err != nil {
		return progress.History{}, err
	}
	fails, err := s.Store.EventDays(ctx, u.ID, store.EventFailed)
	if err != nil {
		return progress.History{}, err
	}
	loc := u.Location()
	for i := range times {
		times[i] = times[i].In(loc)
	}
	return progress.History{Days: days, Times: times, Failures: fails}, nil
}
