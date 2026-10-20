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
	// Detector is "auto" (computed from activity history) or "code"
	// (Go constructs found in commits pushed to GitHub).
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
	// New is set on the requesting user's own unlocks they have not been
	// notified about yet (see POST /achievements/seen).
	New bool `json:"new,omitempty"`
	// Packed is set on the requesting user's own unlocks they have not
	// unpacked in the web app yet (see POST /achievements/{id}/open).
	Packed bool `json:"packed,omitempty"`
}

// UserProgress is the XP, rank and achievements of one user.
type UserProgress struct {
	Name           string `json:"name"`
	XP             int    `json:"xp"`
	XPDays         int    `json:"xp_days"`
	XPAchievements int    `json:"xp_achievements"`
	XPPenalty      int    `json:"xp_penalty"` // already subtracted, >= 0
	Rank           int    `json:"rank"`       // RankDef.Level
	NextRankXP     int    `json:"next_rank_xp,omitempty"`
	ActiveDays     int    `json:"active_days"`
	Failures       int    `json:"failures"`
	// CodeTracking is true when the user's pushes are analyzed (GitHub
	// login set), i.e. "code" achievements can unlock.
	CodeTracking bool               `json:"code_tracking"`
	Achievements []AchievementState `json:"achievements"`
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

// LeaderRow is one participant of the leaderboard (GET /users).
type LeaderRow struct {
	Pos           int       `json:"pos"` // 1-based place in the current sort
	Name          string    `json:"name"`
	Streak        int       `json:"streak"`
	Best          int       `json:"best"`
	State         string    `json:"state"` // safe | pending | warning | failed
	DoneToday     bool      `json:"done_today"`
	Deadline      time.Time `json:"deadline"`
	SecondsLeft   int64     `json:"seconds_left"`
	LastActiveDay string    `json:"last_active_day"`
	XP            int       `json:"xp"`
	Rank          int       `json:"rank"` // RankDef.Level
	RankName      string    `json:"rank_name"`
	Presence      *Presence `json:"presence,omitempty"`
	// Week is tracked seconds per day for the user's last 7 days, oldest first.
	Week []int64 `json:"week"`
	// Relation is how this user relates to the caller:
	// me | friend | incoming | outgoing, empty for strangers.
	Relation string `json:"relation,omitempty"`
}

// LeaderScopes are the sizes of the leaderboard scopes and the number of
// friend requests waiting for the caller. They ignore q and filter.
type LeaderScopes struct {
	Friends  int `json:"friends"`  // the caller and their friends
	All      int `json:"all"`      // every participant
	Incoming int `json:"incoming"` // pending requests sent to the caller
}

// LeaderCounts are the tab sizes after the search query is applied.
type LeaderCounts struct {
	All    int `json:"all"`
	Online int `json:"online"`
	AtRisk int `json:"at_risk"`
}

// UsersResponse is returned by GET /users.
type UsersResponse struct {
	Me     string       `json:"me"`
	Scope  string       `json:"scope"` // friends | all
	Scopes LeaderScopes `json:"scopes"`
	Query  string       `json:"q"`
	Filter string       `json:"filter"` // all | online | risk
	Sort   string       `json:"sort"`   // streak | best | xp | name | seen
	Page   int          `json:"page"`   // 1-based, clamped to the last page
	Limit  int          `json:"limit"`
	Total  int          `json:"total"` // rows matching q and filter
	Counts LeaderCounts `json:"counts"`
	Users  []LeaderRow  `json:"users"`
	// MeRow is the caller's own row in the current sort (ignores q and
	// filter), so the page can pin it when it is not on the current page.
	MeRow *LeaderRow `json:"me_row,omitempty"`
	// AtRisk are the caller's friends (and the caller) whose deadline is
	// closest and who have not counted today yet (state "warning"),
	// nearest first, at most 4. Same in both scopes.
	AtRisk []LeaderRow `json:"at_risk"`
}

// FriendRequest is a pending friend request: the other person and when
// it was sent.
type FriendRequest struct {
	Name   string    `json:"name"`
	Streak int       `json:"streak"`
	Best   int       `json:"best"`
	At     time.Time `json:"at"`
}

// FriendRequestsResponse is returned by GET /friends/requests.
type FriendRequestsResponse struct {
	Incoming []FriendRequest `json:"incoming"` // sent to me, newest first
	Outgoing []FriendRequest `json:"outgoing"` // sent by me, newest first
}

// FriendAddRequest is the body of POST /friends/requests.
type FriendAddRequest struct {
	Name string `json:"name"`
}

// FriendResponse is returned by the friend actions: the other user and
// how they relate to the caller now (friend | outgoing | empty).
type FriendResponse struct {
	Name     string `json:"name"`
	Relation string `json:"relation"`
}

// ProfileDay is one day in the user's activity heatmap.
type ProfileDay struct {
	Day     string `json:"day"`
	Counted bool   `json:"counted"`
	Missed  bool   `json:"missed"`
}

// ProfileCompareDay contains both users' activity and tracked time for a day.
type ProfileCompareDay struct {
	Day         string `json:"day"`
	MeCounted   bool   `json:"me_counted"`
	UserCounted bool   `json:"user_counted"`
	MeSeconds   int64  `json:"me_seconds"`
	UserSeconds int64  `json:"user_seconds"`
}

// ProfileCompareTotals is the total counted days and tracked time in a 30-day comparison.
type ProfileCompareTotals struct {
	MeCounted   int   `json:"me_counted"`
	UserCounted int   `json:"user_counted"`
	MeSeconds   int64 `json:"me_seconds"`
	UserSeconds int64 `json:"user_seconds"`
}

// ProfileComparison compares the requesting user with a profile over 30 days.
type ProfileComparison struct {
	Days   []ProfileCompareDay  `json:"days"`
	Totals ProfileCompareTotals `json:"totals"`
}

// ProfileApp is one program in the user's top tracked programs.
type ProfileApp struct {
	App     string `json:"app"`
	Seconds int64  `json:"seconds"`
}

// ProfileAchievement is a recently unlocked achievement.
type ProfileAchievement struct {
	ID       string `json:"id"`
	Day      string `json:"day"`
	XP       int    `json:"xp"`
	Category string `json:"category"`
	Secret   bool   `json:"secret,omitempty"`
}

// ProfileLearningEntry is one of the requesting user's private learning notes.
type ProfileLearningEntry struct {
	Day  string `json:"day"`
	Note string `json:"note"`
}

// UserProfile is returned by GET /users/{name}/profile.
type UserProfile struct {
	Name               string                 `json:"name"`
	Timezone           string                 `json:"timezone"`
	CreatedDay         string                 `json:"created_day"`
	Relation           string                 `json:"relation"`
	Streak             int                    `json:"streak"`
	Best               int                    `json:"best"`
	ActiveDays         int                    `json:"active_days"`
	XP                 int                    `json:"xp"`
	Rank               int                    `json:"rank"`
	RankName           string                 `json:"rank_name"`
	Achievements       int                    `json:"achievements"`
	Failures           int                    `json:"failures"`
	Days               []ProfileDay           `json:"days"`
	Comparison         *ProfileComparison     `json:"comparison,omitempty"`
	TopApps            []ProfileApp           `json:"top_apps"`
	LatestAchievements []ProfileAchievement   `json:"latest_achievements"`
	LearningLog        []ProfileLearningEntry `json:"learning_log,omitempty"`
	SharedFriends      []string               `json:"shared_friends"`
}
