package server

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/AlexSol/gachi-streak/internal/api"
	"github.com/AlexSol/gachi-streak/internal/store"
)

// befriend makes two users friends through the API: from asks, to accepts.
func befriend(t *testing.T, f *fixture, fromTok, fromName, toTok, toName string) {
	t.Helper()
	if code := f.do(t, "POST", "/friends/requests", fromTok, `{"name":"`+toName+`"}`, nil); code != 200 {
		t.Fatalf("request %s → %s: %d", fromName, toName, code)
	}
	if code := f.do(t, "POST", "/friends/requests/"+fromName+"/accept", toTok, "", nil); code != 200 {
		t.Fatalf("accept %s by %s: %d", fromName, toName, code)
	}
}

func (f *fixture) requests(t *testing.T, token string) api.FriendRequestsResponse {
	t.Helper()
	var r api.FriendRequestsResponse
	if code := f.do(t, "GET", "/friends/requests", token, "", &r); code != 200 {
		t.Fatalf("GET /friends/requests: %d", code)
	}
	return r
}

func reqNames(rs []api.FriendRequest) string {
	var out []string
	for _, r := range rs {
		out = append(out, r.Name)
	}
	return strings.Join(out, ",")
}

func relations(rows []api.LeaderRow) string {
	var out []string
	for _, r := range rows {
		out = append(out, r.Name+"="+r.Relation)
	}
	return strings.Join(out, ",")
}

func TestFriendsAPI(t *testing.T) {
	f := setup(t)
	_, olya, err := f.srv.Store.CreateUser(context.Background(), store.User{Name: "olya", Timezone: "UTC"}, f.now)
	if err != nil {
		t.Fatal(err)
	}

	if code := f.do(t, "GET", "/friends/requests", "", "", nil); code != 401 {
		t.Fatalf("no token: %d", code)
	}

	// Nobody is a friend yet: the friends scope is just me.
	r := f.users(t, f.alex, "?scope=friends")
	if r.Scope != "friends" || names(r.Users) != "alex" || r.MeRow == nil || r.MeRow.Pos != 1 {
		t.Fatalf("friends scope, no friends: %+v", r)
	}
	if r.Scopes != (api.LeaderScopes{Friends: 1, All: 3, Incoming: 0}) {
		t.Fatalf("scopes: %+v", r.Scopes)
	}
	if r := f.users(t, f.alex, "?sort=name"); r.Scope != "all" || relations(r.Users) != "alex=me,olya=,sasha=" {
		t.Fatalf("all scope relations: %s", relations(r.Users))
	}

	// Bad requests.
	for _, c := range []struct {
		method, path, body string
		want               int
	}{
		{"POST", "/friends/requests", `{"name":"alex"}`, http.StatusBadRequest},
		{"POST", "/friends/requests", `{"name":""}`, http.StatusBadRequest},
		{"POST", "/friends/requests", `not json`, http.StatusBadRequest},
		{"POST", "/friends/requests", `{"name":"ghost"}`, http.StatusNotFound},
		{"POST", "/friends/requests/sasha/accept", "", http.StatusNotFound},
		{"POST", "/friends/requests/sasha/decline", "", http.StatusNotFound},
		{"DELETE", "/friends/requests/sasha", "", http.StatusNotFound},
		{"DELETE", "/friends/sasha", "", http.StatusNotFound},
		{"DELETE", "/friends/ghost", "", http.StatusNotFound},
	} {
		if code := f.do(t, c.method, c.path, f.alex, c.body, nil); code != c.want {
			t.Fatalf("%s %s %s: %d, want %d", c.method, c.path, c.body, code, c.want)
		}
	}

	// alex asks sasha and olya.
	var fr api.FriendResponse
	if code := f.do(t, "POST", "/friends/requests", f.alex, `{"name":" sasha "}`, &fr); code != 200 || fr != (api.FriendResponse{Name: "sasha", Relation: "outgoing"}) {
		t.Fatalf("request sasha: %d %+v", code, fr)
	}
	f.do(t, "POST", "/friends/requests", f.alex, `{"name":"olya"}`, nil)

	if rq := f.requests(t, f.alex); reqNames(rq.Outgoing) != "olya,sasha" || len(rq.Incoming) != 0 {
		t.Fatalf("alex requests: %+v", rq)
	}
	if rq := f.requests(t, f.sasha); reqNames(rq.Incoming) != "alex" || rq.Outgoing == nil {
		t.Fatalf("sasha requests: %+v", rq)
	}
	if r := f.users(t, f.sasha, "?sort=name"); r.Scopes.Incoming != 1 || relations(r.Users) != "alex=incoming,olya=,sasha=me" {
		t.Fatalf("sasha sees: %s %+v", relations(r.Users), r.Scopes)
	}

	// sasha accepts; olya declines; alex can ask olya again and then cancel.
	if code := f.do(t, "POST", "/friends/requests/alex/accept", f.sasha, "", &fr); code != 200 || fr.Relation != "friend" {
		t.Fatalf("accept: %d %+v", code, fr)
	}
	if code := f.do(t, "POST", "/friends/requests/alex/decline", olya, "", &fr); code != 200 || fr.Relation != "" {
		t.Fatalf("decline: %d %+v", code, fr)
	}
	f.do(t, "POST", "/friends/requests", f.alex, `{"name":"olya"}`, nil)
	if code := f.do(t, "DELETE", "/friends/requests/olya", f.alex, "", nil); code != 200 {
		t.Fatalf("cancel: %d", code)
	}
	if rq := f.requests(t, olya); len(rq.Incoming) != 0 {
		t.Fatalf("olya still has a request: %+v", rq)
	}

	// The friends scope now holds alex and sasha, ranked among themselves.
	r = f.users(t, f.alex, "?scope=friends&sort=name")
	if relations(r.Users) != "alex=me,sasha=friend" || r.Scopes != (api.LeaderScopes{Friends: 2, All: 3}) {
		t.Fatalf("friends scope: %s %+v", relations(r.Users), r.Scopes)
	}
	if r := f.users(t, f.alex, "?scope=friends&sort=name&q=sas"); names(r.Users) != "sasha" || r.Counts.All != 1 || r.MeRow.Pos != 1 {
		t.Fatalf("friends scope search: %s %+v", names(r.Users), r.Counts)
	}

	// Asking back an incoming request makes friends at once.
	f.do(t, "POST", "/friends/requests", olya, `{"name":"sasha"}`, nil)
	if f.do(t, "POST", "/friends/requests", f.sasha, `{"name":"olya"}`, &fr); fr.Relation != "friend" {
		t.Fatalf("mutual request: %+v", fr)
	}

	// Unfriend works from either side.
	if code := f.do(t, "DELETE", "/friends/alex", f.sasha, "", nil); code != 200 {
		t.Fatalf("remove: %d", code)
	}
	if r := f.users(t, f.alex, "?scope=friends"); names(r.Users) != "alex" {
		t.Fatalf("after remove: %s", names(r.Users))
	}
	if r := f.users(t, f.sasha, "?scope=friends&sort=name"); names(r.Users) != "olya,sasha" {
		t.Fatalf("sasha friends: %s", names(r.Users))
	}
}
