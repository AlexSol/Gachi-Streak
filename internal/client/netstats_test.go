package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNetStats(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/boom" {
			http.Error(w, "nope", http.StatusInternalServerError)
			return
		}
		w.Write([]byte(strings.Repeat("x", 5000)))
	}))
	defer srv.Close()

	start := time.Now()
	st := NewNetStats(start)
	c := &APIClient{BaseURL: srv.URL, HTTP: &http.Client{Transport: st.Transport()}}
	ctx := context.Background()
	for range 3 {
		_ = c.do(ctx, http.MethodGet, "/status?x=1", nil, nil)
	}
	_ = c.do(ctx, http.MethodPost, "/boom", []byte(`{"a":1}`), nil)

	// A server that is gone: a network error.
	dead := httptest.NewServer(http.NotFoundHandler())
	deadURL := dead.URL
	dead.Close()
	bad := &APIClient{BaseURL: deadURL, HTTP: &http.Client{Transport: st.Transport()}}
	if err := bad.do(ctx, http.MethodGet, "/status", nil, nil); err == nil {
		t.Fatal("expected a network error")
	}

	s := st.Snapshot(start.Add(90 * time.Minute))
	if s.Requests != 5 || s.Errors != 2 {
		t.Fatalf("requests %d errors %d", s.Requests, s.Errors)
	}
	if s.Received < 3*5000 || s.Sent == 0 || s.Connections < 1 {
		t.Fatalf("bytes sent %d received %d conns %d", s.Sent, s.Received, s.Connections)
	}
	if s.Endpoints[0].Endpoint != "GET /status" || s.Endpoints[0].Requests != 4 || s.Endpoints[0].Errors != 1 {
		t.Fatalf("endpoints %+v", s.Endpoints)
	}
	if !strings.HasPrefix(s.LastError, "GET /status: ") || strings.Contains(s.LastError, deadURL) {
		t.Fatalf("last error %q", s.LastError)
	}
	if s.Title() != "📡 Мережа: 5 запитів · 2 помилок" {
		t.Fatalf("title %q", s.Title())
	}
	if l := s.Lines(); l[0] != "⏱ Від запуску: 1г 30хв" || l[2] != "❌ Помилок: 2 (40.0%)" || len(l) != 8 {
		t.Fatalf("lines %q", l)
	}
	r := s.Report(start, "https://gachi.example", "v0.1.0 (abc1234)")
	for _, want := range []string{"Gachi Streak v0.1.0 (abc1234) — мережа\r\n", "Сервер: https://gachi.example\r\n", "❌ Помилок: 2 (40.0%)\r\n", "  GET /status — 4 · ❌ 1 · ~"} {
		if !strings.Contains(r, want) {
			t.Fatalf("report misses %q:\n%s", want, r)
		}
	}
}

func TestFmtBytes(t *testing.T) {
	cases := map[int64]string{0: "0 Б", 1023: "1023 Б", 1536: "1.5 КБ", 5 << 20: "5.0 МБ", 3 << 30: "3.00 ГБ"}
	for n, want := range cases {
		if got := FmtBytes(n); got != want {
			t.Errorf("FmtBytes(%d) = %q, want %q", n, got, want)
		}
	}
}
