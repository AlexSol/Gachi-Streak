package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/AlexSol/gachi-streak/internal/codescan"
	"github.com/AlexSol/gachi-streak/internal/store"
	"github.com/AlexSol/gachi-streak/internal/streak"
)

// Limits that keep one pass well inside the anonymous rate limit (60 req/h).
const (
	maxScansPerPass = 5  // pushes analyzed per user per pass, the rest waits
	maxFilesPerPush = 30 // Go files fetched per push
)

// GitHubChecker auto-records activity when a user pushed commits today
// and analyzes the Go code of every push for code achievements.
// It uses the public events API: GET /users/{login}/events.
type GitHubChecker struct {
	S       *Server
	Token   string // optional; raises the rate limit from 60 to 5000 req/h
	BaseURL string // https://api.github.com, overridable in tests
	HTTP    *http.Client

	mu    sync.Mutex
	cache map[string]eventsCache // by login: 304 responses cost no rate limit
}

type eventsCache struct {
	etag   string
	events []ghEvent
}

type ghEvent struct {
	Type      string    `json:"type"`
	CreatedAt time.Time `json:"created_at"`
	Repo      struct {
		Name string `json:"name"`
	} `json:"repo"`
	Payload struct {
		Head   string `json:"head"`
		Before string `json:"before"`
	} `json:"payload"`
}

type ghFile struct {
	Filename string `json:"filename"`
	Status   string `json:"status"`
	Patch    string `json:"patch"`
}

// errNotFound is a 404 from GitHub (force-pushed or deleted refs, files).
var errNotFound = errors.New("github: not found")

// Run polls every interval until ctx is done.
func (g *GitHubChecker) Run(ctx context.Context, interval time.Duration) {
	g.CheckAll(ctx)
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			g.CheckAll(ctx)
		}
	}
}

// CheckAll runs one pass over users that have a GitHub login.
func (g *GitHubChecker) CheckAll(ctx context.Context) {
	users, err := g.S.Store.ListUsers(ctx)
	if err != nil {
		g.S.Log.Error("github: list users", "err", err)
		return
	}
	now := g.S.Now()
	for _, u := range users {
		if u.GitHubLogin == "" {
			continue
		}
		if err := g.checkUser(ctx, u, now); err != nil {
			g.S.Log.Warn("github check", "user", u.Name, "err", err)
		}
	}
}

func (g *GitHubChecker) checkUser(ctx context.Context, u store.User, now time.Time) error {
	events, err := g.fetchEvents(ctx, u.GitHubLogin)
	if err != nil {
		return err
	}
	var pushes []ghEvent
	for _, e := range events {
		if e.Type != "PushEvent" {
			continue
		}
		if u.GitHubRepo != "" && !strings.EqualFold(e.Repo.Name, u.GitHubRepo) {
			continue
		}
		pushes = append(pushes, e)
	}
	if err := g.countToday(ctx, u, pushes, now); err != nil {
		return err
	}
	return g.scanPushes(ctx, u, pushes, now)
}

// countToday records today's activity on the first push made today.
func (g *GitHubChecker) countToday(ctx context.Context, u store.User, pushes []ghEvent, now time.Time) error {
	st, err := g.S.Store.GetStreak(ctx, u.ID)
	if err != nil {
		return err
	}
	loc := u.Location()
	today := streak.DayOf(now, loc)
	if st.LastActiveDay == today {
		return nil
	}
	for _, e := range pushes {
		if streak.DayOf(e.CreatedAt, loc) != today {
			continue
		}
		_, counted, err := g.S.Store.RecordActivity(ctx, u, store.SourceGitHub, "push to "+e.Repo.Name, now)
		if err != nil {
			return err
		}
		if counted {
			g.S.Log.Info("github activity counted", "user", u.Name, "repo", e.Repo.Name)
		}
		return nil
	}
	return nil
}

// scanPushes analyzes the Go code of pushes not seen before, oldest first.
// A push that fails (rate limit, network) is retried on the next pass.
func (g *GitHubChecker) scanPushes(ctx context.Context, u store.User, pushes []ghEvent, now time.Time) error {
	budget := maxScansPerPass
	for i := len(pushes) - 1; i >= 0 && budget > 0; i-- { // events are newest first
		e := pushes[i]
		if e.Payload.Head == "" {
			continue
		}
		ref := e.Repo.Name + "@" + e.Payload.Head
		done, err := g.S.Code.Scanned(ctx, u.ID, ref)
		if err != nil {
			return err
		}
		if done {
			continue
		}
		budget--
		found, err := g.analyzePush(ctx, e.Repo.Name, e.Payload.Before, e.Payload.Head)
		if err != nil {
			return fmt.Errorf("scan %s: %w", ref, err)
		}
		day := streak.DayOf(e.CreatedAt, u.Location())
		if _, err := g.S.Code.SaveScan(ctx, u.ID, ref, day, found, now); err != nil {
			return err
		}
		if len(found) > 0 {
			g.S.Log.Info("github code scanned", "user", u.Name, "ref", ref, "found", found)
		}
	}
	return nil
}

// analyzePush runs codescan over the Go files a push changed.
func (g *GitHubChecker) analyzePush(ctx context.Context, repo, before, head string) (map[string]int, error) {
	files, err := g.pushFiles(ctx, repo, before, head)
	if errors.Is(err, errNotFound) {
		return nil, nil // the commits are gone: nothing to analyze
	}
	if err != nil {
		return nil, err
	}
	var scan []codescan.File
	for _, f := range files {
		if len(scan) == maxFilesPerPush {
			break
		}
		if f.Status == "removed" || f.Patch == "" || !strings.HasSuffix(f.Filename, ".go") {
			continue
		}
		added := codescan.AddedLines(f.Patch)
		if len(added) == 0 {
			continue
		}
		src, err := g.fileAt(ctx, repo, f.Filename, head)
		if errors.Is(err, errNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		scan = append(scan, codescan.File{Path: f.Filename, Src: src, Added: added})
	}
	return codescan.Analyze(scan), nil
}

// pushFiles returns the files changed between before and head (the whole
// push), or by head alone for a new branch or a force push.
func (g *GitHubChecker) pushFiles(ctx context.Context, repo, before, head string) ([]ghFile, error) {
	var body struct {
		Files []ghFile `json:"files"`
	}
	if before != "" && strings.Trim(before, "0") != "" {
		err := g.getJSON(ctx, fmt.Sprintf("/repos/%s/compare/%s...%s", repo, before, head), &body)
		if err == nil {
			return body.Files, nil
		}
		if !errors.Is(err, errNotFound) {
			return nil, err
		}
	}
	err := g.getJSON(ctx, fmt.Sprintf("/repos/%s/commits/%s", repo, head), &body)
	return body.Files, err
}

func (g *GitHubChecker) fileAt(ctx context.Context, repo, path, ref string) ([]byte, error) {
	segs := strings.Split(path, "/")
	for i := range segs {
		segs[i] = url.PathEscape(segs[i])
	}
	resp, err := g.get(ctx, fmt.Sprintf("/repos/%s/contents/%s?ref=%s", repo, strings.Join(segs, "/"), url.QueryEscape(ref)),
		"application/vnd.github.raw+json", "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
}

// fetchEvents returns the user's recent public events, reusing the cached
// copy when GitHub answers 304 Not Modified.
func (g *GitHubChecker) fetchEvents(ctx context.Context, login string) ([]ghEvent, error) {
	g.mu.Lock()
	cached := g.cache[login]
	g.mu.Unlock()

	resp, err := g.get(ctx, fmt.Sprintf("/users/%s/events?per_page=100", login), "application/vnd.github+json", cached.etag)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified {
		return cached.events, nil
	}
	var events []ghEvent
	if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
		return nil, err
	}
	if etag := resp.Header.Get("ETag"); etag != "" {
		g.mu.Lock()
		if g.cache == nil {
			g.cache = map[string]eventsCache{}
		}
		g.cache[login] = eventsCache{etag: etag, events: events}
		g.mu.Unlock()
	}
	return events, nil
}

func (g *GitHubChecker) getJSON(ctx context.Context, path string, out any) error {
	resp, err := g.get(ctx, path, "application/vnd.github+json", "")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(out)
}

// get performs a GitHub API request. Any status but 200 (and 304 when
// etag is set) is an error; the caller closes the body.
func (g *GitHubChecker) get(ctx context.Context, path, accept, etag string) (*http.Response, error) {
	base := g.BaseURL
	if base == "" {
		base = "https://api.github.com"
	}
	client := g.HTTP
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "gachi-streak")
	if g.Token != "" {
		req.Header.Set("Authorization", "Bearer "+g.Token)
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusOK || (etag != "" && resp.StatusCode == http.StatusNotModified) {
		return resp, nil
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, errNotFound
	}
	return nil, fmt.Errorf("github %s: %s", path, resp.Status)
}
