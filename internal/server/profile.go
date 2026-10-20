package server

import (
	"errors"
	"net/http"
	"slices"
	"sort"
	"time"

	"github.com/AlexSol/gachi-streak/internal/api"
	"github.com/AlexSol/gachi-streak/internal/progress"
	"github.com/AlexSol/gachi-streak/internal/store"
	"github.com/AlexSol/gachi-streak/internal/streak"
)

const (
	profileHeatmapDays = 365
	profileCompareDays = 30
	profileTopApps     = 5
	profileRecentAchs  = 5
	profileLogEntries  = 50
)

func (s *Server) handleUserProfile(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	target, err := s.Store.UserByName(r.Context(), r.PathValue("name"))
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "no such user")
		return
	}
	if err != nil {
		s.Log.Error("profile user", "name", r.PathValue("name"), "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	now := s.Now()
	status, err := s.userStatus(r.Context(), target, now)
	if err != nil {
		s.Log.Error("profile status", "user", target.Name, "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	relations, err := s.Friends.Relations(r.Context(), me.ID)
	if err != nil {
		s.Log.Error("profile relations", "user", me.Name, "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	relation := string(relations[target.ID])
	if target.ID == me.ID {
		relation = relMe
	}

	progressUsers, err := s.computeProgress(r.Context(), me.ID)
	if err != nil {
		s.Log.Error("profile progress", "user", target.Name, "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	var targetProgress *api.UserProgress
	for i := range progressUsers {
		if progressUsers[i].Name == target.Name {
			targetProgress = &progressUsers[i]
			break
		}
	}
	if targetProgress == nil {
		s.Log.Error("profile progress missing", "user", target.Name)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	rankName := ""
	for _, rank := range progress.Ranks {
		if rank.Level == targetProgress.Rank {
			rankName = rank.Name
			break
		}
	}

	today := streak.DayOf(now, target.Location())
	from := streak.AddDays(today, -(profileHeatmapDays - 1))
	marks, err := s.Store.DayMarks(r.Context(), target.ID, from, today)
	if err != nil {
		s.Log.Error("profile heatmap", "user", target.Name, "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	days := make([]api.ProfileDay, 0, profileHeatmapDays)
	for day := from; day <= today; day = streak.AddDays(day, 1) {
		mark := marks[day]
		days = append(days, api.ProfileDay{Day: day, Counted: mark.Counted, Missed: mark.Missed})
	}

	apps, err := s.profileApps(r, target.ID, today)
	if err != nil {
		s.Log.Error("profile apps", "user", target.Name, "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	recent, err := s.profileAchievements(r, target)
	if err != nil {
		s.Log.Error("profile achievements", "user", target.Name, "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	out := api.UserProfile{
		Name:               target.Name,
		Timezone:           target.Timezone,
		CreatedDay:         target.CreatedDay,
		Relation:           relation,
		Streak:             status.Streak,
		Best:               status.Best,
		ActiveDays:         targetProgress.ActiveDays,
		XP:                 targetProgress.XP,
		Rank:               targetProgress.Rank,
		RankName:           rankName,
		Achievements:       countUnlocked(targetProgress.Achievements),
		Failures:           targetProgress.Failures,
		Days:               days,
		TopApps:            apps,
		LatestAchievements: recent,
		SharedFriends:      []string{},
	}

	if target.ID == me.ID {
		out.LearningLog, err = s.profileLearningLog(r, target.ID)
		if err != nil {
			s.Log.Error("profile learning log", "user", target.Name, "err", err)
			writeErr(w, http.StatusInternalServerError, "internal error")
			return
		}
	} else {
		out.Comparison, err = s.profileComparison(r, me, target, now)
		if err != nil {
			s.Log.Error("profile comparison", "me", me.Name, "user", target.Name, "err", err)
			writeErr(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	if relation == string(store.RelFriend) || target.ID == me.ID {
		out.SharedFriends, err = s.profileSharedFriends(r, me.ID, target.ID)
		if err != nil {
			s.Log.Error("profile shared friends", "me", me.Name, "user", target.Name, "err", err)
			writeErr(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	writeJSON(w, http.StatusOK, out)
}

func (s *Server) profileApps(r *http.Request, userID int64, today string) ([]api.ProfileApp, error) {
	from := streak.AddDays(today, -(profileCompareDays - 1))
	rows, err := s.Store.UsageRange(r.Context(), userID, from, today)
	if err != nil {
		return nil, err
	}
	totals := make(map[string]int64)
	for _, row := range rows {
		totals[row.App] += row.Seconds
	}
	out := make([]api.ProfileApp, 0, len(totals))
	for app, seconds := range totals {
		out = append(out, api.ProfileApp{App: app, Seconds: seconds})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Seconds != out[j].Seconds {
			return out[i].Seconds > out[j].Seconds
		}
		return out[i].App < out[j].App
	})
	return out[:min(len(out), profileTopApps)], nil
}

func (s *Server) profileAchievements(r *http.Request, user store.User) ([]api.ProfileAchievement, error) {
	unlocks, err := s.Achievements.ListByUser(r.Context(), user.ID)
	if err != nil {
		return nil, err
	}
	defs := make(map[string]api.AchievementDef, len(progress.Achievements))
	for _, def := range progress.Achievements {
		defs[def.ID] = def
	}
	slices.Reverse(unlocks)
	out := make([]api.ProfileAchievement, 0, min(len(unlocks), profileRecentAchs))
	for _, unlock := range unlocks {
		def, ok := defs[unlock.AchievementID]
		if !ok {
			continue
		}
		out = append(out, api.ProfileAchievement{
			ID:       def.ID,
			Day:      unlock.Day,
			XP:       def.XP,
			Category: def.Category,
			Secret:   def.Secret,
		})
		if len(out) == profileRecentAchs {
			break
		}
	}
	return out, nil
}

func (s *Server) profileLearningLog(r *http.Request, userID int64) ([]api.ProfileLearningEntry, error) {
	entries, err := s.Store.LearningLog(r.Context(), userID, profileLogEntries)
	if err != nil {
		return nil, err
	}
	out := make([]api.ProfileLearningEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, api.ProfileLearningEntry{Day: entry.Day, Note: entry.Note})
	}
	return out, nil
}

func (s *Server) profileComparison(r *http.Request, me, target store.User, now time.Time) (*api.ProfileComparison, error) {
	meTo := streak.DayOf(now, me.Location())
	meFrom := streak.AddDays(meTo, -(profileCompareDays - 1))
	userTo := streak.DayOf(now, target.Location())
	userFrom := streak.AddDays(userTo, -(profileCompareDays - 1))

	meMarks, err := s.Store.DayMarks(r.Context(), me.ID, meFrom, meTo)
	if err != nil {
		return nil, err
	}
	userMarks, err := s.Store.DayMarks(r.Context(), target.ID, userFrom, userTo)
	if err != nil {
		return nil, err
	}
	meUsage, err := s.Store.UsageRange(r.Context(), me.ID, meFrom, meTo)
	if err != nil {
		return nil, err
	}
	userUsage, err := s.Store.UsageRange(r.Context(), target.ID, userFrom, userTo)
	if err != nil {
		return nil, err
	}
	meSeconds := make(map[string]int64)
	for _, row := range meUsage {
		meSeconds[row.Day] += row.Seconds
	}
	userSeconds := make(map[string]int64)
	for _, row := range userUsage {
		userSeconds[row.Day] += row.Seconds
	}

	out := &api.ProfileComparison{
		Days: make([]api.ProfileCompareDay, 0, profileCompareDays),
	}
	for i := 0; i < profileCompareDays; i++ {
		meDay := streak.AddDays(meFrom, i)
		userDay := streak.AddDays(userFrom, i)
		mm, um := meMarks[meDay], userMarks[userDay]
		ms, us := meSeconds[meDay], userSeconds[userDay]
		out.Days = append(out.Days, api.ProfileCompareDay{
			Day:         userDay,
			MeCounted:   mm.Counted,
			UserCounted: um.Counted,
			MeSeconds:   ms,
			UserSeconds: us,
		})
		if mm.Counted {
			out.Totals.MeCounted++
		}
		if um.Counted {
			out.Totals.UserCounted++
		}
		out.Totals.MeSeconds += ms
		out.Totals.UserSeconds += us
	}
	return out, nil
}

func (s *Server) profileSharedFriends(r *http.Request, meID, targetID int64) ([]string, error) {
	users, err := s.Store.ListUsers(r.Context())
	if err != nil {
		return nil, err
	}
	meRelations, err := s.Friends.Relations(r.Context(), meID)
	if err != nil {
		return nil, err
	}
	targetRelations := meRelations
	if targetID != meID {
		targetRelations, err = s.Friends.Relations(r.Context(), targetID)
		if err != nil {
			return nil, err
		}
	}
	out := make([]string, 0)
	for _, user := range users {
		if user.ID == meID || user.ID == targetID {
			continue
		}
		if meRelations[user.ID] == store.RelFriend && targetRelations[user.ID] == store.RelFriend {
			out = append(out, user.Name)
		}
	}
	sort.Strings(out)
	return out, nil
}

func countUnlocked(states []api.AchievementState) int {
	n := 0
	for _, state := range states {
		if state.Unlocked {
			n++
		}
	}
	return n
}
