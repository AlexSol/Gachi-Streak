package client

import (
	"path/filepath"
	"testing"
	"time"
)

func TestFriendlyName(t *testing.T) {
	cases := map[string]string{
		`C:\Program Files\JetBrains\GoLand\bin\goland64.exe`: "GoLand",
		"Code.exe":     "VS Code",
		"LockApp.exe":  "",
		"SomeTool.exe": "SomeTool",
		"":             "",
	}
	for in, want := range cases {
		if got := FriendlyName(in); got != want {
			t.Errorf("FriendlyName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTrackerCountsForegroundTime(t *testing.T) {
	tr := NewTracker(filepath.Join(t.TempDir(), "activity.json"), []string{"Telegram.exe"})
	tr.SetLocation(time.UTC)
	t0 := time.Date(2026, 10, 19, 10, 0, 0, 0, time.UTC)

	at := func(sec int) time.Time { return t0.Add(time.Duration(sec) * time.Second) }
	tr.Sample(at(0), "goland64.exe", false)
	tr.Sample(at(5), "goland64.exe", false)
	tr.Sample(at(10), "chrome.exe", false)   // 10s GoLand
	tr.Sample(at(15), "chrome.exe", false)   // 5s Chrome
	tr.Sample(at(20), "Telegram.exe", false) // +5s Chrome, then ignored app
	tr.Sample(at(25), "Telegram.exe", false) // ignored: not counted
	tr.Sample(at(30), "goland64.exe", true)  // idle
	tr.Sample(at(600), "goland64.exe", true) // still idle, long gap
	tr.Sample(at(605), "goland64.exe", false)
	tr.Sample(at(3605), "goland64.exe", false) // huge gap (sleep) → capped at 15s

	r := tr.Report()
	if r.Usage["GoLand"] != 10+15 || r.Usage["Chrome"] != 10 {
		t.Fatalf("usage %+v", r.Usage)
	}
	if _, ok := r.Usage["Telegram"]; ok {
		t.Fatal("ignored app must not be tracked")
	}
	if r.App != "GoLand" || r.Idle || r.Day != "2026-10-19" {
		t.Fatalf("report %+v", r)
	}
	if top := tr.Top(1); len(top) != 1 || top[0].App != "GoLand" {
		t.Fatalf("top %+v", top)
	}

	// persisted and restored
	if err := tr.Save(); err != nil {
		t.Fatal(err)
	}
	tr2 := NewTracker(tr.path, nil)
	if tr2.Apps["GoLand"] != 25 {
		t.Fatalf("restore %+v", tr2.Apps)
	}

	// new day resets totals
	tr2.SetLocation(time.UTC)
	tr2.Sample(t0.Add(24*time.Hour), "code.exe", false)
	if len(tr2.Report().Usage) != 0 || tr2.Report().Day != "2026-10-20" {
		t.Fatalf("new day: %+v", tr2.Report())
	}
}
