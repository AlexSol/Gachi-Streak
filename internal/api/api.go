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
	Note string `json:"note,omitempty"`
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
