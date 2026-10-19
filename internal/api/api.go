// Package api holds the JSON contract shared by the server and the client.
package api

import "time"

// Failure describes the most recent missed deadline of a user.
type Failure struct {
	ID  int64     `json:"id"`
	Day string    `json:"day"`
	At  time.Time `json:"at"`
}

// UserStatus is the current status of one participant.
type UserStatus struct {
	Name          string    `json:"name"`
	Timezone      string    `json:"timezone"`
	Streak        int       `json:"streak"`
	Best          int       `json:"best"`
	LastActiveDay string    `json:"last_active_day"`
	DoneToday     bool      `json:"done_today"`
	State         string    `json:"state"` // safe | pending | warning | failed
	Deadline      time.Time `json:"deadline"`
	SecondsLeft   int64     `json:"seconds_left"`
	LastFailure   *Failure  `json:"last_failure,omitempty"`

	// Discord-like presence: what the user has in the foreground right now.
	Presence *Presence `json:"presence,omitempty"`
	// Time per program today (user's own day), longest first.
	TodayApps []AppTime `json:"today_apps,omitempty"`
}

// AppTime is time spent in one program.
type AppTime struct {
	App     string `json:"app"`
	Seconds int64  `json:"seconds"`
}

// Presence is the current foreground program of a user.
type Presence struct {
	App       string    `json:"app,omitempty"` // "" = nothing / away
	Since     time.Time `json:"since,omitempty"`
	Idle      bool      `json:"idle"`
	Online    bool      `json:"online"` // the client reported recently
	UpdatedAt time.Time `json:"updated_at"`
}

// PresenceRequest is sent by the desktop client to POST /presence.
// Usage holds full totals for Day (not deltas), so retries are harmless.
type PresenceRequest struct {
	App   string           `json:"app"`
	Since time.Time        `json:"since,omitempty"`
	Idle  bool             `json:"idle"`
	Day   string           `json:"day"`
	Usage map[string]int64 `json:"usage"`
}

// ActivityUser is one user's program statistics over a period.
type ActivityUser struct {
	Name         string    `json:"name"`
	TotalSeconds int64     `json:"total_seconds"`
	Apps         []AppTime `json:"apps"`
	ByDay        []DayTime `json:"by_day"`
}

// DayTime is the tracked time on one day.
type DayTime struct {
	Day     string `json:"day"`
	Seconds int64  `json:"seconds"`
}

// ActivityResponse is returned by GET /activity?days=N.
type ActivityResponse struct {
	Days  int            `json:"days"`
	Users []ActivityUser `json:"users"`
}

// StatusResponse is returned by GET /status.
type StatusResponse struct {
	Me         string       `json:"me"`
	ServerTime time.Time    `json:"server_time"`
	WarnHours  float64      `json:"warn_hours"`
	Users      []UserStatus `json:"users"`
}

// PingRequest is the optional body of POST /ping.
type PingRequest struct {
	Note   string `json:"note,omitempty"`
	Source string `json:"source,omitempty"` // "manual" (default) | "local_git"
}

// PingResponse is returned by POST /ping.
type PingResponse struct {
	Counted bool       `json:"counted"` // false if today was already counted
	User    UserStatus `json:"user"`
}

// Error is the body of any non-2xx response.
type Error struct {
	Error string `json:"error"`
}

// RankDef is one step of the rank ladder. Rank = total XP, so it can drop.
type RankDef struct {
	Level int    `json:"level"`
	Key   string `json:"key"`
	Name  string `json:"name"`
	MinXP int    `json:"min_xp"`
	Code  string `json:"code"` // the Go snippet the rank stands for
}

// AchievementDef describes one achievement. Texts live in the frontend,
// keyed by ID.
type AchievementDef struct {
	ID       string `json:"id"`
	Category string `json:"category"` // streak | go | tests | duo | shame | secret
	XP       int    `json:"xp"`
	Target   int    `json:"target"`
	// Detector is "auto" (computed from activity history) or "diff"
	// (needs commit diff analysis — not implemented yet, never unlocks).
	Detector string `json:"detector"`
	Rule     string `json:"rule"` // human-readable detection rule
	Secret   bool   `json:"secret,omitempty"`
}

// AchievementState is one user's progress on one achievement.
type AchievementState struct {
	ID       string `json:"id"`
	Unlocked bool   `json:"unlocked"`
	Day      string `json:"day,omitempty"` // day it was unlocked (user's time zone)
	Progress int    `json:"progress"`
	Target   int    `json:"target"`
}

// UserProgress is the XP, rank and achievements of one user.
type UserProgress struct {
	Name           string             `json:"name"`
	XP             int                `json:"xp"`
	XPDays         int                `json:"xp_days"`
	XPAchievements int                `json:"xp_achievements"`
	XPPenalty      int                `json:"xp_penalty"` // already subtracted, >= 0
	Rank           int                `json:"rank"`       // RankDef.Level
	NextRankXP     int                `json:"next_rank_xp,omitempty"`
	ActiveDays     int                `json:"active_days"`
	Failures       int                `json:"failures"`
	Achievements   []AchievementState `json:"achievements"`
}

// ProgressResponse is returned by GET /progress.
type ProgressResponse struct {
	Me           string           `json:"me"`
	XPPerDay     int              `json:"xp_per_day"`
	XPPerFailure int              `json:"xp_per_failure"`
	Ranks        []RankDef        `json:"ranks"`
	Achievements []AchievementDef `json:"achievements"`
	Users        []UserProgress   `json:"users"`
}
