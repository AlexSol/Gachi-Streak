// Package progress turns a user's activity history into XP, a rank on the
// ladder and achievements. XP and rank are derived from history on every
// request; achievements are derived too, but once earned they are stored
// (History.Earned) and stay unlocked even if a rule changes later.
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
	auto = "auto" // activity history
	code = "code" // Go code in pushed commits (internal/codescan)
)

// Achievements is the catalog. Texts are in the frontend, keyed by ID.
var Achievements = []api.AchievementDef{
	{ID: "streak_7", Category: "streak", XP: 30, Target: 7, Detector: auto, Rule: "best_streak >= 7"},
	{ID: "streak_30", Category: "streak", XP: 100, Target: 30, Detector: auto, Rule: "best_streak >= 30"},
	{ID: "streak_100", Category: "streak", XP: 300, Target: 100, Detector: auto, Rule: "best_streak >= 100"},
	{ID: "streak_365", Category: "streak", XP: 1000, Target: 365, Detector: auto, Rule: "best_streak >= 365"},

	{ID: "goroutine", Category: "go", XP: 40, Target: 1, Detector: code, Rule: `AST: go statement`},
	{ID: "channel", Category: "go", XP: 40, Target: 1, Detector: code, Rule: `AST: make(chan T)`},
	{ID: "select", Category: "go", XP: 50, Target: 1, Detector: code, Rule: `AST: SelectStmt, >= 2 case`},
	{ID: "context", Category: "go", XP: 60, Target: 1, Detector: code, Rule: `AST: context.With(Cancel|Timeout|Deadline)`},
	{ID: "generics", Category: "go", XP: 60, Target: 1, Detector: code, Rule: `AST: FuncType.TypeParams != nil`},
	{ID: "errwrap", Category: "go", XP: 40, Target: 5, Detector: code, Rule: `AST: fmt.Errorf("…%w") ×5`},

	{ID: "first_test", Category: "tests", XP: 40, Target: 1, Detector: code, Rule: `*_test.go: func TestXxx(t *testing.T)`},
	{ID: "benchmark", Category: "tests", XP: 60, Target: 1, Detector: code, Rule: `func BenchmarkXxx(b *testing.B)`},
	{ID: "table_test", Category: "tests", XP: 50, Target: 1, Detector: code, Rule: `[]struct{…} + t.Run(name, func…)`},
	{ID: "fuzz", Category: "tests", XP: 80, Target: 1, Detector: code, Rule: `func FuzzXxx(f *testing.F)`},

	{ID: "duo_7", Category: "duo", XP: 50, Target: 7, Detector: auto, Rule: "both active, 7 days in a row"},
	{ID: "duo_30", Category: "duo", XP: 120, Target: 30, Detector: auto, Rule: "30 days when both were active"},

	{ID: "first_fail", Category: "shame", XP: 0, Target: 1, Detector: auto, Rule: "event: deadline missed"},
	{ID: "midnight", Category: "shame", XP: 5, Target: 1, Detector: auto, Rule: "activity at 23:50–23:59"},
	{ID: "comeback", Category: "shame", XP: 40, Target: 7, Detector: auto, Rule: "7-day streak after a failure"},

	{ID: "night_owl", Category: "secret", XP: 30, Target: 1, Detector: auto, Rule: "activity at 02:00–04:59", Secret: true},
	{ID: "nil_map", Category: "secret", XP: 50, Target: 1, Detector: code, Rule: "var m map[K]V; m[k] = v", Secret: true},
}

// History is everything progress is derived from, in the user's time zone.
type History struct {
	Days     []string    // distinct counted days, any order
	Times    []time.Time // local time of every activity
	Failures []string    // missed days, any order
	Code     []Finding   // constructs found in pushed code
	// Earned are achievements already stored as unlocked: ID → day.
	// They stay unlocked whatever the rules say now.
	Earned map[string]string
}

// Finding is how many times a code construct (codescan kind, equal to the
// achievement ID) appeared in code pushed on a day.
type Finding struct {
	Kind  string
	Day   string
	Count int
}

func (h History) normalized() History {
	h.Days = uniqSorted(h.Days)
	h.Failures = uniqSorted(h.Failures)
	return h
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
		if day, ok := h.Earned[def.ID]; ok {
			st.Unlocked, st.Day, st.Progress = true, day, def.Target
		}
		if st.Unlocked {
			up.XPAchievements += def.XP
		}
		up.Achievements = append(up.Achievements, st)
	}
	up.XPDays = XPPerDay * len(h.Days)
	up.XP, up.XPPenalty = XPFor(len(h.Days), up.XPAchievements, len(h.Failures))
	r, next := RankFor(up.XP)
	up.Rank = r.Level
	if next != nil {
		up.NextRankXP = next.MinXP
	}
	return up
}

// XPFor returns total XP and the penalty already taken from it. The
// penalty never pushes XP below zero.
func XPFor(activeDays, achievementXP, failures int) (xp, penalty int) {
	gross := XPPerDay*activeDays + achievementXP
	penalty = min(XPPerFailure*failures, gross)
	return gross - penalty, penalty
}

// AchievementXP sums the XP of achievement IDs (unknown IDs count 0).
func AchievementXP(ids []string) int {
	sum := 0
	for _, id := range ids {
		for _, def := range Achievements {
			if def.ID == id {
				sum += def.XP
				break
			}
		}
	}
	return sum
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
	if def.Detector == code {
		set(codeReach(h.Code, def.ID, def.Target))
	}
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

// codeReach sums the findings of one kind and returns the total and the day
// the running total first reached target ("" if never).
func codeReach(fs []Finding, kind string, target int) (total int, reached string) {
	fs = append([]Finding(nil), fs...)
	sort.SliceStable(fs, func(i, j int) bool { return fs[i].Day < fs[j].Day })
	for _, f := range fs {
		if f.Kind != kind {
			continue
		}
		total += f.Count
		if reached == "" && total >= target {
			reached = f.Day
		}
	}
	return total, reached
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
