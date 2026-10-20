package server

import (
	"context"
	"testing"
	"time"

	"github.com/AlexSol/gachi-streak/internal/api"
	"github.com/AlexSol/gachi-streak/internal/store"
)

func TestUserProfile(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	alex, err := f.srv.Store.UserByName(ctx, "alex")
	if err != nil {
		t.Fatal(err)
	}
	sasha, err := f.srv.Store.UserByName(ctx, "sasha")
	if err != nil {
		t.Fatal(err)
	}
	_, olyaToken, err := f.srv.Store.CreateUser(ctx, store.User{Name: "olya", Timezone: "UTC"}, f.now)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := f.srv.Store.MarkFailed(ctx, sasha.ID, "2026-10-16", f.now); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		user store.User
		day  string
		note string
	}{
		{alex, "2026-10-17", "Go maps"},
		{alex, "2026-10-19", "channels"},
		{sasha, "2026-10-18", "goroutines"},
		{sasha, "2026-10-19", "interfaces"},
	} {
		at, _ := time.Parse(time.RFC3339, item.day+"T10:00:00Z")
		if _, _, err := f.srv.Store.RecordActivity(ctx, item.user, store.SourceManual, item.note, at); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.srv.Store.SavePresence(ctx, sasha.ID, store.Presence{UpdatedAt: f.now}, "2026-10-19", map[string]int64{
		"GoLand": 3600, "Chrome": 900,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.Achievements.Unlock(ctx, sasha.ID, "streak_7", "2026-10-18", f.now); err != nil {
		t.Fatal(err)
	}
	befriend(t, f, f.alex, "alex", f.sasha, "sasha")
	befriend(t, f, f.alex, "alex", olyaToken, "olya")
	befriend(t, f, f.sasha, "sasha", olyaToken, "olya")

	if code := f.do(t, "GET", "/users/sasha/profile", "", "", nil); code != 401 {
		t.Fatalf("anonymous profile: %d", code)
	}
	var other api.UserProfile
	if code := f.do(t, "GET", "/users/sasha/profile", f.alex, "", &other); code != 200 {
		t.Fatalf("GET profile: %d", code)
	}
	if other.Name != "sasha" || other.Relation != string(store.RelFriend) || other.Streak != 2 || other.Best != 2 {
		t.Fatalf("profile identity and streak: %+v", other)
	}
	if len(other.Days) != profileHeatmapDays {
		t.Fatalf("heatmap length = %d", len(other.Days))
	}
	dayMarks := map[string]api.ProfileDay{}
	for _, day := range other.Days {
		dayMarks[day.Day] = day
	}
	if !dayMarks["2026-10-19"].Counted || !dayMarks["2026-10-16"].Missed {
		t.Fatalf("heatmap marks: %+v %+v", dayMarks["2026-10-19"], dayMarks["2026-10-16"])
	}
	if other.LearningLog != nil {
		t.Fatalf("another user's learning log must be private: %+v", other.LearningLog)
	}
	if other.Comparison == nil || len(other.Comparison.Days) != profileCompareDays ||
		other.Comparison.Totals.MeCounted != 2 || other.Comparison.Totals.UserCounted != 2 {
		t.Fatalf("30-day comparison: %+v", other.Comparison)
	}
	if len(other.TopApps) != 2 || other.TopApps[0].App != "GoLand" || other.TopApps[0].Seconds != 3600 {
		t.Fatalf("top programs: %+v", other.TopApps)
	}
	if len(other.LatestAchievements) == 0 || other.LatestAchievements[0].ID != "streak_7" {
		t.Fatalf("recent achievements: %+v", other.LatestAchievements)
	}
	if len(other.SharedFriends) != 1 || other.SharedFriends[0] != "olya" {
		t.Fatalf("shared friends: %+v", other.SharedFriends)
	}

	var own api.UserProfile
	if code := f.do(t, "GET", "/users/alex/profile", f.alex, "", &own); code != 200 {
		t.Fatalf("GET own profile: %d", code)
	}
	if own.Relation != "me" || len(own.LearningLog) != 2 || own.Comparison != nil {
		t.Fatalf("own profile: %+v", own)
	}
	if code := f.do(t, "GET", "/users/unknown/profile", f.alex, "", nil); code != 404 {
		t.Fatalf("unknown profile: %d", code)
	}
}
