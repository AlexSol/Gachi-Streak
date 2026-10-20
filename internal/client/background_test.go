package client

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/AlexSol/gachi-streak/assets"
)

func TestWidgetBackgroundJSON(t *testing.T) {
	var c Config
	if err := json.Unmarshal([]byte(`{"widget_background":"C:\\bg.webm"}`), &c); err != nil || c.WidgetBackground.For("failed") != `C:\bg.webm` {
		t.Fatalf("string form: %+v %v", c.WidgetBackground, err)
	}
	c = DefaultConfig(t.TempDir()) // bundled per-state folders must not survive an explicit object
	if err := json.Unmarshal([]byte(`{"widget_background":{"default":"a","failed":"f"}}`), &c); err != nil {
		t.Fatal(err)
	}
	if c.WidgetBackground.For("failed") != "f" || c.WidgetBackground.For("safe") != "a" || c.WidgetBackground.For("") != "a" {
		t.Fatalf("object form: %+v", c.WidgetBackground)
	}
}

func TestBackgroundPicker(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"a.webp", "b.WEBM", "c.jpg", "skip.gif", "skip.txt"} {
		os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o600)
	}
	if got := ListBackgrounds(dir); len(got) != 3 {
		t.Fatalf("ListBackgrounds = %v", got)
	}
	single := filepath.Join(dir, "c.jpg")
	if got := ListBackgrounds(single); len(got) != 1 || ListBackgrounds(filepath.Join(dir, "skip.gif")) != nil {
		t.Fatalf("single file: %v", got)
	}
	if !IsBackgroundVideo("x.WebM") || IsBackgroundVideo("x.webp") {
		t.Fatal("IsBackgroundVideo")
	}

	var p BackgroundPicker
	b := WidgetBackground{Default: dir, Failed: single}
	first := p.Pick(b, "safe")
	for range 20 {
		if p.Pick(b, "safe") != first {
			t.Fatal("same state must keep the picked file")
		}
	}
	if p.Pick(b, "failed") != single {
		t.Fatal("state override")
	}
	if p.Pick(WidgetBackground{}, "safe") != "" {
		t.Fatal("no background configured")
	}
}

func TestMediaServer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bg.webm")
	os.WriteFile(path, []byte("0123456789"), 0o600)
	s, err := StartMediaServer()
	if err != nil {
		t.Fatal(err)
	}
	u := s.URL(path)
	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("Range", "bytes=2-4")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent || string(body) != "234" || resp.Header.Get("Content-Type") != "video/webm" {
		t.Fatalf("range: %d %q %q", resp.StatusCode, body, resp.Header.Get("Content-Type"))
	}
	if resp, err := http.Get(u[:len(u)-10] + "nope"); err != nil || resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unregistered file must 404: %v %v", resp, err)
	}
	if s.URL("") != "" || (*MediaServer)(nil).URL(path) != "" {
		t.Fatal("empty URL")
	}
}

func TestBundledWidgetBackgrounds(t *testing.T) {
	dataDir := t.TempDir()
	def := DefaultConfig(dataDir).WidgetBackground
	for _, st := range BackgroundStates {
		dst := filepath.Join(dataDir, "widget", st)
		if n, err := InstallDefaults(assets.FS, "widget/"+st, dst, BackgroundExts()); err != nil || n == 0 {
			t.Fatalf("%s: installed %d, %v", st, n, err)
		}
		if def.For(st) != dst || len(ListBackgrounds(def.For(st))) == 0 {
			t.Fatalf("%s: default config must point at the unpacked folder, got %q", st, def.For(st))
		}
	}
}

func TestBackgroundLibrary(t *testing.T) {
	dataDir := t.TempDir()
	for _, st := range BackgroundStates {
		InstallDefaults(assets.FS, "widget/"+st, BackgroundDir(dataDir, st), BackgroundExts())
	}
	bundled := BackgroundLibrary(dataDir)
	custom := filepath.Join(t.TempDir(), "mine.webp")
	os.WriteFile(custom, []byte("x"), 0o600)
	got := BackgroundLibrary(dataDir, custom, bundled[0], BackgroundDir(dataDir, "safe"), "")
	if len(bundled) < len(BackgroundStates) || len(got) != len(bundled)+1 || got[len(got)-1] != custom {
		t.Fatalf("library: bundled %v, with extras %v", bundled, got)
	}
}
