package server

import (
	"bytes"
	_ "embed"
	"html/template"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

//go:embed web/index.html
var indexHTML string

var indexTmpl = template.Must(template.New("index").Parse(indexHTML))

// supportedLangs are the page languages; the first is the fallback.
var supportedLangs = []string{"en", "uk"}

// handleIndex serves the "under construction" landing page at "/".
// The language comes from Accept-Language (the browser's / OS language);
// the page refines it client-side with navigator.languages.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	// Signed-in users have nothing to do on the invite page: go to the app.
	if s.UI != nil && s.hasSession(r) {
		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, r, "/app/", http.StatusFound)
		return
	}
	lang := PickLang(r.Header.Get("Accept-Language"))
	var buf bytes.Buffer
	if err := indexTmpl.Execute(&buf, struct{ Lang string }{lang}); err != nil {
		s.Log.Error("index", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Language", lang)
	w.Header().Set("Vary", "Accept-Language")
	// The response depends on the session cookie, so it must not be cached.
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Add("Vary", "Cookie")
	w.Write(buf.Bytes())
}

// hasSession reports whether the request carries a live browser session.
func (s *Server) hasSession(r *http.Request) bool {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return false
	}
	_, err = s.Store.UserBySession(r.Context(), c.Value, s.Now())
	return err == nil
}

// PickLang returns the best supported language for an Accept-Language
// header, honoring q-values, e.g. "uk-UA,uk;q=0.9,en;q=0.8" → "uk".
func PickLang(header string) string {
	type pref struct {
		tag string
		q   float64
		pos int
	}
	var prefs []pref
	for i, part := range strings.Split(header, ",") {
		fields := strings.Split(strings.TrimSpace(part), ";")
		tag := strings.ToLower(strings.TrimSpace(fields[0]))
		if tag == "" {
			continue
		}
		q := 1.0
		for _, f := range fields[1:] {
			if v, ok := strings.CutPrefix(strings.TrimSpace(f), "q="); ok {
				if parsed, err := strconv.ParseFloat(v, 64); err == nil {
					q = parsed
				}
			}
		}
		if q <= 0 {
			continue
		}
		prefs = append(prefs, pref{tag, q, i})
	}
	sort.SliceStable(prefs, func(i, j int) bool { return prefs[i].q > prefs[j].q })
	for _, p := range prefs {
		primary, _, _ := strings.Cut(p.tag, "-")
		for _, l := range supportedLangs {
			if primary == l {
				return l
			}
		}
	}
	return supportedLangs[0]
}
