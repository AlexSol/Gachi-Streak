package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/AlexSol/gachi-streak/internal/store"
	"github.com/AlexSol/gachi-streak/internal/streak"
)

// GitHubChecker auto-records activity when a user pushed commits today.
// It uses the public events API: GET /users/{login}/events.
type GitHubChecker struct {
	S       *Server
	Token   string // optional; raises the rate limit from 60 to 5000 req/h
	BaseURL string // https://api.github.com, overridable in tests
	HTTP    *http.Client
}

type ghEvent struct {
	Type      string    `json:"type"`
	CreatedAt time.Time `json:"created_at"`
	Repo      struct {
		Name string `json:"name"`
	} `json:"repo"`
}

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
	st, err := g.S.Store.GetStreak(ctx, u.ID)
	if err != nil {
		return err
	}
	loc := u.Location()
	today := streak.DayOf(now, loc)
	if st.LastActiveDay == today {
		return nil // already counted, save the rate limit
	}
	events, err := g.fetch(ctx, u.GitHubLogin)
	if err != nil {
		return err
	}
	for _, e := range events {
		if e.Type != "PushEvent" || streak.DayOf(e.CreatedAt, loc) != today {
			continue
		}
		if u.GitHubRepo != "" && !strings.EqualFold(e.Repo.Name, u.GitHubRepo) {
			continue
		}
		note := "push to " + e.Repo.Name
		if _, counted, err := g.S.Store.RecordActivity(ctx, u, store.SourceGitHub, note, now); err != nil {
			return err
		} else if counted {
			g.S.Log.Info("github activity counted", "user", u.Name, "repo", e.Repo.Name)
		}
		return nil
	}
	return nil
}

func (g *GitHubChecker) fetch(ctx context.Context, login string) ([]ghEvent, error) {
	base := g.BaseURL
	if base == "" {
		base = "https://api.github.com"
	}
	client := g.HTTP
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/users/%s/events?per_page=100", base, login), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "gachi-streak")
	if g.Token != "" {
		req.Header.Set("Authorization", "Bearer "+g.Token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github: %s", resp.Status)
	}
	var events []ghEvent
	if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
		return nil, err
	}
	return events, nil
}
