package client

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// LocalCommit is a commit found in a local repository.
type LocalCommit struct {
	Repo    string
	Hash    string
	Email   string
	When    time.Time
	Subject string
}

// LocalGit finds today's commits in local repositories using the git CLI.
type LocalGit struct {
	Repos   []string // repository folders
	Authors []string // author emails to count; empty → each repo's own user.email
	GitPath string   // "git" by default
}

// ErrNoGit means the git executable is not available.
var ErrNoGit = errors.New("git not found in PATH")

// FindToday returns the first commit authored "today" in loc (all branches)
// whose author matches. A commit counts if its author date falls on today's
// calendar day in the user's time zone — the same day the server uses.
func (g LocalGit) FindToday(ctx context.Context, now time.Time, loc *time.Location) (*LocalCommit, []error) {
	git := g.GitPath
	if git == "" {
		git = "git"
	}
	if _, err := exec.LookPath(git); err != nil {
		return nil, []error{ErrNoGit}
	}
	today := now.In(loc).Format("2006-01-02")
	// --since filters by committer date; take a generous window and filter
	// precisely by author date below.
	since := now.Add(-48 * time.Hour).UTC().Format(time.RFC3339)

	var errs []error
	for _, repo := range g.Repos {
		repo = strings.TrimSpace(repo)
		if repo == "" {
			continue
		}
		authors := normEmails(g.Authors)
		if len(authors) == 0 {
			// Default: only the person whose repo it is — so pulling your
			// partner's commits does not count as your work.
			if e, err := runGit(ctx, git, repo, "config", "user.email"); err == nil {
				authors = normEmails([]string{e})
			}
		}
		out, err := runGit(ctx, git, repo, "log", "--all", "--no-merges",
			"--since="+since, "--format=%H%x1f%ae%x1f%aI%x1f%s")
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", repo, err))
			continue
		}
		for _, line := range strings.Split(out, "\n") {
			c, ok := parseLogLine(repo, line)
			if !ok || c.When.In(loc).Format("2006-01-02") != today {
				continue
			}
			if len(authors) > 0 && !authors[strings.ToLower(c.Email)] {
				continue
			}
			return &c, errs
		}
	}
	return nil, errs
}

func parseLogLine(repo, line string) (LocalCommit, bool) {
	parts := strings.SplitN(strings.TrimSpace(line), "\x1f", 4)
	if len(parts) < 3 {
		return LocalCommit{}, false
	}
	when, err := time.Parse(time.RFC3339, parts[2])
	if err != nil {
		return LocalCommit{}, false
	}
	c := LocalCommit{Repo: repo, Hash: parts[0], Email: parts[1], When: when}
	if len(parts) == 4 {
		c.Subject = parts[3]
	}
	return c, true
}

func normEmails(in []string) map[string]bool {
	m := map[string]bool{}
	for _, e := range in {
		if e = strings.ToLower(strings.TrimSpace(e)); e != "" {
			m[e] = true
		}
	}
	if len(m) == 0 {
		return nil
	}
	return m
}

func runGit(ctx context.Context, git, repo string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, git, append([]string{"-C", repo}, args...)...)
	hideWindow(cmd) // no console flash on Windows
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", errors.New(msg)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// Note builds the ping note for a found commit.
func (c LocalCommit) Note() string {
	h := c.Hash
	if len(h) > 8 {
		h = h[:8]
	}
	s := fmt.Sprintf("local commit %s in %s", h, filepath.Base(c.Repo))
	if c.Subject != "" {
		s += ": " + c.Subject
	}
	if r := []rune(s); len(r) > 200 {
		s = string(r[:200])
	}
	return s
}
