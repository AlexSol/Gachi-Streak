package store

import (
	"context"
	"testing"
	"time"
)

func TestAchievementRepo(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	now := time.Date(2026, 10, 19, 10, 0, 0, 0, time.UTC)
	u, _, err := s.CreateUser(ctx, User{Name: "alex", Timezone: "UTC"}, now)
	if err != nil {
		t.Fatal(err)
	}
	repo := s.Achievements()

	if ok, err := repo.Unlock(ctx, u.ID, "streak_7", "2026-10-18", now); err != nil || !ok {
		t.Fatalf("first unlock: %v %v", ok, err)
	}
	if ok, err := repo.Unlock(ctx, u.ID, "streak_7", "2026-10-19", now); err != nil || ok {
		t.Fatalf("second unlock must be a no-op: %v %v", ok, err)
	}
	if _, err := repo.Unlock(ctx, u.ID, "first_fail", "2026-10-10", now); err != nil {
		t.Fatal(err)
	}

	list, err := repo.ListByUser(ctx, u.ID)
	if err != nil || len(list) != 2 || list[0].AchievementID != "first_fail" || list[1].Day != "2026-10-18" {
		t.Fatalf("list %+v %v", list, err)
	}
	if !list[0].SeenAt.IsZero() || !list[1].UnlockedAt.Equal(now) {
		t.Fatalf("fresh unlocks %+v", list)
	}

	if n, err := repo.MarkSeen(ctx, u.ID, now.Add(time.Minute)); err != nil || n != 2 {
		t.Fatalf("mark seen %d %v", n, err)
	}
	if n, _ := repo.MarkSeen(ctx, u.ID, now.Add(time.Hour)); n != 0 {
		t.Fatalf("already seen, got %d", n)
	}
	list, _ = repo.ListByUser(ctx, u.ID)
	if !list[0].SeenAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("seen_at %+v", list[0])
	}
}

func TestCodeRepo(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	now := time.Date(2026, 10, 19, 10, 0, 0, 0, time.UTC)
	u, _, err := s.CreateUser(ctx, User{Name: "alex", Timezone: "UTC"}, now)
	if err != nil {
		t.Fatal(err)
	}
	repo := s.Code()

	if ok, _ := repo.Scanned(ctx, u.ID, "a/b@1"); ok {
		t.Fatal("nothing scanned yet")
	}
	if ok, err := repo.SaveScan(ctx, u.ID, "a/b@1", "2026-10-18", map[string]int{"errwrap": 2, "goroutine": 1}, now); err != nil || !ok {
		t.Fatalf("save: %v %v", ok, err)
	}
	if ok, _ := repo.SaveScan(ctx, u.ID, "a/b@1", "2026-10-18", map[string]int{"errwrap": 9}, now); ok {
		t.Fatal("same ref saved twice")
	}
	// An empty scan is remembered too, so it is not fetched again.
	if _, err := repo.SaveScan(ctx, u.ID, "a/b@2", "2026-10-19", nil, now); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SaveScan(ctx, u.ID, "a/b@3", "2026-10-18", map[string]int{"errwrap": 3}, now); err != nil {
		t.Fatal(err)
	}
	if ok, _ := repo.Scanned(ctx, u.ID, "a/b@2"); !ok {
		t.Fatal("empty scan not remembered")
	}

	got, err := repo.Findings(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []CodeFinding{{"errwrap", "2026-10-18", 5}, {"goroutine", "2026-10-18", 1}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("findings %+v, want %+v", got, want)
	}
}
