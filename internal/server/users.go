package server

import (
	"context"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/AlexSol/gachi-streak/internal/api"
	"github.com/AlexSol/gachi-streak/internal/progress"
	"github.com/AlexSol/gachi-streak/internal/store"
	"github.com/AlexSol/gachi-streak/internal/streak"
)

const (
	usersDefaultLimit = 50
	usersMaxLimit     = 100
	atRiskShown       = 4
	weekDays          = 7
)

// handleUsers returns the leaderboard: whose rows (?scope=friends|all),
// search (?q=), tab (?filter=), sort (?sort=) and page (?page=, ?limit=).
// The "friends" scope is the caller and their accepted friends; places
// are counted inside the scope. "Needs a push" (at_risk) is always taken
// from the friends scope.
//
// Everything is computed in memory from a few grouped queries. That is
// fine for thousands of users on SQLite; when it stops being fine, the
// filtering and sorting move into SQL.
func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	qv := r.URL.Query()

	scope := qv.Get("scope")
	if scope == "" {
		scope = "all"
	}
	if scope != "all" && scope != "friends" {
		writeErr(w, http.StatusBadRequest, "scope must be friends or all")
		return
	}
	filter := qv.Get("filter")
	if filter == "" {
		filter = "all"
	}
	if filter != "all" && filter != "online" && filter != "risk" {
		writeErr(w, http.StatusBadRequest, "filter must be all, online or risk")
		return
	}
	sortBy := qv.Get("sort")
	if sortBy == "" {
		sortBy = "streak"
	}
	less, ok := leaderSorts[sortBy]
	if !ok {
		writeErr(w, http.StatusBadRequest, "sort must be streak, best, xp, name or seen")
		return
	}
	limit := usersDefaultLimit
	if v := qv.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			writeErr(w, http.StatusBadRequest, "bad limit")
			return
		}
		limit = min(n, usersMaxLimit)
	}
	page := 1
	if v := qv.Get("page"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			writeErr(w, http.StatusBadRequest, "bad page")
			return
		}
		page = n
	}
	q := strings.TrimSpace(qv.Get("q"))
	if len([]rune(q)) > 64 {
		writeErr(w, http.StatusBadRequest, "query too long")
		return
	}

	all, err := s.leaderRows(r.Context(), s.Now(), me)
	if err != nil {
		s.Log.Error("users", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := api.UsersResponse{Me: me.Name, Scope: scope, Query: q, Filter: filter, Sort: sortBy, Limit: limit, Users: []api.LeaderRow{}}
	resp.Scopes.All = len(all)
	var rows, risk []api.LeaderRow
	for _, row := range all {
		friendly := inFriendScope(row)
		if friendly {
			resp.Scopes.Friends++
			if row.State == string(streak.StateWarning) {
				risk = append(risk, row)
			}
		}
		if row.Relation == string(store.RelIncoming) {
			resp.Scopes.Incoming++
		}
		if scope == "all" || friendly {
			rows = append(rows, row)
		}
	}

	slices.SortStableFunc(rows, less)
	for i := range rows {
		rows[i].Pos = i + 1
		if rows[i].Name == me.Name {
			row := rows[i]
			resp.MeRow = &row
		}
	}

	// Nearest deadline first, ties by name.
	slices.SortStableFunc(risk, byName)
	slices.SortStableFunc(risk, func(a, b api.LeaderRow) int { return cmpInt64(a.SecondsLeft, b.SecondsLeft) })
	resp.AtRisk = risk[:min(len(risk), atRiskShown)]
	if resp.AtRisk == nil {
		resp.AtRisk = []api.LeaderRow{}
	}

	match := matcher(q)
	var shown []api.LeaderRow
	for _, row := range rows {
		if !match(row.Name) {
			continue
		}
		online := row.Presence != nil && row.Presence.Online
		atRisk := row.State == string(streak.StateWarning)
		resp.Counts.All++
		if online {
			resp.Counts.Online++
		}
		if atRisk {
			resp.Counts.AtRisk++
		}
		if filter == "all" || (filter == "online" && online) || (filter == "risk" && atRisk) {
			shown = append(shown, row)
		}
	}

	resp.Total = len(shown)
	pages := max(1, (resp.Total+limit-1)/limit)
	resp.Page = min(page, pages)
	from := (resp.Page - 1) * limit
	resp.Users = append(resp.Users, shown[from:min(from+limit, resp.Total)]...)
	writeJSON(w, http.StatusOK, resp)
}

// inFriendScope reports whether a row belongs to the "friends" scope.
func inFriendScope(row api.LeaderRow) bool {
	return row.Relation == relMe || row.Relation == string(store.RelFriend)
}

// relMe marks the caller's own row (not a stored relation).
const relMe = "me"

// matcher returns a case-insensitive name filter. "@name" matches the
// name exactly, anything else is a substring search; "" matches all.
func matcher(q string) func(string) bool {
	q = strings.ToLower(q)
	if exact, ok := strings.CutPrefix(q, "@"); ok {
		return func(name string) bool { return strings.ToLower(name) == exact }
	}
	return func(name string) bool { return strings.Contains(strings.ToLower(name), q) }
}

// leaderSorts order rows best-first; ties fall back to the name.
var leaderSorts = map[string]func(a, b api.LeaderRow) int{
	"streak": func(a, b api.LeaderRow) int {
		return firstNonZero(b.Streak-a.Streak, b.Best-a.Best, byName(a, b))
	},
	"best": func(a, b api.LeaderRow) int {
		return firstNonZero(b.Best-a.Best, b.Streak-a.Streak, byName(a, b))
	},
	"xp": func(a, b api.LeaderRow) int {
		return firstNonZero(b.XP-a.XP, b.Streak-a.Streak, byName(a, b))
	},
	"name": byName,
	"seen": func(a, b api.LeaderRow) int {
		return firstNonZero(cmpInt64(seenKey(b), seenKey(a)), byName(a, b))
	},
}

func byName(a, b api.LeaderRow) int {
	return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
}

// seenKey ranks online users first, then by the last presence report.
func seenKey(r api.LeaderRow) int64 {
	if r.Presence == nil {
		return 0
	}
	k := r.Presence.UpdatedAt.Unix()
	if r.Presence.Online {
		k += 1 << 40
	}
	return k
}

func firstNonZero(vs ...int) int {
	for _, v := range vs {
		if v != 0 {
			return v
		}
	}
	return 0
}

func cmpInt64(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// leaderRows builds one row per user. XP comes from stored data (counted
// days, failures, unlocked achievements), which the deadline worker keeps
// fresh every minute, so no per-user history is replayed here.
func (s *Server) leaderRows(ctx context.Context, now time.Time, me store.User) ([]api.LeaderRow, error) {
	users, err := s.Store.LeaderUsers(ctx)
	if err != nil {
		return nil, err
	}
	rels, err := s.Friends.Relations(ctx, me.ID)
	if err != nil {
		return nil, err
	}
	unlocked, err := s.Store.UnlockedIDs(ctx)
	if err != nil {
		return nil, err
	}
	// Users' "today" spans ±1 day around UTC; load a range that covers
	// every time zone and pick each user's own 7 days from it.
	utcToday := streak.DayOf(now, time.UTC)
	usage, err := s.Store.DailyUsage(ctx, streak.AddDays(utcToday, -weekDays), streak.AddDays(utcToday, 1))
	if err != nil {
		return nil, err
	}

	locs := map[string]*time.Location{}
	out := make([]api.LeaderRow, 0, len(users))
	for _, u := range users {
		loc, ok := locs[u.Timezone]
		if !ok {
			loc = u.Location()
			locs[u.Timezone] = loc
		}
		ev := streak.Evaluate(now, loc, u.Streak.LastActiveDay, u.CreatedDay, s.Warn)
		xp, _ := progress.XPFor(u.ActiveDays, progress.AchievementXP(unlocked[u.ID]), u.Failures)
		rank, _ := progress.RankFor(xp)
		row := api.LeaderRow{
			Name:          u.Name,
			Streak:        u.Streak.Current,
			Best:          u.Streak.Best,
			State:         string(ev.State),
			DoneToday:     ev.DoneToday,
			Deadline:      ev.Deadline.UTC(),
			SecondsLeft:   int64(ev.Left.Seconds()),
			LastActiveDay: u.Streak.LastActiveDay,
			XP:            xp,
			Rank:          rank.Level,
			RankName:      rank.Name,
			Presence:      presenceView(u.Presence, now),
			Week:          make([]int64, weekDays),
			Relation:      string(rels[u.ID]),
		}
		if u.ID == me.ID {
			row.Relation = relMe
		}
		// Same rule as /status: never show a stale streak for a failed user.
		if ev.State == streak.StateFailed {
			row.Streak = 0
		}
		today := streak.DayOf(now, loc)
		for i := range weekDays {
			row.Week[i] = usage[u.ID][streak.AddDays(today, i-weekDays+1)]
		}
		out = append(out, row)
	}
	return out, nil
}

// presenceView turns stored presence into the API shape (nil = never).
func presenceView(p *store.Presence, now time.Time) *api.Presence {
	if p == nil {
		return nil
	}
	online := now.Sub(p.UpdatedAt) < onlineWindow
	pr := &api.Presence{Online: online, Idle: p.Idle, UpdatedAt: p.UpdatedAt.UTC()}
	if online && !p.Idle {
		pr.App, pr.Since = p.App, p.Since.UTC()
	}
	return pr
}
