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

func TestPickLang(t *testing.T) {
	cases := map[string]string{
		"":                                 "en",
		"uk-UA,uk;q=0.9,en-US;q=0.8":       "uk",
		"en-US,en;q=0.9,uk;q=0.8":          "en",
		"pt-PT,pt;q=0.9,uk;q=0.7,en;q=0.5": "uk",
		"de-DE":                            "en",
		"en;q=0.5,uk;q=0.9":                "uk",
		"uk;q=0":                           "en",
	}
	for h, want := range cases {
		if got := PickLang(h); got != want {
			t.Errorf("PickLang(%q) = %q, want %q", h, got, want)
		}
	}
}

func TestIndexPage(t *testing.T) {
	f := setup(t)
	for _, c := range []struct{ accept, lang string }{
		{"uk-UA,uk;q=0.9", "uk"},
		{"en-GB", "en"},
	} {
		req, _ := http.NewRequest("GET", f.http.URL+"/", nil)
		req.Header.Set("Accept-Language", c.accept)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 || !strings.Contains(string(body), `<html lang="`+c.lang+`">`) {
			t.Fatalf("%s: code %d, lang not %s", c.accept, resp.StatusCode, c.lang)
		}
		if !strings.Contains(string(body), "Ресурс ще в розробці") || !strings.Contains(string(body), "still under development") {
			t.Fatal("both language blocks must be present for client-side switch")
		}
	}
}

func TestNotFound(t *testing.T) {
	f := setup(t)
	get := func(path, accept, lang string) (*http.Response, string) {
		req, _ := http.NewRequest("GET", f.http.URL+path, nil)
		if accept != "" {
			req.Header.Set("Accept", accept)
		}
		if lang != "" {
			req.Header.Set("Accept-Language", lang)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return resp, string(b)
	}

	// Browser → themed HTML page, path is HTML-escaped.
	resp, body := get("/<script>x", "text/html,application/xhtml+xml", "uk-UA,uk")
	if resp.StatusCode != 404 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") {
		t.Fatalf("browser 404: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	if !strings.Contains(body, `<html lang="uk">`) || !strings.Contains(body, "заблукав у dungeon") {
		t.Fatal("themed Ukrainian 404 expected")
	}
	if strings.Contains(body, "<script>x") {
		t.Fatal("path must be escaped")
	}

	// API client → JSON.
	resp, body = get("/nope", "application/json", "")
	if resp.StatusCode != 404 || !strings.Contains(body, `"not found"`) {
		t.Fatalf("api 404: %d %s", resp.StatusCode, body)
	}

	// Wrong method on a real route stays 405, not 404.
	resp, _ = get("/ping", "text/html", "")
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("GET /ping: %d", resp.StatusCode)
	}

	// Real routes are untouched.
	resp, body = get("/healthz", "text/html", "")
	if resp.StatusCode != 200 || body != "ok" {
		t.Fatalf("healthz: %d %q", resp.StatusCode, body)
	}
}

func TestPresenceAndActivity(t *testing.T) {
	f := setup(t)
	f.now = time.Date(2026, 10, 19, 15, 0, 0, 0, time.UTC)
	body := `{"app":"GoLand","since":"2026-10-19T14:30:00Z","idle":false,"day":"2026-10-19",
	          "usage":{"GoLand":5400,"Chrome":1200,"Bad\u0007Name":60}}`
	if code := f.do(t, "POST", "/presence", f.alex, body, nil); code != 204 {
		t.Fatalf("presence: %d", code)
	}
	if code := f.do(t, "POST", "/presence", f.alex, `{"day":"2026-10-10","usage":{}}`, nil); code != 400 {
		t.Fatalf("old day must be rejected: %d", code)
	}

	s := f.status(t, f.sasha)
	a := s["alex"]
	if a.Presence == nil || !a.Presence.Online || a.Presence.App != "GoLand" {
		t.Fatalf("presence %+v", a.Presence)
	}
	if len(a.TodayApps) != 3 || a.TodayApps[0].App != "GoLand" || a.TodayApps[0].Seconds != 5400 {
		t.Fatalf("today apps %+v", a.TodayApps)
	}
	for _, x := range a.TodayApps {
		if strings.ContainsRune(x.App, '\a') {
			t.Fatal("control chars must be stripped")
		}
	}
	if s["sasha"].Presence != nil {
		t.Fatal("sasha never reported")
	}

	// 5 minutes later without reports → offline, app hidden
	f.now = f.now.Add(5 * time.Minute)
	if p := f.status(t, f.alex)["alex"].Presence; p == nil || p.Online || p.App != "" {
		t.Fatalf("stale presence %+v", p)
	}

	var act api.ActivityResponse
	if code := f.do(t, "GET", "/activity?days=7", f.alex, "", &act); code != 200 {
		t.Fatalf("activity %d", code)
	}
	for _, u := range act.Users {
		if len(u.ByDay) != 7 {
			t.Fatalf("by_day len %d", len(u.ByDay))
		}
		if u.Name == "alex" && (u.TotalSeconds != 5400+1200+60 || u.Apps[0].App != "GoLand") {
			t.Fatalf("alex activity %+v", u)
		}
	}
}

func TestProgress(t *testing.T) {
	f := setup(t)
	if code := f.do(t, "GET", "/progress", "", "", nil); code != 401 {
		t.Fatalf("anonymous progress: %d", code)
	}
	start := f.now
	for i := 0; i < 7; i++ {
		f.now = start.AddDate(0, 0, i)
		f.do(t, "POST", "/ping", f.alex, `{"source":"manual"}`, nil)
		f.do(t, "POST", "/ping", f.alex, `{"source":"manual"}`, nil) // same day again: no extra XP
		if i < 3 {
			f.do(t, "POST", "/ping", f.sasha, `{"source":"manual"}`, nil)
		}
	}

	var resp api.ProgressResponse
	if code := f.do(t, "GET", "/progress", f.sasha, "", &resp); code != 200 {
		t.Fatalf("progress %d", code)
	}
	if resp.Me != "sasha" || len(resp.Ranks) != 7 || len(resp.Achievements) == 0 {
		t.Fatalf("resp %+v", resp)
	}
	by := map[string]api.UserProgress{}
	for _, u := range resp.Users {
		by[u.Name] = u
	}
	alex := by["alex"]
	// 7 days * 30 + streak_7 (30)
	if alex.ActiveDays != 7 || alex.XP != 240 || alex.Rank != 2 || alex.NextRankXP != 300 {
		t.Fatalf("alex %+v", alex)
	}
	unlocked := map[string]bool{}
	for _, a := range alex.Achievements {
		unlocked[a.ID] = a.Unlocked
		if a.ID == "duo_7" && a.Progress != 3 {
			t.Fatalf("duo_7 progress %+v", a)
		}
	}
	if !unlocked["streak_7"] || unlocked["duo_7"] {
		t.Fatalf("alex achievements %+v", unlocked)
	}
	if s := by["sasha"]; s.ActiveDays != 3 || s.XP != 90 || s.Rank != 1 {
		t.Fatalf("sasha %+v", s)
	}
}

func TestGitHubCodeAchievements(t *testing.T) {
	f := setup(t)
	f.now = time.Date(2026, 10, 19, 15, 0, 0, 0, time.UTC)
	const src = "package x\n\nimport \"fmt\"\n\nfunc a(err error) error {\n\tgo a(nil)\n\treturn fmt.Errorf(\"a: %w\", err)\n}\n"
	var hits = map[string]int{}
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits[r.URL.Path]++
		switch r.URL.Path {
		case "/users/sasha-gh/events":
			if r.Header.Get("If-None-Match") == `"v1"` {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			w.Header().Set("ETag", `"v1"`)
			w.Write([]byte(`[
				{"type":"PushEvent","created_at":"2026-10-19T12:00:00Z","repo":{"name":"sasha-gh/go-diary"},
				 "payload":{"before":"aaa","head":"bbb"}},
				{"type":"PushEvent","created_at":"2026-10-18T12:00:00Z","repo":{"name":"sasha-gh/go-diary"},
				 "payload":{"before":"0000000000","head":"gone"}}
			]`))
		case "/repos/sasha-gh/go-diary/compare/aaa...bbb":
			w.Write([]byte(`{"files":[
				{"filename":"x.go","status":"modified","patch":"@@ -5,2 +5,4 @@\n func a(err error) error {\n+\tgo a(nil)\n+\treturn fmt.Errorf(\"a: %w\", err)\n }"},
				{"filename":"README.md","status":"modified","patch":"@@ -1 +1 @@\n+go func() {}"},
				{"filename":"old.go","status":"removed","patch":"@@ -1 +0,0 @@\n-package x"}
			]}`))
		case "/repos/sasha-gh/go-diary/contents/x.go":
			if r.URL.Query().Get("ref") != "bbb" {
				t.Errorf("contents ref %q", r.URL.RawQuery)
			}
			w.Write([]byte(src))
		default: // commits/gone: force-pushed away
			http.NotFound(w, r)
		}
	}))
	defer gh.Close()

	c := &GitHubChecker{S: f.srv, BaseURL: gh.URL}
	c.CheckAll(context.Background())
	c.CheckAll(context.Background()) // second pass: 304 and nothing rescanned

	if hits["/repos/sasha-gh/go-diary/compare/aaa...bbb"] != 1 || hits["/repos/sasha-gh/go-diary/commits/gone"] != 1 {
		t.Fatalf("each push must be fetched once: %v", hits)
	}
	if hits["/repos/sasha-gh/go-diary/contents/README.md"] != 0 {
		t.Fatalf("non-Go files fetched: %v", hits)
	}

	var resp api.ProgressResponse
	if code := f.do(t, "GET", "/progress", f.sasha, "", &resp); code != 200 {
		t.Fatalf("progress %d", code)
	}
	var sasha api.UserProgress
	for _, u := range resp.Users {
		if u.Name == "sasha" {
			sasha = u
		} else if u.CodeTracking {
			t.Fatalf("%s has no github login", u.Name)
		}
	}
	st := map[string]api.AchievementState{}
	for _, a := range sasha.Achievements {
		st[a.ID] = a
	}
	if g := st["goroutine"]; !g.Unlocked || g.Day != "2026-10-19" || !g.New {
		t.Fatalf("goroutine %+v", g)
	}
	if e := st["errwrap"]; e.Unlocked || e.Progress != 1 {
		t.Fatalf("errwrap %+v", e)
	}
	if !sasha.CodeTracking {
		t.Fatal("sasha code tracking")
	}
}

func TestAchievementsPersistAndSeen(t *testing.T) {
	f := setup(t)
	start := f.now
	for i := range 7 {
		f.now = start.AddDate(0, 0, i)
		f.do(t, "POST", "/ping", f.alex, "", nil)
	}
	f.srv.CheckDeadlines(context.Background()) // the worker stores the unlock

	ctx := context.Background()
	users, _ := f.srv.Store.ListUsers(ctx)
	alexID := users[0].ID
	unlocks, err := f.srv.Achievements.ListByUser(ctx, alexID)
	if err != nil || len(unlocks) != 1 || unlocks[0].AchievementID != "streak_7" || unlocks[0].Day != "2026-10-25" {
		t.Fatalf("unlocks %+v %v", unlocks, err)
	}

	get := func(token string) map[string]api.AchievementState {
		var resp api.ProgressResponse
		f.do(t, "GET", "/progress", token, "", &resp)
		out := map[string]api.AchievementState{}
		for _, u := range resp.Users {
			if u.Name == "alex" {
				for _, a := range u.Achievements {
					out[a.ID] = a
				}
			}
		}
		return out
	}
	if a := get(f.alex)["streak_7"]; !a.Unlocked || !a.New {
		t.Fatalf("own new unlock %+v", a)
	}
	if a := get(f.sasha)["streak_7"]; !a.Unlocked || a.New {
		t.Fatalf("partner must not see alex's New flag: %+v", a)
	}
	if code := f.do(t, "POST", "/achievements/seen", f.alex, "", nil); code != 204 {
		t.Fatalf("seen %d", code)
	}
	if a := get(f.alex)["streak_7"]; !a.Unlocked || a.New {
		t.Fatalf("after seen %+v", a)
	}
}
