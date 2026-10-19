// Package progress turns a user's activity history into XP, a rank on the
// ladder and achievements. Everything is derived from history on every
// request — nothing is stored — so the rules can change without migrations.
// No I/O here, so it is fully unit-testable.
package progress

import (
	"sort"
	"time"

	"github.com/AlexSol/gachi-streak/internal/api"
	"github.com/AlexSol/gachi-streak/internal/streak"
)

const (
	// XPPerDay is earned for every counted day.
	XPPerDay = 30
	// XPPerFailure is taken for every missed deadline.
	XPPerFailure = 50
)

// Ranks is the ladder, lowest first. Rank = total XP, so it can drop.
var Ranks = []api.RankDef{
	{Level: 1, Key: "slave", Name: "Slave", MinXP: 0, Code: `fmt.Println("Hello, World")`},
	{Level: 2, Key: "jabroni", Name: "Jabroni", MinXP: 100, Code: `if err != nil { panic(err) }`},
	{Level: 3, Key: "boy_next_door", Name: "Boy Next Door", MinXP: 300, Code: `go func() { … }()`},
	{Level: 4, Key: "performance_artist", Name: "Performance Artist", MinXP: 700, Code: `select { case v := <-ch: }`},
	{Level: 5, Key: "gym_boss", Name: "Boss of This Gym", MinXP: 1500, Code: `ctx, cancel := context.WithTimeout(…)`},
	{Level: 6, Key: "deep_dark_gopher", Name: "Deep Dark Gopher", MinXP: 3000, Code: `runtime.GOMAXPROCS(0)`},
	{Level: 7, Key: "dungeon_master", Name: "Dungeon Master", MinXP: 6000, Code: `go tool pprof -http=:8080`},
}

const (
	auto = "auto"
	diff = "diff" // needs commit diff analysis — planned, never unlocks yet
)

// Achievements is the catalog. Texts are in the frontend, keyed by ID.
var Achievements = []api.AchievementDef{
	{ID: "streak_7", Category: "streak", XP: 30, Target: 7, Detector: auto, Rule: "best_streak >= 7"},
	{ID: "streak_30", Category: "streak", XP: 100, Target: 30, Detector: auto, Rule: "best_streak >= 30"},
	{ID: "streak_100", Category: "streak", XP: 300, Target: 100, Detector: auto, Rule: "best_streak >= 100"},
	{ID: "streak_365", Category: "streak", XP: 1000, Target: 365, Detector: auto, Rule: "best_streak >= 365"},

	{ID: "goroutine", Category: "go", XP: 40, Target: 1, Detector: diff, Rule: `diff: go func(`},
	{ID: "channel", Category: "go", XP: 40, Target: 1, Detector: diff, Rule: `diff: make(chan`},
	{ID: "select", Category: "go", XP: 50, Target: 1, Detector: diff, Rule: `AST: SelectStmt, >= 2 case`},
	{ID: "context", Category: "go", XP: 60, Target: 1, Detector: diff, Rule: `diff: context.With(Cancel|Timeout|Deadline)`},
	{ID: "generics", Category: "go", XP: 60, Target: 1, Detector: diff, Rule: `AST: FuncType.TypeParams != nil`},
	{ID: "errwrap", Category: "go", XP: 40, Target: 5, Detector: diff, Rule: `diff: fmt.Errorf("…%w") ×5`},

	{ID: "first_test", Category: "tests", XP: 40, Target: 1, Detector: diff, Rule: `*_test.go + func Test`},
	{ID: "benchmark", Category: "tests", XP: 60, Target: 1, Detector: diff, Rule: `diff: func Benchmark…(b *testing.B)`},
	{ID: "table_test", Category: "tests", XP: 50, Target: 1, Detector: diff, Rule: `diff: []struct{…} + t.Run(`},
	{ID: "fuzz", Category: "tests", XP: 80, Target: 1, Detector: diff, Rule: `diff: func Fuzz…(f *testing.F)`},

	{ID: "duo_7", Category: "duo", XP: 50, Target: 7, Detector: auto, Rule: "both active, 7 days in a row"},
	{ID: "duo_30", Category: "duo", XP: 120, Target: 30, Detector: auto, Rule: "30 days when both were active"},

	{ID: "first_fail", Category: "shame", XP: 0, Target: 1, Detector: auto, Rule: "event: deadline missed"},
	{ID: "midnight", Category: "shame", XP: 5, Target: 1, Detector: auto, Rule: "activity at 23:50–23:59"},
	{ID: "comeback", Category: "shame", XP: 40, Target: 7, Detector: auto, Rule: "7-day streak after a failure"},

	{ID: "night_owl", Category: "secret", XP: 30, Target: 1, Detector: auto, Rule: "activity at 02:00–04:59", Secret: true},
	{ID: "nil_map", Category: "secret", XP: 50, Target: 1, Detector: diff, Rule: "hidden", Secret: true},
}

// History is everything progress is derived from, in the user's time zone.
type History struct {
	Days     []string    // distinct counted days, any order
	Times    []time.Time // local time of every activity
	Failures []string    // missed days, any order
}

func (h History) normalized() History {
	return History{Days: uniqSorted(h.Days), Times: h.Times, Failures: uniqSorted(h.Failures)}
}

// Compute returns the progress of one user. partner may be nil.
func Compute(name string, h History, partner *History) api.UserProgress {
	h = h.normalized()
	var p *History
	if partner != nil {
		pn := partner.normalized()
		p = &pn
	}

	up := api.UserProgress{Name: name, ActiveDays: len(h.Days), Failures: len(h.Failures)}
	for _, def := range Achievements {
		st := evaluate(def, h, p)
		if st.Unlocked {
			up.XPAchievements += def.XP
		}
		up.Achievements = append(up.Achievements, st)
	}
	up.XPDays = XPPerDay * len(h.Days)
	gross := up.XPDays + up.XPAchievements
	up.XPPenalty = min(XPPerFailure*len(h.Failures), gross)
	up.XP = gross - up.XPPenalty
	r, next := RankFor(up.XP)
	up.Rank = r.Level
	if next != nil {
		up.NextRankXP = next.MinXP
	}
	return up
}

// RankFor returns the rank for xp and the next one (nil at the top).
func RankFor(xp int) (api.RankDef, *api.RankDef) {
	cur := Ranks[0]
	for i, r := range Ranks {
		if xp < r.MinXP {
			return cur, &Ranks[i]
		}
		cur = r
	}
	return cur, nil
}

func evaluate(def api.AchievementDef, h History, p *History) api.AchievementState {
	st := api.AchievementState{ID: def.ID, Target: def.Target}
	set := func(progress int, day string) {
		st.Progress = min(progress, def.Target)
		if day != "" {
			st.Unlocked, st.Day, st.Progress = true, day, def.Target
		}
	}
	switch def.ID {
	case "streak_7", "streak_30", "streak_100", "streak_365":
		set(runReach(h.Days, def.Target))
	case "first_fail":
		if len(h.Failures) > 0 {
			set(1, h.Failures[0])
		}
	case "comeback":
		if len(h.Failures) > 0 {
			set(runReach(after(h.Days, h.Failures[0]), def.Target))
		}
	case "midnight":
		set(firstTime(h.Times, func(t time.Time) bool { return t.Hour() == 23 && t.Minute() >= 50 }))
	case "night_owl":
		set(firstTime(h.Times, func(t time.Time) bool { return t.Hour() >= 2 && t.Hour() < 5 }))
	case "duo_7":
		if p != nil {
			set(runReach(intersect(h.Days, p.Days), def.Target))
		}
	case "duo_30":
		if p != nil {
			set(countReach(intersect(h.Days, p.Days), def.Target))
		}
	}
	// "diff" detectors are not implemented yet: they stay locked at 0.
	return st
}

// runReach returns the longest run of consecutive days and the day the
// first run reached target ("" if never). days must be sorted and unique.
func runReach(days []string, target int) (best int, reached string) {
	run := 0
	for i, d := range days {
		if i > 0 && streak.AddDays(days[i-1], 1) == d {
			run++
		} else {
			run = 1
		}
		best = max(best, run)
		if reached == "" && run >= target {
			reached = d
		}
	}
	return best, reached
}

// countReach returns how many days there are and the target-th day.
func countReach(days []string, target int) (int, string) {
	if len(days) >= target && target > 0 {
		return len(days), days[target-1]
	}
	return len(days), ""
}

func firstTime(times []time.Time, ok func(time.Time) bool) (int, string) {
	var first time.Time
	n := 0
	for _, t := range times {
		if ok(t) {
			n++
			if first.IsZero() || t.Before(first) {
				first = t
			}
		}
	}
	if n == 0 {
		return 0, ""
	}
	return n, first.Format(streak.DayLayout)
}

func after(days []string, day string) []string {
	i := sort.SearchStrings(days, day)
	for i < len(days) && days[i] <= day {
		i++
	}
	return days[i:]
}

func intersect(a, b []string) []string {
	var out []string
	for i, j := 0, 0; i < len(a) && j < len(b); {
		switch {
		case a[i] == b[j]:
			out = append(out, a[i])
			i++
			j++
		case a[i] < b[j]:
			i++
		default:
			j++
		}
	}
	return out
}

func uniqSorted(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	w := 0
	for i, s := range out {
		if s == "" || (i > 0 && s == out[i-1]) {
			continue
		}
		out[w] = s
		w++
	}
	return out[:w]
}
