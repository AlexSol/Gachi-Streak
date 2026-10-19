package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AlexSol/gachi-streak/internal/api"
	"github.com/AlexSol/gachi-streak/internal/store"
)

type fixture struct {
	srv         *Server
	http        *httptest.Server
	now         time.Time
	alex, sasha string // tokens
}

func setup(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	f := &fixture{now: time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)}
	f.srv = New(st, 5*time.Hour, slog.New(slog.NewTextHandler(io.Discard, nil)))
	f.srv.Now = func() time.Time { return f.now }

	_, f.alex, err = st.CreateUser(ctx, store.User{Name: "alex", Timezone: "UTC"}, f.now.AddDate(0, 0, -7))
	if err != nil {
		t.Fatal(err)
	}
	_, f.sasha, err = st.CreateUser(ctx, store.User{Name: "sasha", Timezone: "UTC", GitHubLogin: "sasha-gh", GitHubRepo: "sasha-gh/go-diary"}, f.now.AddDate(0, 0, -7))
	if err != nil {
		t.Fatal(err)
	}
	f.http = httptest.NewServer(f.srv.Handler())
	t.Cleanup(f.http.Close)
	return f
}

func (f *fixture) do(t *testing.T, method, path, token, body string, out any) int {
	t.Helper()
	req, _ := http.NewRequest(method, f.http.URL+path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatal(err)
		}
	}
	return resp.StatusCode
}

func (f *fixture) status(t *testing.T, token string) map[string]api.UserStatus {
	t.Helper()
	var resp api.StatusResponse
	if code := f.do(t, "GET", "/status", token, "", &resp); code != 200 {
		t.Fatalf("status code %d", code)
	}
	m := map[string]api.UserStatus{}
	for _, u := range resp.Users {
		m[u.Name] = u
	}
	return m
}

func TestAuth(t *testing.T) {
	f := setup(t)
	if code := f.do(t, "GET", "/status", "", "", nil); code != 401 {
		t.Fatalf("no token: %d", code)
	}
	if code := f.do(t, "GET", "/status", "wrong", "", nil); code != 401 {
		t.Fatalf("bad token: %d", code)
	}
}

func TestFullDayCycle(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	// Both missed "yesterday" (never pinged, created a week ago) → failed.
	f.srv.CheckDeadlines(ctx)
	s := f.status(t, f.alex)
	if s["alex"].State != "failed" || s["alex"].LastFailure == nil {
		t.Fatalf("alex should be failed: %+v", s["alex"])
	}

	// Alex pings → safe, streak 1.
	var pr api.PingResponse
	if code := f.do(t, "POST", "/ping", f.alex, `{"note":"goroutines"}`, &pr); code != 200 {
		t.Fatalf("ping code %d", code)
	}
	if !pr.Counted || pr.User.Streak != 1 || pr.User.State != "safe" {
		t.Fatalf("ping resp %+v", pr)
	}
	// Unknown / reserved source is rejected.
	if code := f.do(t, "POST", "/ping", f.alex, `{"source":"github"}`, nil); code != 400 {
		t.Fatalf("github source must be rejected from clients: %d", code)
	}
	// Second ping same day: not counted, empty body is fine.
	f.do(t, "POST", "/ping", f.alex, "", &pr)
	if pr.Counted || pr.User.Streak != 1 {
		t.Fatalf("second ping %+v", pr)
	}

	// 20:00 → sasha (still nothing today) stays failed; alex safe.
	f.now = time.Date(2026, 10, 19, 20, 0, 0, 0, time.UTC)
	s = f.status(t, f.sasha)
	if s["alex"].State != "safe" || s["sasha"].State != "failed" {
		t.Fatalf("20:00: %+v", s)
	}

	// Next day 19:30: alex hasn't pinged today → warning (4.5h left).
	f.now = time.Date(2026, 10, 20, 19, 30, 0, 0, time.UTC)
	f.srv.CheckDeadlines(ctx)
	s = f.status(t, f.alex)
	if s["alex"].State != "warning" || s["alex"].Streak != 1 {
		t.Fatalf("warning: %+v", s["alex"])
	}

	// Alex pings → streak 2.
	f.do(t, "POST", "/ping", f.alex, "", &pr)
	if pr.User.Streak != 2 {
		t.Fatalf("streak 2: %+v", pr.User)
	}

	// Alex skips 10-21 entirely; right after its deadline the streak resets.
	f.now = time.Date(2026, 10, 22, 0, 5, 0, 0, time.UTC)
	f.srv.CheckDeadlines(ctx)
	s = f.status(t, f.alex)
	if s["alex"].State != "failed" || s["alex"].Streak != 0 || s["alex"].Best != 2 {
		t.Fatalf("reset: %+v", s["alex"])
	}
	if s["alex"].LastFailure.Day != "2026-10-21" {
		t.Fatalf("failure day %+v", s["alex"].LastFailure)
	}
}

func TestGitHubChecker(t *testing.T) {
	f := setup(t)
	f.now = time.Date(2026, 10, 19, 15, 0, 0, 0, time.UTC)
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users/sasha-gh/events" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(`[
			{"type":"PushEvent","created_at":"2026-10-19T12:00:00Z","repo":{"name":"sasha-gh/other"}},
			{"type":"WatchEvent","created_at":"2026-10-19T13:00:00Z","repo":{"name":"sasha-gh/go-diary"}},
			{"type":"PushEvent","created_at":"2026-10-19T11:00:00Z","repo":{"name":"sasha-gh/go-diary"}}
		]`))
	}))
	defer gh.Close()

	c := &GitHubChecker{S: f.srv, BaseURL: gh.URL}
	c.CheckAll(context.Background())
	s := f.status(t, f.alex)
	if !s["sasha"].DoneToday || s["sasha"].Streak != 1 {
		t.Fatalf("sasha should be counted by github push: %+v", s["sasha"])
	}
	if s["alex"].DoneToday {
		t.Fatal("alex has no github login")
	}
}
