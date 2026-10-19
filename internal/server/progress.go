package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/AlexSol/gachi-streak/internal/api"
	"github.com/AlexSol/gachi-streak/internal/progress"
	"github.com/AlexSol/gachi-streak/internal/store"
)

// AchievementRepository stores the achievements users have earned.
type AchievementRepository interface {
	ListByUser(ctx context.Context, userID int64) ([]store.Unlock, error)
	Unlock(ctx context.Context, userID int64, achievementID, day string, now time.Time) (inserted bool, err error)
	MarkSeen(ctx context.Context, userID int64, now time.Time) (int64, error)
	MarkOpened(ctx context.Context, userID int64, achievementID string, now time.Time) error
}

// CodeRepository stores which pushes were analyzed and what was found.
type CodeRepository interface {
	Scanned(ctx context.Context, userID int64, ref string) (bool, error)
	SaveScan(ctx context.Context, userID int64, ref, day string, found map[string]int, now time.Time) (inserted bool, err error)
	Findings(ctx context.Context, userID int64) ([]store.CodeFinding, error)
}

// handleProgress returns the rank ladder, the achievement catalog and the
// XP / rank / achievements of every participant.
func (s *Server) handleProgress(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	users, err := s.computeProgress(r.Context(), me.ID)
	if err != nil {
		s.Log.Error("progress", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, api.ProgressResponse{
		Me:           me.Name,
		XPPerDay:     progress.XPPerDay,
		XPPerFailure: progress.XPPerFailure,
		Ranks:        progress.Ranks,
		Achievements: progress.Achievements,
		Users:        users,
	})
}

// handleAchievementsSeen marks the caller's new unlocks as notified.
func (s *Server) handleAchievementsSeen(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	if _, err := s.Achievements.MarkSeen(r.Context(), u.ID, s.Now()); err != nil {
		s.Log.Error("achievements seen", "user", u.Name, "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleAchievementOpen records that the caller unpacked an achievement
// in the web app (plays the opening effect only once).
func (s *Server) handleAchievementOpen(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	id := r.PathValue("id")
	err := s.Achievements.MarkOpened(r.Context(), u.ID, id, s.Now())
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "achievement not unlocked")
		return
	}
	if err != nil {
		s.Log.Error("achievement open", "user", u.Name, "id", id, "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// computeProgress derives every user's progress, stores achievements that
// were just earned and returns the result. Unlocks of viewerID (0 = none)
// that were not seen yet are flagged New.
func (s *Server) computeProgress(ctx context.Context, viewerID int64) ([]api.UserProgress, error) {
	users, err := s.Store.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	hist := make([]progress.History, len(users))
	unseen := map[string]bool{}
	unopened := map[string]bool{} // viewer's unlocks not unpacked yet
	for i, u := range users {
		unlocks, err := s.Achievements.ListByUser(ctx, u.ID)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", u.Name, err)
		}
		if hist[i], err = s.history(ctx, u); err != nil {
			return nil, fmt.Errorf("%s: %w", u.Name, err)
		}
		hist[i].Earned = map[string]string{}
		for _, un := range unlocks {
			hist[i].Earned[un.AchievementID] = un.Day
			if u.ID == viewerID && un.SeenAt.IsZero() {
				unseen[un.AchievementID] = true
			}
			if u.ID == viewerID && un.OpenedAt.IsZero() {
				unopened[un.AchievementID] = true
			}
		}
	}

	now := s.Now()
	out := make([]api.UserProgress, 0, len(users))
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
		up := progress.Compute(u.Name, hist[i], partner)
		up.CodeTracking = u.GitHubLogin != ""
		for k, st := range up.Achievements {
			if !st.Unlocked {
				continue
			}
			if _, stored := hist[i].Earned[st.ID]; !stored {
				inserted, err := s.Achievements.Unlock(ctx, u.ID, st.ID, st.Day, now)
				if err != nil {
					return nil, fmt.Errorf("%s: unlock %s: %w", u.Name, st.ID, err)
				}
				if inserted {
					s.Log.Info("achievement unlocked", "user", u.Name, "id", st.ID, "day", st.Day)
					if u.ID == viewerID {
						unseen[st.ID] = true
						unopened[st.ID] = true
					}
				}
			}
			if u.ID == viewerID {
				up.Achievements[k].New = unseen[st.ID]
				up.Achievements[k].Packed = unopened[st.ID]
			}
		}
		out = append(out, up)
	}
	return out, nil
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
	found, err := s.Code.Findings(ctx, u.ID)
	if err != nil {
		return progress.History{}, err
	}
	loc := u.Location()
	for i := range times {
		times[i] = times[i].In(loc)
	}
	h := progress.History{Days: days, Times: times, Failures: fails}
	for _, f := range found {
		h.Code = append(h.Code, progress.Finding{Kind: f.Kind, Day: f.Day, Count: f.Count})
	}
	return h, nil
}
