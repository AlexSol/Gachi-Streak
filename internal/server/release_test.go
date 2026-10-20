package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AlexSol/gachi-streak/internal/api"
)

func TestClientVersion(t *testing.T) {
	f := setup(t)
	var v api.ClientVersion
	if code := f.do(t, "GET", "/client/version", "", "", &v); code != 200 || v.Latest != "" {
		t.Fatalf("without a checker: %d %+v", code, v)
	}

	hits := 0
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.URL.Path != "/repos/o/r/releases/latest" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("If-None-Match") == `"e1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"e1"`)
		w.Write([]byte(`{"tag_name":"v0.3.0","html_url":"https://gh/rel","assets":[
			{"name":"checksums.txt","browser_download_url":"https://gh/sums"},
			{"name":"gachi-client.exe","browser_download_url":"https://gh/exe","digest":"sha256:abc"}]}`))
	}))
	defer gh.Close()

	f.srv.Releases = &ReleaseChecker{GH: &GitHubChecker{S: f.srv, BaseURL: gh.URL}, Repo: "o/r", Min: "0.2.0"}
	for range 2 { // the second pass is a 304 and keeps the release
		if err := f.srv.Releases.Refresh(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if hits != 2 {
		t.Fatalf("GitHub hits = %d", hits)
	}
	f.do(t, "GET", "/client/version", "", "", &v)
	want := api.ClientVersion{Latest: "0.3.0", Min: "0.2.0", URL: "https://gh/rel", Download: "https://gh/exe", SHA256: "abc"}
	if v != want {
		t.Fatalf("got %+v, want %+v", v, want)
	}

	// a repo without releases is not an error
	f.srv.Releases.Repo = "o/none"
	if err := f.srv.Releases.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
}
