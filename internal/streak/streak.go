// Package streak holds the pure domain rules: what "a day" is for a user,
// when the deadline is, how a ping changes the streak and which punishment
// state a user is in. No I/O here, so everything is unit-testable.
package streak

import (
	"time"
)

// DayLayout is how calendar days are stored and compared ("2026-10-19").
// Lexicographic order of such strings equals chronological order.
const DayLayout = "2006-01-02"

// State is the punishment state of a user right now.
type State string

const (
	// StateSafe — activity for today is already recorded.
	StateSafe State = "safe"
	// StatePending — nothing yet today, but the deadline is still far away.
	StatePending State = "pending"
	// StateWarning — nothing yet today and less than the warning window is left.
	StateWarning State = "warning"
	// StateFailed — the previous deadline was missed and nothing happened today yet.
	StateFailed State = "failed"
)

// DayOf returns the calendar day of t in loc.
func DayOf(t time.Time, loc *time.Location) string {
	return t.In(loc).Format(DayLayout)
}

// AddDays shifts a day string by n days. Invalid input is returned as is.
func AddDays(day string, n int) string {
	t, err := time.Parse(DayLayout, day)
	if err != nil {
		return day
	}
	return t.AddDate(0, 0, n).Format(DayLayout)
}

// Deadline is the end of the current local day (next local midnight).
// DST-safe: computed through the calendar, not by adding 24h.
func Deadline(now time.Time, loc *time.Location) time.Time {
	l := now.In(loc)
	return time.Date(l.Year(), l.Month(), l.Day()+1, 0, 0, 0, 0, loc)
}

// ApplyPing returns the streak values after an activity on `today`.
// Repeated activity on the same day does not change the streak.
func ApplyPing(lastActiveDay, today string, current, best int) (newCurrent, newBest int, counted bool) {
	switch {
	case lastActiveDay == today:
		return current, best, false
	case lastActiveDay != "" && AddDays(lastActiveDay, 1) == today:
		current++
	default:
		current = 1
	}
	if current > best {
		best = current
	}
	return current, best, true
}

// MissedDay reports the day whose deadline has just been missed, if any.
// Only "yesterday" is checked: the worker runs every minute, and one
// punishment per missed deadline is the rule.
// createdDay is the first day the user is accountable for.
func MissedDay(now time.Time, loc *time.Location, lastActiveDay, createdDay string) (string, bool) {
	yesterday := AddDays(DayOf(now, loc), -1)
	if yesterday < createdDay {
		return "", false
	}
	if lastActiveDay >= yesterday { // "" sorts before any day
		return "", false
	}
	return yesterday, true
}

// Evaluation is the computed status of a user at a moment.
type Evaluation struct {
	State     State
	Today     string
	DoneToday bool
	Deadline  time.Time
	Left      time.Duration
}

// Evaluate computes the punishment state.
func Evaluate(now time.Time, loc *time.Location, lastActiveDay, createdDay string, warn time.Duration) Evaluation {
	today := DayOf(now, loc)
	dl := Deadline(now, loc)
	ev := Evaluation{
		Today:     today,
		DoneToday: lastActiveDay == today,
		Deadline:  dl,
		Left:      dl.Sub(now),
	}
	switch {
	case ev.DoneToday:
		ev.State = StateSafe
	case isMissed(now, loc, lastActiveDay, createdDay):
		ev.State = StateFailed
	case ev.Left <= warn:
		ev.State = StateWarning
	default:
		ev.State = StatePending
	}
	return ev
}

func isMissed(now time.Time, loc *time.Location, lastActiveDay, createdDay string) bool {
	_, ok := MissedDay(now, loc, lastActiveDay, createdDay)
	return ok
}
