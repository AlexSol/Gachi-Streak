package client

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// NetStats counts the agent's traffic to the server since start: requests,
// errors and latency per endpoint (HTTP level) and bytes on the wire
// (TCP level, so TLS and headers are included). Safe for concurrent use.
type NetStats struct {
	Started time.Time

	sent, received atomic.Int64
	conns          atomic.Int64

	mu        sync.Mutex
	endpoints map[string]*EndpointStats
	lastErr   string
	lastErrAt time.Time

	once      sync.Once
	transport http.RoundTripper
}

// EndpointStats is the counters of one "METHOD /path".
type EndpointStats struct {
	Endpoint string
	Requests int
	Errors   int // network errors and HTTP status >= 400
	Total    time.Duration
	Max      time.Duration
}

// Avg returns the mean latency.
func (e EndpointStats) Avg() time.Duration {
	if e.Requests == 0 {
		return 0
	}
	return e.Total / time.Duration(e.Requests)
}

// NewNetStats starts counting now.
func NewNetStats(now time.Time) *NetStats {
	return &NetStats{Started: now, endpoints: map[string]*EndpointStats{}}
}

// Transport returns the counting transport (one per NetStats, so
// connections are reused across config reloads).
func (s *NetStats) Transport() http.RoundTripper {
	s.once.Do(func() {
		base := http.DefaultTransport.(*http.Transport).Clone()
		dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
		base.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			c, err := dialer.DialContext(ctx, network, addr)
			if err != nil {
				return nil, err
			}
			s.conns.Add(1)
			return &countingConn{Conn: c, s: s}, nil
		}
		s.transport = &countingRT{next: base, s: s}
	})
	return s.transport
}

// Snapshot is a copy of the counters at one moment.
type Snapshot struct {
	Uptime      time.Duration
	Requests    int
	Errors      int
	Sent        int64
	Received    int64
	Connections int64
	Avg, Max    time.Duration
	LastError   string
	LastErrorAt time.Time
	Endpoints   []EndpointStats // most requested first
}

// Snapshot copies the counters.
func (s *NetStats) Snapshot(now time.Time) Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap := Snapshot{
		Uptime:      now.Sub(s.Started),
		Sent:        s.sent.Load(),
		Received:    s.received.Load(),
		Connections: s.conns.Load(),
		LastError:   s.lastErr,
		LastErrorAt: s.lastErrAt,
	}
	var total time.Duration
	for _, e := range s.endpoints {
		snap.Endpoints = append(snap.Endpoints, *e)
		snap.Requests += e.Requests
		snap.Errors += e.Errors
		total += e.Total
		snap.Max = max(snap.Max, e.Max)
	}
	if snap.Requests > 0 {
		snap.Avg = total / time.Duration(snap.Requests)
	}
	sort.Slice(snap.Endpoints, func(i, j int) bool {
		a, b := snap.Endpoints[i], snap.Endpoints[j]
		if a.Requests != b.Requests {
			return a.Requests > b.Requests
		}
		return a.Endpoint < b.Endpoint
	})
	return snap
}

func (s *NetStats) record(endpoint string, d time.Duration, failure string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.endpoints[endpoint]
	if e == nil {
		e = &EndpointStats{Endpoint: endpoint}
		s.endpoints[endpoint] = e
	}
	e.Requests++
	e.Total += d
	e.Max = max(e.Max, d)
	if failure != "" {
		e.Errors++
		s.lastErr = endpoint + ": " + failure
		s.lastErrAt = time.Now()
	}
}

type countingRT struct {
	next http.RoundTripper
	s    *NetStats
}

func (rt *countingRT) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()
	resp, err := rt.next.RoundTrip(req)
	endpoint := req.Method + " " + req.URL.Path
	switch {
	case err != nil:
		rt.s.record(endpoint, time.Since(start), shortErr(err))
	case resp.StatusCode >= 400:
		rt.s.record(endpoint, time.Since(start), resp.Status)
	default:
		rt.s.record(endpoint, time.Since(start), "")
	}
	return resp, err
}

// shortErr drops the URL that net/http errors repeat.
func shortErr(err error) string {
	msg := err.Error()
	if i := strings.LastIndex(msg, ": "); i >= 0 {
		msg = msg[i+2:]
	}
	return msg
}

type countingConn struct {
	net.Conn
	s *NetStats
}

func (c *countingConn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	c.s.received.Add(int64(n))
	return n, err
}

func (c *countingConn) Write(b []byte) (int, error) {
	n, err := c.Conn.Write(b)
	c.s.sent.Add(int64(n))
	return n, err
}

// ---------- formatting for the tray ----------

// FmtBytes formats a byte count as Б / КБ / МБ / ГБ.
func FmtBytes(n int64) string {
	const k = 1024
	switch {
	case n < k:
		return fmt.Sprintf("%d Б", n)
	case n < k*k:
		return fmt.Sprintf("%.1f КБ", float64(n)/k)
	case n < k*k*k:
		return fmt.Sprintf("%.1f МБ", float64(n)/(k*k))
	default:
		return fmt.Sprintf("%.2f ГБ", float64(n)/(k*k*k))
	}
}

func fmtLatency(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%d мс", d.Milliseconds())
	}
	return fmt.Sprintf("%.1f с", d.Seconds())
}

func fmtUptime(d time.Duration) string {
	d = d.Round(time.Minute)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h >= 24 {
		return fmt.Sprintf("%dд %dг %dхв", h/24, h%24, m)
	}
	if h > 0 {
		return fmt.Sprintf("%dг %dхв", h, m)
	}
	return fmt.Sprintf("%dхв", m)
}

// Title is the one-line summary for the tray item.
func (s Snapshot) Title() string {
	return fmt.Sprintf("📡 Мережа: %d запитів · %d помилок", s.Requests, s.Errors)
}

// Lines are the details for the submenu.
func (s Snapshot) Lines() []string {
	errPct := 0.0
	if s.Requests > 0 {
		errPct = float64(s.Errors) * 100 / float64(s.Requests)
	}
	lines := []string{
		"⏱ Від запуску: " + fmtUptime(s.Uptime),
		fmt.Sprintf("🔁 Запитів: %d", s.Requests),
		fmt.Sprintf("❌ Помилок: %d (%.1f%%)", s.Errors, errPct),
		"⬆️ Відправлено: " + FmtBytes(s.Sent),
		"⬇️ Отримано: " + FmtBytes(s.Received),
		fmt.Sprintf("🔌 TCP-з'єднань: %d", s.Connections),
		"⌛ Відповідь: середня " + fmtLatency(s.Avg) + " · макс " + fmtLatency(s.Max),
	}
	if s.LastError != "" {
		lines = append(lines, "⚠️ Остання помилка "+s.LastErrorAt.Format("15:04")+": "+s.LastError)
	}
	return lines
}

// EndpointLine describes one endpoint.
func (e EndpointStats) Line() string {
	l := fmt.Sprintf("%s — %d", e.Endpoint, e.Requests)
	if e.Errors > 0 {
		l += fmt.Sprintf(" · ❌ %d", e.Errors)
	}
	return l + " · ~" + fmtLatency(e.Avg())
}

// Report is the full plain-text report for the clipboard and the log
// (Windows line endings, so it pastes cleanly into Notepad or a chat).
func (s Snapshot) Report(started time.Time, server, build string) string {
	var b strings.Builder
	b.WriteString("Gachi Streak " + build + " — мережа\r\n")
	b.WriteString("Сервер: " + server + "\r\n")
	b.WriteString("Запуск: " + started.Format("2006-01-02 15:04:05") + "\r\n")
	for _, l := range s.Lines() {
		b.WriteString(l + "\r\n")
	}
	if len(s.Endpoints) > 0 {
		b.WriteString("Ендпоінти:\r\n")
	}
	for _, e := range s.Endpoints {
		b.WriteString("  " + e.Line() + " · макс " + fmtLatency(e.Max) + "\r\n")
	}
	return b.String()
}
