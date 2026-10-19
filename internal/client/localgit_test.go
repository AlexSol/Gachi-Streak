package client

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func gitRepo(t *testing.T, email string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", email)
	run("config", "user.name", "Test")
	return dir
}

func commitAt(t *testing.T, dir, email string, when time.Time, msg string) {
	t.Helper()
	f := filepath.Join(dir, "f.txt")
	old, _ := os.ReadFile(f)
	os.WriteFile(f, append(old, []byte(msg+"\n")...), 0o644)
	date := when.Format(time.RFC3339)
	for _, args := range [][]string{
		{"add", "."},
		{"-c", "user.email=" + email, "commit", "-q", "-m", msg},
	} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE="+date, "GIT_COMMITTER_DATE="+date)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

func TestLocalGitFindToday(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Lisbon")
	now := time.Now().In(loc)
	yesterday := now.AddDate(0, 0, -1)

	repo := gitRepo(t, "alex@example.com")
	commitAt(t, repo, "alex@example.com", yesterday, "old work")
	commitAt(t, repo, "buddy@example.com", now.Add(-time.Minute), "partner's commit pulled in")

	g := LocalGit{Repos: []string{repo}}
	if c, errs := g.FindToday(context.Background(), now, loc); c != nil || len(errs) > 0 {
		t.Fatalf("yesterday's own + today's foreign commit must not count: %+v %v", c, errs)
	}

	commitAt(t, repo, "alex@example.com", now.Add(-time.Minute), "learned channels")
	c, errs := g.FindToday(context.Background(), now, loc)
	if c == nil || len(errs) > 0 {
		t.Fatalf("today's own commit must count: %v", errs)
	}
	if c.Email != "alex@example.com" || !strings.Contains(c.Note(), "learned channels") {
		t.Fatalf("commit %+v note %q", c, c.Note())
	}

	// Explicit author list overrides repo's user.email.
	g.Authors = []string{"BUDDY@example.com"}
	if c, _ := g.FindToday(context.Background(), now, loc); c == nil || c.Email != "buddy@example.com" {
		t.Fatalf("explicit author filter: %+v", c)
	}
}

func TestLocalGitBadRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	g := LocalGit{Repos: []string{filepath.Join(t.TempDir(), "nope")}}
	c, errs := g.FindToday(context.Background(), time.Now(), time.UTC)
	if c != nil || len(errs) != 1 {
		t.Fatalf("missing repo must yield one error: %+v %v", c, errs)
	}
}
