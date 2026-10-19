package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/AlexSol/gachi-streak/internal/store"
)

const (
	sessionCookie = "gs_session"
	sessionTTL    = 30 * 24 * time.Hour
)

// LoginRequest is the body of POST /auth/login.
type LoginRequest struct {
	Token string `json:"token"`
}

// MeResponse is returned by GET /auth/me and POST /auth/login.
type MeResponse struct {
	Name     string `json:"name"`
	Timezone string `json:"timezone"`
}

// auth accepts either a bearer token (desktop client, curl) or a browser
// session cookie. Cookie-authenticated unsafe requests must come from our
// own origin (CSRF protection on top of SameSite=Lax).
func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var (
			u   store.User
			err error
		)
		if token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok && token != "" {
			u, err = s.Store.UserByToken(r.Context(), token)
		} else if c, cerr := r.Cookie(sessionCookie); cerr == nil && c.Value != "" {
			if !safeMethod(r.Method) && !sameOrigin(r) {
				writeErr(w, http.StatusForbidden, "cross-origin request rejected")
				return
			}
			u, err = s.Store.UserBySession(r.Context(), c.Value, s.Now())
		} else {
			writeErr(w, http.StatusUnauthorized, "not logged in")
			return
		}
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusUnauthorized, "invalid token or session")
			return
		}
		if err != nil {
			s.Log.Error("auth", "err", err)
			writeErr(w, http.StatusInternalServerError, "internal error")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, u)))
	})
}

// handleLogin exchanges an invite token for an HttpOnly session cookie.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		writeErr(w, http.StatusForbidden, "cross-origin request rejected")
		return
	}
	ip := clientIP(r)
	if !s.limiter().allow(ip, s.Now()) {
		writeErr(w, http.StatusTooManyRequests, "too many attempts, try again later")
		return
	}
	var req LoginRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad json")
		return
	}
	token := strings.TrimSpace(req.Token)
	u, err := s.Store.UserByToken(r.Context(), token)
	if errors.Is(err, store.ErrNotFound) || token == "" {
		s.limiter().fail(ip, s.Now())
		s.Log.Warn("login failed", "ip", ip)
		writeErr(w, http.StatusUnauthorized, "invalid token")
		return
	}
	if err != nil {
		s.Log.Error("login", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	sid, err := s.Store.CreateSession(r.Context(), u.ID, sessionTTL, r.UserAgent(), s.Now())
	if err != nil {
		s.Log.Error("login: session", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    sid,
		Path:     "/",
		MaxAge:   int(sessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
	})
	s.Log.Info("login", "user", u.Name, "ip", ip)
	writeJSON(w, http.StatusOK, MeResponse{Name: u.Name, Timezone: u.Timezone})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		writeErr(w, http.StatusForbidden, "cross-origin request rejected")
		return
	}
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		if err := s.Store.DeleteSession(r.Context(), c.Value); err != nil {
			s.Log.Error("logout", "err", err)
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: isHTTPS(r), SameSite: http.SameSiteLaxMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	writeJSON(w, http.StatusOK, MeResponse{Name: u.Name, Timezone: u.Timezone})
}

func safeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

// sameOrigin checks Origin (or Referer) against the request host.
// Requests with neither header (non-browser clients) are allowed:
// browsers always send Origin on cross-site POSTs.
func sameOrigin(r *http.Request) bool {
	src := r.Header.Get("Origin")
	if src == "" {
		src = r.Header.Get("Referer")
	}
	if src == "" {
		return true
	}
	u, err := url.Parse(src)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}

func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// clientIP prefers Fly's header, then X-Forwarded-For, then the socket.
func clientIP(r *http.Request) string {
	if ip := r.Header.Get("Fly-Client-IP"); ip != "" {
		return ip
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		first, _, _ := strings.Cut(xff, ",")
		return strings.TrimSpace(first)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// loginLimiter allows maxFails failed logins per IP within window.
type loginLimiter struct {
	mu       sync.Mutex
	fails    map[string][]time.Time
	maxFails int
	window   time.Duration
}

func (s *Server) limiter() *loginLimiter {
	s.limOnce.Do(func() {
		s.lim = &loginLimiter{fails: map[string][]time.Time{}, maxFails: 10, window: 15 * time.Minute}
	})
	return s.lim
}

func (l *loginLimiter) prune(ip string, now time.Time) []time.Time {
	keep := l.fails[ip][:0]
	for _, t := range l.fails[ip] {
		if now.Sub(t) < l.window {
			keep = append(keep, t)
		}
	}
	if len(keep) == 0 {
		delete(l.fails, ip)
		return nil
	}
	l.fails[ip] = keep
	return keep
}

func (l *loginLimiter) allow(ip string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.prune(ip, now)) < l.maxFails
}

func (l *loginLimiter) fail(ip string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.fails[ip] = append(l.prune(ip, now), now)
}
