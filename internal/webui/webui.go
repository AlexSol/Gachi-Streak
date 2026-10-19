// Package webui serves the browser app (Vite + Preact, built from
// /frontend into ./dist) embedded into the server binary.
package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// dist is filled by `npm run build` in /frontend. The .gitkeep file keeps
// the directory present so the Go build works before the first npm build.
//
//go:embed all:dist
var dist embed.FS

// Handler serves the app mounted at prefix (e.g. "/app/"):
// existing files as is (hashed assets cached forever), every other path
// falls back to index.html so client-side routes work on reload.
func Handler(prefix string) http.Handler {
	root, _ := fs.Sub(dist, "dist")
	files := http.FileServer(http.FS(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, strings.TrimSuffix(prefix, "/"))
		p = strings.TrimPrefix(path.Clean("/"+p), "/")

		if p != "" && p != "index.html" {
			if st, err := fs.Stat(root, p); err == nil && !st.IsDir() {
				if strings.HasPrefix(p, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				r2 := r.Clone(r.Context())
				r2.URL.Path = "/" + p
				files.ServeHTTP(w, r2)
				return
			}
			if path.Ext(p) != "" { // a missing asset is a real 404
				http.NotFound(w, r)
				return
			}
		}
		index, err := fs.ReadFile(root, "index.html")
		if err != nil {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("frontend is not built: run `npm ci && npm run build` in /frontend\n"))
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(index)
	})
}
