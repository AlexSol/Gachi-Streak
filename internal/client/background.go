package client

import (
	cryptorand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math/rand/v2"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// WidgetBackground is a file or a folder (a random file is picked) per
// widget state; an empty state falls back to Default.
// In config.json it can also be a plain string, meaning Default.
type WidgetBackground struct {
	Default string `json:"default"`
	Safe    string `json:"safe"`
	Pending string `json:"pending"`
	Warning string `json:"warning"`
	Failed  string `json:"failed"`
}

func (b *WidgetBackground) UnmarshalJSON(data []byte) error {
	var s string
	if json.Unmarshal(data, &s) == nil {
		*b = WidgetBackground{Default: s}
		return nil
	}
	// an object replaces the defaults as a whole: {"default": "x"} must not
	// lose to the bundled per-state folders
	type plain WidgetBackground // no UnmarshalJSON, no recursion
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	*b = WidgetBackground(p)
	return nil
}

// For returns the source configured for a state ("safe", "pending", "warning", "failed").
func (b WidgetBackground) For(state string) string {
	src := map[string]string{"safe": b.Safe, "pending": b.Pending, "warning": b.Warning, "failed": b.Failed}[state]
	if src == "" {
		return b.Default
	}
	return src
}

// bgTypes are the widget background formats WebView2 plays out of the box.
var bgTypes = map[string]string{
	".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".png": "image/png",
	".webp": "image/webp", // static or animated
	".webm": "video/webm",
}

// BackgroundExts are the accepted widget background file kinds.
func BackgroundExts() map[string]bool {
	m := map[string]bool{}
	for ext := range bgTypes {
		m[ext] = true
	}
	return m
}

// BackgroundStates are the widget states that can have their own background.
var BackgroundStates = []string{"safe", "pending", "warning", "failed"}

// IsBackgroundVideo reports whether a background file is a video.
func IsBackgroundVideo(path string) bool {
	return strings.HasPrefix(bgTypes[strings.ToLower(filepath.Ext(path))], "video/")
}

// ListBackgrounds returns usable files: src itself, or the files in folder src, sorted.
func ListBackgrounds(src string) []string {
	if src == "" {
		return nil
	}
	st, err := os.Stat(src)
	if err != nil {
		return nil
	}
	var names []string
	if st.IsDir() {
		entries, _ := os.ReadDir(src)
		for _, e := range entries {
			if !e.IsDir() {
				names = append(names, filepath.Join(src, e.Name()))
			}
		}
	} else {
		names = []string{src}
	}
	var out []string
	for _, n := range names {
		if _, ok := bgTypes[strings.ToLower(filepath.Ext(n))]; !ok {
			continue
		}
		if p, err := filepath.Abs(n); err == nil {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// BackgroundDir is where the bundled backgrounds of a state are unpacked
// (and where the user can drop more).
func BackgroundDir(dataDir, state string) string {
	return filepath.Join(dataDir, "widget", state)
}

// BackgroundLibrary lists every file the settings gallery offers: the
// per-state folders plus extra files (already chosen ones outside them).
func BackgroundLibrary(dataDir string, extra ...string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(p string) {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	for _, st := range BackgroundStates {
		for _, p := range ListBackgrounds(BackgroundDir(dataDir, st)) {
			add(p)
		}
	}
	for _, e := range extra {
		if st, err := os.Stat(e); err == nil && !st.IsDir() {
			for _, p := range ListBackgrounds(e) {
				add(p)
			}
		}
	}
	return out
}

// BackgroundPicker keeps the picked file while the state stays the same
// and picks a new random one when the state (or its source) changes.
type BackgroundPicker struct {
	state, src, path string
}

// Pick returns the background file for state, or "" if there is none.
func (p *BackgroundPicker) Pick(b WidgetBackground, state string) string {
	src := b.For(state)
	if state == p.state && src == p.src && p.path != "" {
		if _, err := os.Stat(p.path); err == nil {
			return p.path
		}
	}
	p.state, p.src, p.path = state, src, ""
	if files := ListBackgrounds(src); len(files) > 0 {
		p.path = files[rand.IntN(len(files))]
	}
	return p.path
}

// MediaServer serves the chosen background files to the widget page over
// loopback: a page loaded from a string may not read file:// URLs.
// Only files handed to URL are served, under a random per-run token.
type MediaServer struct {
	base  string // http://127.0.0.1:port/token/
	mu    sync.Mutex
	files map[string]string // id → path
}

// StartMediaServer listens on a random loopback port.
func StartMediaServer() (*MediaServer, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	tok := make([]byte, 16)
	_, _ = cryptorand.Read(tok)
	prefix := "/" + hex.EncodeToString(tok) + "/"
	s := &MediaServer{base: "http://" + ln.Addr().String() + prefix, files: map[string]string{}}
	mux := http.NewServeMux()
	mux.HandleFunc(prefix, func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		path, ok := s.files[strings.TrimPrefix(r.URL.Path, prefix)]
		s.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		f, err := os.Open(path)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		st, err := f.Stat()
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", bgTypes[strings.ToLower(filepath.Ext(path))])
		http.ServeContent(w, r, "", st.ModTime(), f) // handles Range for video seeking
	})
	go func() { _ = http.Serve(ln, mux) }()
	return s, nil
}

// URL returns a URL for a file (registering it), or "" for "".
// The URL changes when the file is edited, so the page reloads it.
func (s *MediaServer) URL(path string) string {
	if s == nil || path == "" {
		return ""
	}
	h := fnv.New64a()
	h.Write([]byte(path))
	if st, err := os.Stat(path); err == nil {
		fmt.Fprint(h, st.Size(), st.ModTime().UnixNano())
	}
	id := fmt.Sprintf("%016x%s", h.Sum64(), strings.ToLower(filepath.Ext(path)))
	s.mu.Lock()
	s.files[id] = path
	s.mu.Unlock()
	return s.base + id
}
