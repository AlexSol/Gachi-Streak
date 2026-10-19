package server

import (
	"bytes"
	_ "embed"
	"html/template"
	"net/http"
	"strings"
)

//go:embed web/404.html
var notFoundHTML string

var notFoundTmpl = template.Must(template.New("404").Parse(notFoundHTML))

// withNotFound replaces the mux's plain-text 404 for unknown routes:
// browsers get the themed page, API clients get JSON. Real handlers and
// 405 Method Not Allowed responses are passed through untouched.
func (s *Server) withNotFound(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, pattern := mux.Handler(r); pattern != "" {
			mux.ServeHTTP(w, r) // a route matched
			return
		}
		iw := &notFoundInterceptor{ResponseWriter: w}
		mux.ServeHTTP(iw, r)
		if iw.notFound {
			s.writeNotFound(w, r)
		}
	})
}

// notFoundInterceptor swallows the default "404 page not found" body.
type notFoundInterceptor struct {
	http.ResponseWriter
	notFound bool
}

func (w *notFoundInterceptor) WriteHeader(code int) {
	if code == http.StatusNotFound {
		w.notFound = true
		return
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *notFoundInterceptor) Write(b []byte) (int, error) {
	if w.notFound {
		return len(b), nil
	}
	return w.ResponseWriter.Write(b)
}

func wantsHTML(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "text/html")
}

func (s *Server) writeNotFound(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Del("X-Content-Type-Options")
	if r.Method != http.MethodGet && r.Method != http.MethodHead || !wantsHTML(r) {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	path := r.URL.Path
	if r := []rune(path); len(r) > 120 {
		path = string(r[:120]) + "…"
	}
	lang := PickLang(r.Header.Get("Accept-Language"))
	var buf bytes.Buffer
	if err := notFoundTmpl.Execute(&buf, struct{ Lang, Path string }{lang, path}); err != nil {
		s.Log.Error("404 page", "err", err)
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Content-Language", lang)
	h.Set("Vary", "Accept, Accept-Language")
	h.Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusNotFound)
	if r.Method != http.MethodHead {
		w.Write(buf.Bytes())
	}
}
