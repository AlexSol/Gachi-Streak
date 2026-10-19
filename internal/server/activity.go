package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/AlexSol/gachi-streak/internal/api"
	"github.com/AlexSol/gachi-streak/internal/store"
	"github.com/AlexSol/gachi-streak/internal/streak"
)

const (
	onlineWindow = 3 * time.Minute // presence older than this = offline
	maxApps      = 60
	maxAppName   = 64
)

// cleanApp keeps program names printable and short.
func cleanApp(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(s))
	if r := []rune(s); len(r) > maxAppName {
		s = string(r[:maxAppName])
	}
	return s
}

// handlePresence receives the desktop client's current program and the
// day's per-program totals.
func (s *Server) handlePresence(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	var req api.PresenceRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad json")
		return
	}
	now := s.Now()
	today := streak.DayOf(now, u.Location())
	if req.Day != today && req.Day != streak.AddDays(today, -1) {
		writeErr(w, http.StatusBadRequest, "day must be today or yesterday in your time zone")
		return
	}
	if len(req.Usage) > maxApps {
		writeErr(w, http.StatusBadRequest, "too many apps")
		return
	}
	usage := make(map[string]int64, len(req.Usage))
	for app, sec := range req.Usage {
		app = cleanApp(app)
		if app == "" || sec <= 0 {
			continue
		}
		if sec > 25*3600 {
			sec = 25 * 3600 // a (DST) day can't hold more
		}
		usage[app] += sec
	}
	p := store.Presence{App: cleanApp(req.App), Idle: req.Idle, UpdatedAt: now}
	if p.App != "" && !req.Since.IsZero() && !req.Since.After(now) {
		p.Since = req.Since
	}
	if err := s.Store.SavePresence(r.Context(), u.ID, p, req.Day, usage); err != nil {
		s.Log.Error("presence", "user", u.Name, "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// presenceFor fills the Discord-like fields of a user's status.
func (s *Server) presenceFor(ctx context.Context, u store.User, now time.Time, us *api.UserStatus) error {
	p, err := s.Store.GetPresence(ctx, u.ID)
	switch {
	case errors.Is(err, store.ErrNotFound):
		// never reported — no presence
	case err != nil:
		return err
	default:
		online := now.Sub(p.UpdatedAt) < onlineWindow
		pr := &api.Presence{Online: online, Idle: p.Idle, UpdatedAt: p.UpdatedAt.UTC()}
		if online && !p.Idle {
			pr.App, pr.Since = p.App, p.Since.UTC()
		}
		us.Presence = pr
	}

	today := streak.DayOf(now, u.Location())
	rows, err := s.Store.UsageRange(ctx, u.ID, today, today)
	if err != nil {
		return err
	}
	for _, row := range rows {
		us.TodayApps = append(us.TodayApps, api.AppTime{App: row.App, Seconds: row.Seconds})
	}
	if len(us.TodayApps) > 8 {
		us.TodayApps = us.TodayApps[:8]
	}
	return nil
}

// handleActivity returns per-program statistics for the last N days.
func (s *Server) handleActivity(w http.ResponseWriter, r *http.Request) {
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days < 1 {
		days = 7
	}
	if days > 31 {
		days = 31
	}
	users, err := s.Store.ListUsers(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	now := s.Now()
	resp := api.ActivityResponse{Days: days}
	for _, u := range users {
		to := streak.DayOf(now, u.Location())
		from := streak.AddDays(to, -(days - 1))
		rows, err := s.Store.UsageRange(r.Context(), u.ID, from, to)
		if err != nil {
			s.Log.Error("activity", "user", u.Name, "err", err)
			writeErr(w, http.StatusInternalServerError, "internal error")
			return
		}
		perApp := map[string]int64{}
		perDay := map[string]int64{}
		var total int64
		for _, row := range rows {
			perApp[row.App] += row.Seconds
			perDay[row.Day] += row.Seconds
			total += row.Seconds
		}
		au := api.ActivityUser{Name: u.Name, TotalSeconds: total}
		for app, sec := range perApp {
			au.Apps = append(au.Apps, api.AppTime{App: app, Seconds: sec})
		}
		sort.Slice(au.Apps, func(i, j int) bool {
			if au.Apps[i].Seconds != au.Apps[j].Seconds {
				return au.Apps[i].Seconds > au.Apps[j].Seconds
			}
			return au.Apps[i].App < au.Apps[j].App
		})
		if len(au.Apps) > 12 {
			au.Apps = au.Apps[:12]
		}
		for d := from; d <= to; d = streak.AddDays(d, 1) {
			au.ByDay = append(au.ByDay, api.DayTime{Day: d, Seconds: perDay[d]})
		}
		resp.Users = append(resp.Users, au)
	}
	writeJSON(w, http.StatusOK, resp)
}
