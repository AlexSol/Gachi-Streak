package server

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBrowserSessionFlow(t *testing.T) {
	f := setup(t)
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar}
	origin := f.http.URL

	post := func(path, body, org string) *http.Response {
		req, _ := http.NewRequest("POST", f.http.URL+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if org != "" {
			req.Header.Set("Origin", org)
		}
		resp, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp
	}

	// not logged in
	resp, _ := c.Get(f.http.URL + "/auth/me")
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("me before login: %d", resp.StatusCode)
	}

	// wrong token
	if r := post("/auth/login", `{"token":"nope"}`, origin); r.StatusCode != 401 {
		t.Fatalf("bad token: %d", r.StatusCode)
	}
	// cross-origin login rejected
	if r := post("/auth/login", `{"token":"`+f.alex+`"}`, "https://evil.example"); r.StatusCode != 403 {
		t.Fatalf("cross-origin login: %d", r.StatusCode)
	}

	// good login → HttpOnly cookie
	r := post("/auth/login", `{"token":"`+f.alex+`"}`, origin)
	if r.StatusCode != 200 {
		t.Fatalf("login: %d", r.StatusCode)
	}
	var ck *http.Cookie
	for _, x := range r.Cookies() {
		if x.Name == sessionCookie {
			ck = x
		}
	}
	if ck == nil || !ck.HttpOnly || ck.SameSite != http.SameSiteLaxMode {
		t.Fatalf("session cookie: %+v", ck)
	}

	// cookie works for /auth/me and /status
	resp, _ = c.Get(f.http.URL + "/auth/me")
	var me MeResponse
	json.NewDecoder(resp.Body).Decode(&me)
	resp.Body.Close()
	if resp.StatusCode != 200 || me.Name != "alex" {
		t.Fatalf("me: %d %+v", resp.StatusCode, me)
	}
	resp, _ = c.Get(f.http.URL + "/status")
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status via cookie: %d", resp.StatusCode)
	}

	// ping via cookie: same origin ok, foreign origin rejected (CSRF)
	if r := post("/ping", `{}`, "https://evil.example"); r.StatusCode != 403 {
		t.Fatalf("csrf ping: %d", r.StatusCode)
	}
	if r := post("/ping", `{"note":"web"}`, origin); r.StatusCode != 200 {
		t.Fatalf("ping via cookie: %d", r.StatusCode)
	}

	// logout kills the session
	if r := post("/auth/logout", ``, origin); r.StatusCode != 204 {
		t.Fatalf("logout: %d", r.StatusCode)
	}
	resp, _ = c.Get(f.http.URL + "/auth/me")
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("me after logout: %d", resp.StatusCode)
	}
}

func TestLoginRateLimit(t *testing.T) {
	f := setup(t)
	var last int
	for i := 0; i < 12; i++ {
		req, _ := http.NewRequest("POST", f.http.URL+"/auth/login", strings.NewReader(`{"token":"wrong"}`))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		last = resp.StatusCode
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after many failures, got %d", last)
	}
	// even the right token is blocked while limited
	req, _ := http.NewRequest("POST", f.http.URL+"/auth/login", strings.NewReader(`{"token":"`+f.alex+`"}`))
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("limited ip: %d", resp.StatusCode)
	}
}

func TestIndexRedirectsSignedInUsers(t *testing.T) {
	f := setup(t)
	f.srv.UI = http.NotFoundHandler() // mount something at /app/ for the test
	srv := httptest.NewServer(f.srv.Handler())
	defer srv.Close()

	noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	get := func(cookie string) *http.Response {
		req, _ := http.NewRequest("GET", srv.URL+"/", nil)
		if cookie != "" {
			req.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
		}
		resp, err := noRedirect.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp
	}

	// guest → landing page
	if r := get(""); r.StatusCode != 200 {
		t.Fatalf("guest: %d", r.StatusCode)
	}
	// stale / fake cookie → still the landing page
	if r := get("gs_fake"); r.StatusCode != 200 {
		t.Fatalf("fake cookie: %d", r.StatusCode)
	}

	// real session → redirect to the app
	req, _ := http.NewRequest("POST", srv.URL+"/auth/login", strings.NewReader(`{"token":"`+f.alex+`"}`))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	var sid string
	for _, c := range resp.Cookies() {
		if c.Name == sessionCookie {
			sid = c.Value
		}
	}
	r := get(sid)
	if r.StatusCode != http.StatusFound || r.Header.Get("Location") != "/app/" {
		t.Fatalf("signed in: %d %q", r.StatusCode, r.Header.Get("Location"))
	}
}
