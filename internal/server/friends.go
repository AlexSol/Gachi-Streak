package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/AlexSol/gachi-streak/internal/api"
	"github.com/AlexSol/gachi-streak/internal/store"
)

// FriendRepository stores friend requests and friendships.
type FriendRepository interface {
	Request(ctx context.Context, from, to int64, now time.Time) (store.Relation, error)
	Accept(ctx context.Context, me, from int64, now time.Time) error
	Decline(ctx context.Context, me, from int64) error
	Cancel(ctx context.Context, me, to int64) error
	Remove(ctx context.Context, me, other int64) error
	Relations(ctx context.Context, me int64) (map[int64]store.Relation, error)
	Requests(ctx context.Context, me int64) (incoming, outgoing []store.FriendRequest, err error)
}

// handleFriendRequests lists my pending requests: GET /friends/requests.
func (s *Server) handleFriendRequests(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	in, out, err := s.Friends.Requests(r.Context(), me.ID)
	if err != nil {
		s.Log.Error("friend requests", "user", me.Name, "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, api.FriendRequestsResponse{Incoming: requestViews(in), Outgoing: requestViews(out)})
}

func requestViews(rs []store.FriendRequest) []api.FriendRequest {
	out := make([]api.FriendRequest, 0, len(rs))
	for _, fr := range rs {
		out = append(out, api.FriendRequest{Name: fr.Name, Streak: fr.Streak, Best: fr.Best, At: fr.CreatedAt.UTC()})
	}
	return out
}

// handleFriendAdd sends a request: POST /friends/requests {"name": "..."}.
// If they have already asked me, this accepts their request instead.
func (s *Server) handleFriendAdd(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	var req api.FriendAddRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad json: "+err.Error())
		return
	}
	other, ok := s.friendTarget(w, r, me, strings.TrimSpace(req.Name))
	if !ok {
		return
	}
	rel, err := s.Friends.Request(r.Context(), me.ID, other.ID, s.Now())
	if err != nil {
		s.Log.Error("friend request", "from", me.Name, "to", other.Name, "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	s.Log.Info("friend request", "from", me.Name, "to", other.Name, "relation", rel)
	writeJSON(w, http.StatusOK, api.FriendResponse{Name: other.Name, Relation: string(rel)})
}

// friendAction builds a handler for an action on the user in {name}.
// act returns store.ErrNotFound when there is nothing to act on; result
// is the relation once the action succeeded.
func (s *Server) friendAction(name string, result store.Relation, act func(ctx context.Context, me, other int64) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		me := userFrom(r)
		other, ok := s.friendTarget(w, r, me, r.PathValue("name"))
		if !ok {
			return
		}
		err := act(r.Context(), me.ID, other.ID)
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "no such request or friendship")
			return
		}
		if err != nil {
			s.Log.Error("friend "+name, "user", me.Name, "other", other.Name, "err", err)
			writeErr(w, http.StatusInternalServerError, "internal error")
			return
		}
		s.Log.Info("friend "+name, "user", me.Name, "other", other.Name)
		writeJSON(w, http.StatusOK, api.FriendResponse{Name: other.Name, Relation: string(result)})
	}
}

// friendTarget resolves the other user of a friend action and rejects
// empty names, unknown users and the caller themself.
func (s *Server) friendTarget(w http.ResponseWriter, r *http.Request, me store.User, name string) (store.User, bool) {
	if name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return store.User{}, false
	}
	if name == me.Name {
		writeErr(w, http.StatusBadRequest, "cannot befriend yourself")
		return store.User{}, false
	}
	other, err := s.Store.UserByName(r.Context(), name)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "no such user")
		return store.User{}, false
	}
	if err != nil {
		s.Log.Error("friend target", "name", name, "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return store.User{}, false
	}
	return other, true
}

func (s *Server) friendRoutes(mux *http.ServeMux) {
	mux.Handle("GET /friends/requests", s.auth(http.HandlerFunc(s.handleFriendRequests)))
	mux.Handle("POST /friends/requests", s.auth(http.HandlerFunc(s.handleFriendAdd)))
	mux.Handle("POST /friends/requests/{name}/accept", s.auth(s.friendAction("accept", store.RelFriend,
		func(ctx context.Context, me, other int64) error { return s.Friends.Accept(ctx, me, other, s.Now()) })))
	mux.Handle("POST /friends/requests/{name}/decline", s.auth(s.friendAction("decline", store.RelNone,
		func(ctx context.Context, me, other int64) error { return s.Friends.Decline(ctx, me, other) })))
	mux.Handle("DELETE /friends/requests/{name}", s.auth(s.friendAction("cancel", store.RelNone,
		func(ctx context.Context, me, other int64) error { return s.Friends.Cancel(ctx, me, other) })))
	mux.Handle("DELETE /friends/{name}", s.auth(s.friendAction("remove", store.RelNone,
		func(ctx context.Context, me, other int64) error { return s.Friends.Remove(ctx, me, other) })))
}
