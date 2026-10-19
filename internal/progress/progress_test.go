package progress

import (
	"testing"
	"time"

	"github.com/AlexSol/gachi-streak/internal/api"
	"github.com/AlexSol/gachi-streak/internal/codescan"
	"github.com/AlexSol/gachi-streak/internal/streak"
)

// days returns n consecutive days starting at from.
func days(from string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = streak.AddDays(from, i)
	}
	return out
}

func ach(t *testing.T, up api.UserProgress, id string) api.AchievementState {
	t.Helper()
	for _, a := range up.Achievements {
		if a.ID == id {
			return a
		}
	}
	t.Fatalf("achievement %q missing", id)
	return api.AchievementState{}
}

func TestRankFor(t *testing.T) {
	cases := []struct {
		xp, level, next int
	}{
		{0, 1, 100}, {99, 1, 100}, {100, 2, 300}, {699, 3, 700}, {700, 4, 1500}, {5999, 6, 6000}, {6000, 7, 0}, {99999, 7, 0},
	}
	for _, c := range cases {
		r, next := RankFor(c.xp)
		gotNext := 0
		if next != nil {
			gotNext = next.MinXP
		}
		if r.Level != c.level || gotNext != c.next {
			t.Errorf("RankFor(%d) = %d, next %d; want %d, next %d", c.xp, r.Level, gotNext, c.level, c.next)
		}
	}
}

func TestEmptyHistory(t *testing.T) {
	up := Compute("alex", History{}, nil)
	if up.XP != 0 || up.Rank != 1 || up.NextRankXP != 100 {
		t.Fatalf("got %+v", up)
	}
	if len(up.Achievements) != len(Achievements) {
		t.Fatalf("want every achievement listed, got %d", len(up.Achievements))
	}
}

func TestStreakAchievementsAndXP(t *testing.T) {
	d := days("2026-10-01", 8)
	d = append(d, d[3], "") // duplicates and blanks are ignored
	up := Compute("alex", History{Days: d}, nil)

	if up.ActiveDays != 8 || up.XPDays != 8*XPPerDay {
		t.Fatalf("days: %+v", up)
	}
	s7 := ach(t, up, "streak_7")
	if !s7.Unlocked || s7.Day != "2026-10-07" || s7.Progress != 7 {
		t.Fatalf("streak_7 = %+v", s7)
	}
	s30 := ach(t, up, "streak_30")
	if s30.Unlocked || s30.Progress != 8 || s30.Target != 30 {
		t.Fatalf("streak_30 = %+v", s30)
	}
	if up.XPAchievements != 30 || up.XP != 8*XPPerDay+30 {
		t.Fatalf("xp: %+v", up)
	}
	if up.Rank != 2 { // 270 XP = Jabroni
		t.Fatalf("rank %d", up.Rank)
	}
}

func TestFailuresPenaltyAndComeback(t *testing.T) {
	before := days("2026-09-01", 3)
	afterFail := days("2026-09-06", 7)
	h := History{Days: append(before, afterFail...), Failures: []string{"2026-09-04", "2026-09-05"}}
	up := Compute("alex", h, nil)

	ff := ach(t, up, "first_fail")
	if !ff.Unlocked || ff.Day != "2026-09-04" {
		t.Fatalf("first_fail = %+v", ff)
	}
	cb := ach(t, up, "comeback")
	if !cb.Unlocked || cb.Day != "2026-09-12" {
		t.Fatalf("comeback = %+v", cb)
	}
	// 10 days * 30 + streak_7 30 + comeback 40 + first_fail 0 - 2 * 50
	if up.XPPenalty != 100 || up.XP != 300+30+40-100 {
		t.Fatalf("xp: %+v", up)
	}
}

func TestPenaltyNeverBelowZero(t *testing.T) {
	up := Compute("alex", History{Failures: []string{"2026-09-01", "2026-09-02"}}, nil)
	if up.XP != 0 || up.XPPenalty != 0 {
		t.Fatalf("got %+v", up)
	}
}

func TestComebackNeedsRunAfterFailure(t *testing.T) {
	h := History{Days: days("2026-09-01", 10), Failures: []string{"2026-09-11"}}
	if cb := ach(t, Compute("alex", h, nil), "comeback"); cb.Unlocked || cb.Progress != 0 {
		t.Fatalf("comeback = %+v", cb)
	}
}

func TestTimeOfDayAchievements(t *testing.T) {
	loc := time.FixedZone("x", 3*3600)
	h := History{Times: []time.Time{
		time.Date(2026, 10, 2, 23, 49, 0, 0, loc),
		time.Date(2026, 10, 3, 23, 55, 0, 0, loc),
		time.Date(2026, 10, 1, 23, 51, 0, 0, loc),
		time.Date(2026, 10, 4, 3, 10, 0, 0, loc),
	}}
	up := Compute("alex", h, nil)
	if m := ach(t, up, "midnight"); !m.Unlocked || m.Day != "2026-10-01" {
		t.Fatalf("midnight = %+v", m)
	}
	if n := ach(t, up, "night_owl"); !n.Unlocked || n.Day != "2026-10-04" {
		t.Fatalf("night_owl = %+v", n)
	}
}

func TestDuo(t *testing.T) {
	me := History{Days: days("2026-10-01", 10)}
	partner := History{Days: append(days("2026-10-01", 3), days("2026-10-05", 6)...)}
	up := Compute("alex", me, &partner)

	d7 := ach(t, up, "duo_7")
	if d7.Unlocked || d7.Progress != 6 {
		t.Fatalf("duo_7 = %+v", d7)
	}
	d30 := ach(t, up, "duo_30")
	if d30.Unlocked || d30.Progress != 9 {
		t.Fatalf("duo_30 = %+v", d30)
	}
	if solo := ach(t, Compute("alex", me, nil), "duo_7"); solo.Progress != 0 {
		t.Fatalf("no partner: %+v", solo)
	}
}

func TestCodeAchievementsNeedFindings(t *testing.T) {
	up := Compute("alex", History{Days: days("2026-01-01", 400)}, nil)
	for _, def := range Achievements {
		st := ach(t, up, def.ID)
		if def.Detector == code && st.Unlocked {
			t.Errorf("%s unlocked without code findings", def.ID)
		}
	}
	if up.Rank != 7 {
		t.Fatalf("400 days should be Dungeon Master, got %d (%d XP)", up.Rank, up.XP)
	}
}

func TestCodeAchievements(t *testing.T) {
	h := History{Code: []Finding{
		{Kind: "errwrap", Day: "2026-10-05", Count: 3},
		{Kind: "goroutine", Day: "2026-10-02", Count: 1},
		{Kind: "errwrap", Day: "2026-10-01", Count: 1},
		{Kind: "errwrap", Day: "2026-10-09", Count: 4},
	}}
	up := Compute("alex", h, nil)
	if g := ach(t, up, "goroutine"); !g.Unlocked || g.Day != "2026-10-02" {
		t.Fatalf("goroutine %+v", g)
	}
	// 1 + 3 = 4 by the 5th, the 5th wrap lands on the 9th.
	if e := ach(t, up, "errwrap"); !e.Unlocked || e.Day != "2026-10-09" || e.Progress != 5 {
		t.Fatalf("errwrap %+v", e)
	}
	if c := ach(t, up, "channel"); c.Unlocked {
		t.Fatalf("channel %+v", c)
	}
	if up.XPAchievements != 40+40 {
		t.Fatalf("xp %d", up.XPAchievements)
	}

	h.Code = h.Code[:3] // 4 wraps only
	if e := ach(t, Compute("alex", h, nil), "errwrap"); e.Unlocked || e.Progress != 4 {
		t.Fatalf("errwrap partial %+v", e)
	}
}

func TestEarnedStayUnlocked(t *testing.T) {
	// streak_7 was stored earlier; the history no longer proves it.
	h := History{Days: days("2026-10-01", 2), Earned: map[string]string{"streak_7": "2026-09-07", "gone": "2026-01-01"}}
	up := Compute("alex", h, nil)
	if s := ach(t, up, "streak_7"); !s.Unlocked || s.Day != "2026-09-07" || s.Progress != 7 {
		t.Fatalf("streak_7 %+v", s)
	}
	// Earned IDs missing from the catalog are ignored.
	if up.XPAchievements != 30 || len(up.Achievements) != len(Achievements) {
		t.Fatalf("up %+v", up)
	}
}

func TestCatalogIsConsistent(t *testing.T) {
	seen := map[string]bool{}
	for _, a := range Achievements {
		if seen[a.ID] {
			t.Errorf("duplicate id %s", a.ID)
		}
		seen[a.ID] = true
		if a.Target < 1 || (a.Detector != auto && a.Detector != code) {
			t.Errorf("bad def %+v", a)
		}
		if a.Detector == code && !codeKinds[a.ID] {
			t.Errorf("%s: no codescan kind with this id", a.ID)
		}
	}
	for i := 1; i < len(Ranks); i++ {
		if Ranks[i].MinXP <= Ranks[i-1].MinXP || Ranks[i].Level != i+1 {
			t.Errorf("ranks out of order at %d", i)
		}
	}
}

var codeKinds = map[string]bool{
	codescan.Goroutine: true, codescan.Channel: true, codescan.Select: true, codescan.Context: true,
	codescan.Generics: true, codescan.ErrWrap: true, codescan.FirstTest: true, codescan.Benchmark: true,
	codescan.TableTest: true, codescan.Fuzz: true, codescan.NilMap: true,
}
