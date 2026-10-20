package server

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/AlexSol/gachi-streak/internal/api"
	"github.com/AlexSol/gachi-streak/internal/progress"
	"github.com/AlexSol/gachi-streak/internal/store"
)

func (f *fixture) users(t *testing.T, token, query string) api.UsersResponse {
	t.Helper()
	var resp api.UsersResponse
	if code := f.do(t, "GET", "/users"+query, token, "", &resp); code != 200 {
		t.Fatalf("GET /users%s: %d", query, code)
	}
	return resp
}

func names(rows []api.LeaderRow) string {
	out := ""
	for i, r := range rows {
		if i > 0 {
			out += ","
		}
		out += r.Name
	}
	return out
}

func TestUsersLeaderboard(t *testing.T) {
	f := setup(t)
	f.now = time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)

	if code := f.do(t, "GET", "/users", "", "", nil); code != 401 {
		t.Fatalf("no token: %d", code)
	}

	// alex counts today and the desktop client reports GoLand.
	f.do(t, "POST", "/ping", f.alex, "", nil)
	body := `{"app":"GoLand","since":"2026-10-19T08:30:00Z","day":"2026-10-19","usage":{"GoLand":3600,"Chrome":600}}`
	if code := f.do(t, "POST", "/presence", f.alex, body, nil); code != 204 {
		t.Fatalf("presence: %d", code)
	}

	r := f.users(t, f.sasha, "")
	if r.Me != "sasha" || r.Total != 2 || r.Sort != "streak" || r.Filter != "all" || r.Page != 1 || r.Limit != 50 {
		t.Fatalf("defaults: %+v", r)
	}
	if names(r.Users) != "alex,sasha" || r.Users[0].Pos != 1 || r.Users[1].Pos != 2 {
		t.Fatalf("streak order: %s", names(r.Users))
	}
	a := r.Users[0]
	if a.Streak != 1 || !a.DoneToday || a.State != "safe" {
		t.Fatalf("alex row: %+v", a)
	}
	if a.Presence == nil || !a.Presence.Online || a.Presence.App != "GoLand" {
		t.Fatalf("alex presence: %+v", a.Presence)
	}
	if len(a.Week) != 7 || a.Week[6] != 4200 || a.Week[0] != 0 {
		t.Fatalf("alex week: %v", a.Week)
	}
	if want, _ := progress.XPFor(1, 0, 0); a.XP != want || a.Rank != 1 {
		t.Fatalf("alex xp %d rank %d, want xp %d", a.XP, a.Rank, want)
	}
	if r.MeRow == nil || r.MeRow.Name != "sasha" || r.MeRow.Pos != 2 {
		t.Fatalf("me row: %+v", r.MeRow)
	}
	if r.Counts != (api.LeaderCounts{All: 2, Online: 1, AtRisk: 0}) {
		t.Fatalf("counts: %+v", r.Counts)
	}

	// Search: substring, case-insensitive; "@" is an exact name.
	if r := f.users(t, f.alex, "?q=SAS"); names(r.Users) != "sasha" || r.Counts.All != 1 {
		t.Fatalf("q=SAS: %s %+v", names(r.Users), r.Counts)
	}
	if r := f.users(t, f.alex, "?q=@ale"); r.Total != 0 {
		t.Fatalf("@ale must not match alex: %s", names(r.Users))
	}
	if r := f.users(t, f.alex, "?q=@ALEX"); names(r.Users) != "alex" {
		t.Fatalf("@ALEX: %s", names(r.Users))
	}
	// The pinned own row ignores the search.
	if r := f.users(t, f.alex, "?q=sasha"); r.MeRow == nil || r.MeRow.Name != "alex" {
		t.Fatalf("me row under search: %+v", r.MeRow)
	}

	// Tabs and sorts.
	if r := f.users(t, f.alex, "?filter=online"); names(r.Users) != "alex" || r.Total != 1 {
		t.Fatalf("online: %s", names(r.Users))
	}
	if r := f.users(t, f.alex, "?sort=name"); names(r.Users) != "alex,sasha" {
		t.Fatalf("name: %s", names(r.Users))
	}
	if r := f.users(t, f.alex, "?sort=seen"); names(r.Users) != "alex,sasha" {
		t.Fatalf("seen: %s", names(r.Users))
	}

	// Pagination, clamped to the last page.
	if r := f.users(t, f.alex, "?limit=1&page=2"); names(r.Users) != "sasha" || r.Page != 2 || r.Total != 2 {
		t.Fatalf("page 2: %+v", r)
	}
	if r := f.users(t, f.alex, "?limit=1&page=9"); r.Page != 2 || names(r.Users) != "sasha" {
		t.Fatalf("clamped page: %+v", r)
	}
	if r := f.users(t, f.alex, "?limit=1000"); r.Limit != 100 {
		t.Fatalf("limit cap: %d", r.Limit)
	}
	if r := f.users(t, f.alex, "?q=nobody"); r.Total != 0 || r.Users == nil || r.Page != 1 {
		t.Fatalf("empty result must be a [] page 1: %+v", r)
	}

	for _, bad := range []string{"?sort=deadline", "?filter=friends", "?scope=mine", "?page=0", "?limit=x"} {
		if code := f.do(t, "GET", "/users"+bad, f.alex, "", nil); code != 400 {
			t.Fatalf("%s: %d, want 400", bad, code)
		}
	}

	// Next evening alex has not counted yet: within the warning window.
	f.now = time.Date(2026, 10, 20, 21, 0, 0, 0, time.UTC)
	r = f.users(t, f.sasha, "?filter=risk")
	if names(r.Users) != "alex" || r.Counts.AtRisk != 1 || len(r.AtRisk) != 0 {
		t.Fatalf("a stranger is not someone to push: %s %+v %s", names(r.Users), r.Counts, names(r.AtRisk))
	}
	befriend(t, f, f.sasha, "sasha", f.alex, "alex")
	r = f.users(t, f.sasha, "?filter=risk")
	if names(r.Users) != "alex" || r.Counts.AtRisk != 1 || names(r.AtRisk) != "alex" {
		t.Fatalf("risk: %s %+v %s", names(r.Users), r.Counts, names(r.AtRisk))
	}
	if r.AtRisk[0].SecondsLeft != 3*3600 {
		t.Fatalf("seconds left: %d", r.AtRisk[0].SecondsLeft)
	}
	// Presence went stale: alex is offline now.
	if r := f.users(t, f.sasha, "?filter=online"); r.Total != 0 {
		t.Fatalf("stale presence must be offline: %s", names(r.Users))
	}
}

// XP on the leaderboard matches /progress for the same data.
func TestUsersXPMatchesProgress(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.srv.CheckDeadlines(ctx) // both missed yesterday → a failure each
	for day := range 3 {
		f.now = time.Date(2026, 10, 19+day, 9, 0, 0, 0, time.UTC)
		f.do(t, "POST", "/ping", f.alex, "", nil)
		f.srv.CheckDeadlines(ctx) // stores the achievements just earned
	}
	var pr api.ProgressResponse
	if code := f.do(t, "GET", "/progress", f.alex, "", &pr); code != 200 {
		t.Fatalf("progress: %d", code)
	}
	want := map[string]api.UserProgress{}
	for _, u := range pr.Users {
		want[u.Name] = u
	}
	for _, row := range f.users(t, f.alex, "?sort=xp").Users {
		if row.XP != want[row.Name].XP || row.Rank != want[row.Name].Rank {
			t.Fatalf("%s: board xp %d rank %d, progress xp %d rank %d",
				row.Name, row.XP, row.Rank, want[row.Name].XP, want[row.Name].Rank)
		}
	}
}

// A few thousand users stay well inside one request.
func TestUsersManyUsers(t *testing.T) {
	if testing.Short() {
		t.Skip("slow")
	}
	f := setup(t)
	ctx := context.Background()
	for i := range 3000 {
		if _, _, err := f.srv.Store.CreateUser(ctx, store.User{Name: fmt.Sprintf("gopher%04d", i), Timezone: "Europe/Kyiv"}, f.now); err != nil {
			t.Fatal(err)
		}
	}
	start := time.Now()
	r := f.users(t, f.alex, "?q=gopher12&sort=name&limit=20")
	if r.Total != 100 || len(r.Users) != 20 || r.Users[0].Name != "gopher1200" {
		t.Fatalf("search in 3002 users: total %d first %v", r.Total, names(r.Users))
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("too slow: %s", d)
	}
}
