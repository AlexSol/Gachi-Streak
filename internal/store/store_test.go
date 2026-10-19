package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestUserTokenAndStreakFlow(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	day1 := time.Date(2026, 10, 19, 10, 0, 0, 0, time.UTC)

	u, token, err := s.CreateUser(ctx, User{Name: "alex", Timezone: "Europe/Lisbon"}, day1)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.CreateUser(ctx, User{Name: "bad", Timezone: "Mars/Olympus"}, day1); err == nil {
		t.Fatal("expected bad timezone error")
	}

	got, err := s.UserByToken(ctx, token)
	if err != nil || got.ID != u.ID || got.CreatedDay != "2026-10-19" {
		t.Fatalf("UserByToken: %+v %v", got, err)
	}
	if _, err := s.UserByToken(ctx, "nope"); err != ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
	}

	st, counted, err := s.RecordActivity(ctx, u, SourceManual, "hello", day1)
	if err != nil || !counted || st.Current != 1 {
		t.Fatalf("first ping: %+v %v %v", st, counted, err)
	}
	st, counted, _ = s.RecordActivity(ctx, u, SourceManual, "again", day1.Add(time.Hour))
	if counted || st.Current != 1 {
		t.Fatalf("second ping same day: %+v %v", st, counted)
	}
	st, counted, _ = s.RecordActivity(ctx, u, SourceGitHub, "", day1.AddDate(0, 0, 1))
	if !counted || st.Current != 2 || st.Best != 2 {
		t.Fatalf("next day: %+v", st)
	}

	ins, err := s.MarkFailed(ctx, u.ID, "2026-10-21", day1.AddDate(0, 0, 3))
	if err != nil || !ins {
		t.Fatalf("MarkFailed: %v %v", ins, err)
	}
	ins, _ = s.MarkFailed(ctx, u.ID, "2026-10-21", day1.AddDate(0, 0, 3))
	if ins {
		t.Fatal("MarkFailed must be idempotent per day")
	}
	st, _ = s.GetStreak(ctx, u.ID)
	if st.Current != 0 || st.Best != 2 {
		t.Fatalf("after fail: %+v", st)
	}
	ev, err := s.LastEvent(ctx, u.ID, EventFailed)
	if err != nil || ev.Day != "2026-10-21" {
		t.Fatalf("LastEvent: %+v %v", ev, err)
	}

	newTok, err := s.RotateToken(ctx, "alex")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UserByToken(ctx, token); err != ErrNotFound {
		t.Fatal("old token must stop working")
	}
	if _, err := s.UserByToken(ctx, newTok); err != nil {
		t.Fatal(err)
	}
}
