package streak

import (
	"testing"
	"time"
)

func mustLoc(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func TestApplyPing(t *testing.T) {
	cases := []struct {
		name              string
		last, today       string
		cur, best         int
		wantCur, wantBest int
		wantCounted       bool
	}{
		{"first ever", "", "2026-10-19", 0, 0, 1, 1, true},
		{"same day", "2026-10-19", "2026-10-19", 3, 5, 3, 5, false},
		{"next day", "2026-10-18", "2026-10-19", 3, 3, 4, 4, true},
		{"gap resets", "2026-10-16", "2026-10-19", 0, 9, 1, 9, true},
		{"month edge", "2026-10-31", "2026-11-01", 7, 7, 8, 8, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cur, best, counted := ApplyPing(c.last, c.today, c.cur, c.best)
			if cur != c.wantCur || best != c.wantBest || counted != c.wantCounted {
				t.Fatalf("got (%d,%d,%v) want (%d,%d,%v)", cur, best, counted, c.wantCur, c.wantBest, c.wantCounted)
			}
		})
	}
}

func TestEvaluateStates(t *testing.T) {
	loc := mustLoc(t, "Europe/Kyiv")
	warn := 5 * time.Hour
	at := func(h, m int) time.Time { return time.Date(2026, 10, 19, h, m, 0, 0, loc) }

	if s := Evaluate(at(10, 0), loc, "2026-10-19", "2026-10-01", warn).State; s != StateSafe {
		t.Fatalf("done today: got %s", s)
	}
	if s := Evaluate(at(10, 0), loc, "2026-10-18", "2026-10-01", warn).State; s != StatePending {
		t.Fatalf("morning: got %s", s)
	}
	if s := Evaluate(at(19, 0), loc, "2026-10-18", "2026-10-01", warn).State; s != StateWarning {
		t.Fatalf("19:00 (5h left): got %s", s)
	}
	if s := Evaluate(at(18, 59), loc, "2026-10-18", "2026-10-01", warn).State; s != StatePending {
		t.Fatalf("18:59: got %s", s)
	}
	if s := Evaluate(at(1, 0), loc, "2026-10-17", "2026-10-01", warn).State; s != StateFailed {
		t.Fatalf("missed yesterday: got %s", s)
	}
	// brand-new user created today is not punished for yesterday
	if s := Evaluate(at(1, 0), loc, "", "2026-10-19", warn).State; s != StatePending {
		t.Fatalf("new user: got %s", s)
	}
	// new user created yesterday who did nothing — failed
	if s := Evaluate(at(1, 0), loc, "", "2026-10-18", warn).State; s != StateFailed {
		t.Fatalf("new user missed first day: got %s", s)
	}
}

func TestDeadlineDST(t *testing.T) {
	loc := mustLoc(t, "Europe/Lisbon")
	// 2026-10-25 is the DST switch day in Europe; the day is 25h long.
	now := time.Date(2026, 10, 25, 0, 30, 0, 0, loc)
	dl := Deadline(now, loc)
	want := time.Date(2026, 10, 26, 0, 0, 0, 0, loc)
	if !dl.Equal(want) {
		t.Fatalf("deadline %v want %v", dl, want)
	}
	if left := dl.Sub(now); left != 24*time.Hour+30*time.Minute {
		t.Fatalf("left %v", left)
	}
}

func TestMissedDay(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 10, 19, 0, 1, 0, 0, loc)
	if d, ok := MissedDay(now, loc, "2026-10-17", "2026-10-01"); !ok || d != "2026-10-18" {
		t.Fatalf("got %q %v", d, ok)
	}
	if _, ok := MissedDay(now, loc, "2026-10-18", "2026-10-01"); ok {
		t.Fatal("yesterday was active, nothing missed")
	}
}
