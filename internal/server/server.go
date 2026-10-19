// Package server exposes the HTTP API and runs the background workers.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/AlexSol/gachi-streak/internal/api"
	"github.com/AlexSol/gachi-streak/internal/store"
	"github.com/AlexSol/gachi-streak/internal/streak"
)

// Server holds dependencies of the HTTP handlers and workers.
type Server struct {
	Store *store.Store
	Warn  time.Duration // warning window before the deadline
	Log   *slog.Logger
	Now   func() time.Time

	// UI serves the single-page app under /app/ (nil = not mounted).
	UI http.Handler

	limOnce sync.Once
	lim     *loginLimiter
}

// New creates a server with sane defaults.
func New(st *store.Store, warn time.Duration, log *slog.Logger) *Server {
	return &Server{Store: st, Warn: warn, Log: log, Now: time.Now}
}

type ctxKey struct{}

// Handler returns the HTTP routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleIndex) // landing page, exact "/" only
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("ok"))
	})
	mux.Handle("POST /ping", s.auth(http.HandlerFunc(s.handlePing)))
	mux.Handle("GET /status", s.auth(http.HandlerFunc(s.handleStatus)))
	mux.Handle("POST /presence", s.auth(http.HandlerFunc(s.handlePresence)))
	mux.Handle("GET /activity", s.auth(http.HandlerFunc(s.handleActivity)))

	// Browser sessions: an invite token is exchanged for an HttpOnly cookie.
	mux.HandleFunc("POST /auth/login", s.handleLogin)
	mux.HandleFunc("POST /auth/logout", s.handleLogout)
	mux.Handle("GET /auth/me", s.auth(http.HandlerFunc(s.handleMe)))

	if s.UI != nil {
		mux.Handle("GET /app/", s.UI)
		mux.Handle("GET /app", http.RedirectHandler("/app/", http.StatusMovedPermanently))
	}
	return s.logRequests(s.withNotFound(mux))
}

func userFrom(r *http.Request) store.User {
	u, _ := r.Context().Value(ctxKey{}).(store.User)
	return u
}

func (s *Server) handlePing(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	var req api.PingRequest
	body, _ := io.ReadAll(io.LimitReader(r.Body, 4096))
	if len(strings.TrimSpace(string(body))) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			writeErr(w, http.StatusBadRequest, "bad json: "+err.Error())
			return
		}
	}
	source := req.Source
	switch source {
	case "":
		source = store.SourceManual
	case store.SourceManual, store.SourceLocalGit:
	default: // "github" is reserved for the server-side checker
		writeErr(w, http.StatusBadRequest, "unknown source: "+source)
		return
	}
	now := s.Now()
	_, counted, err := s.Store.RecordActivity(r.Context(), u, source, req.Note, now)
	if err != nil {
		s.Log.Error("ping", "user", u.Name, "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	s.Log.Info("ping", "user", u.Name, "source", source, "counted", counted, "note", req.Note)
	us, err := s.userStatus(r.Context(), u, now)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, api.PingResponse{Counted: counted, User: us})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	now := s.Now()
	users, err := s.Store.ListUsers(r.Context())
	if err != nil {
		s.Log.Error("status", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	resp := api.StatusResponse{Me: me.Name, ServerTime: now.UTC(), WarnHours: s.Warn.Hours()}
	for _, u := range users {
		us, err := s.userStatus(r.Context(), u, now)
		if err != nil {
			s.Log.Error("status", "user", u.Name, "err", err)
			writeErr(w, http.StatusInternalServerError, "internal error")
			return
		}
		resp.Users = append(resp.Users, us)
	}
	writeJSON(w, http.StatusOK, resp)
}

// userStatus builds the API view of one user.
func (s *Server) userStatus(ctx context.Context, u store.User, now time.Time) (api.UserStatus, error) {
	st, err := s.Store.GetStreak(ctx, u.ID)
	if err != nil {
		return api.UserStatus{}, err
	}
	ev := streak.Evaluate(now, u.Location(), st.LastActiveDay, u.CreatedDay, s.Warn)
	us := api.UserStatus{
		Name:          u.Name,
		Timezone:      u.Timezone,
		Streak:        st.Current,
		Best:          st.Best,
		LastActiveDay: st.LastActiveDay,
		DoneToday:     ev.DoneToday,
		State:         string(ev.State),
		Deadline:      ev.Deadline.UTC(),
		SecondsLeft:   int64(ev.Left.Seconds()),
	}
	// The worker may not have reset the streak yet (it ticks once a minute);
	// never show a stale non-zero streak for a failed user.
	if ev.State == streak.StateFailed {
		us.Streak = 0
	}
	if fe, err := s.Store.LastEvent(ctx, u.ID, store.EventFailed); err == nil {
		us.LastFailure = &api.Failure{ID: fe.ID, Day: fe.Day, At: fe.CreatedAt}
	} else if !errors.Is(err, store.ErrNotFound) {
		return api.UserStatus{}, err
	}
	if err := s.presenceFor(ctx, u, now, &us); err != nil {
		return api.UserStatus{}, err
	}
	return us, nil
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		s.Log.Debug("http", "method", r.Method, "path", r.URL.Path, "dur", time.Since(start))
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, api.Error{Error: msg})
}
