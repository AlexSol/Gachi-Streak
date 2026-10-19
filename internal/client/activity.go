package client

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/AlexSol/gachi-streak/internal/api"
)

// IdleAfter: no keyboard/mouse input for this long = away, time is not counted.
const IdleAfter = 5 * time.Minute

// maxGap caps the time credited between two samples, so sleep/hibernate
// or a stalled sampler never turns into hours of fake usage.
const maxGap = 15 * time.Second

// knownApps maps executable names (lower case) to friendly names.
var knownApps = map[string]string{
	"code.exe":                    "VS Code",
	"code - insiders.exe":         "VS Code Insiders",
	"cursor.exe":                  "Cursor",
	"goland64.exe":                "GoLand",
	"goland.exe":                  "GoLand",
	"idea64.exe":                  "IntelliJ IDEA",
	"clion64.exe":                 "CLion",
	"pycharm64.exe":               "PyCharm",
	"rider64.exe":                 "Rider",
	"datagrip64.exe":              "DataGrip",
	"zed.exe":                     "Zed",
	"sublime_text.exe":            "Sublime Text",
	"nvim-qt.exe":                 "Neovim",
	"devenv.exe":                  "Visual Studio",
	"qtcreator.exe":               "Qt Creator",
	"windowsterminal.exe":         "Windows Terminal",
	"wt.exe":                      "Windows Terminal",
	"powershell.exe":              "PowerShell",
	"pwsh.exe":                    "PowerShell",
	"cmd.exe":                     "Command Prompt",
	"wezterm-gui.exe":             "WezTerm",
	"alacritty.exe":               "Alacritty",
	"chrome.exe":                  "Chrome",
	"msedge.exe":                  "Edge",
	"firefox.exe":                 "Firefox",
	"brave.exe":                   "Brave",
	"opera.exe":                   "Opera",
	"arc.exe":                     "Arc",
	"telegram.exe":                "Telegram",
	"discord.exe":                 "Discord",
	"slack.exe":                   "Slack",
	"ms-teams.exe":                "Teams",
	"teams.exe":                   "Teams",
	"zoom.exe":                    "Zoom",
	"spotify.exe":                 "Spotify",
	"steam.exe":                   "Steam",
	"explorer.exe":                "Explorer",
	"notepad.exe":                 "Notepad",
	"notepad++.exe":               "Notepad++",
	"obsidian.exe":                "Obsidian",
	"notion.exe":                  "Notion",
	"postman.exe":                 "Postman",
	"dbeaver.exe":                 "DBeaver",
	"docker desktop.exe":          "Docker Desktop",
	"winword.exe":                 "Word",
	"excel.exe":                   "Excel",
	"outlook.exe":                 "Outlook",
	"olk.exe":                     "Outlook",
	"vlc.exe":                     "VLC",
	"claude.exe":                  "Claude",
	"lockapp.exe":                 "", // lock screen — never counted
	"searchhost.exe":              "",
	"shellexperiencehost.exe":     "",
	"startmenuexperiencehost.exe": "",
	"gachi-client.exe":            "",
}

// FriendlyName turns an executable name into what people see.
// Returns "" for system surfaces that should not be counted.
func FriendlyName(exe string) string {
	base := exeBase(exe)
	if base == "" {
		return ""
	}
	if name, ok := knownApps[strings.ToLower(base)]; ok {
		return name
	}
	name := base
	if i := strings.LastIndex(name, "."); i > 0 {
		name = name[:i]
	}
	if r := []rune(name); len(r) > 40 {
		name = string(r[:40])
	}
	return name
}

// exeBase returns the file name of a path with either \ or / separators.
func exeBase(p string) string {
	p = strings.TrimSpace(p)
	if i := strings.LastIndexAny(p, `\/`); i >= 0 {
		p = p[i+1:]
	}
	return p
}

// Tracker aggregates foreground-app time per calendar day.
// Only executable names are kept — never window titles.
type Tracker struct {
	mu      sync.Mutex
	path    string
	ignore  map[string]bool // lower-case exe names never tracked
	loc     *time.Location
	Day     string           `json:"day"`
	Apps    map[string]int64 `json:"apps"` // friendly name → seconds today
	current string
	since   time.Time
	idle    bool
	last    time.Time
}

// NewTracker loads today's totals from path (if any).
func NewTracker(path string, ignore []string) *Tracker {
	t := &Tracker{path: path, ignore: map[string]bool{}, loc: time.Local, Apps: map[string]int64{}}
	for _, e := range ignore {
		t.ignore[strings.ToLower(strings.TrimSpace(e))] = true
	}
	if b, err := os.ReadFile(path); err == nil {
		var saved struct {
			Day  string           `json:"day"`
			Apps map[string]int64 `json:"apps"`
		}
		if json.Unmarshal(b, &saved) == nil && saved.Apps != nil {
			t.Day, t.Apps = saved.Day, saved.Apps
		}
	}
	return t
}

// SetLocation sets the user's time zone (from the server) for day boundaries.
func (t *Tracker) SetLocation(loc *time.Location) {
	if loc == nil {
		return
	}
	t.mu.Lock()
	t.loc = loc
	t.mu.Unlock()
}

// SetIgnore replaces the ignore list (config reload).
func (t *Tracker) SetIgnore(ignore []string) {
	m := map[string]bool{}
	for _, e := range ignore {
		m[strings.ToLower(strings.TrimSpace(e))] = true
	}
	t.mu.Lock()
	t.ignore = m
	t.mu.Unlock()
}

// Sample records what is in the foreground right now.
func (t *Tracker) Sample(now time.Time, exe string, idle bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	day := now.In(t.loc).Format("2006-01-02")
	if day != t.Day {
		t.Day, t.Apps = day, map[string]int64{}
	}

	app := ""
	if !t.ignore[strings.ToLower(exeBase(exe))] {
		app = FriendlyName(exe)
	}
	if idle {
		app = ""
	}

	// credit the elapsed time to what was in front since the last sample
	if !t.last.IsZero() && t.current != "" && !t.idle {
		gap := now.Sub(t.last)
		if gap > maxGap {
			gap = maxGap
		}
		if gap > 0 {
			t.Apps[t.current] += int64(gap.Round(time.Second) / time.Second)
		}
	}
	if app != t.current {
		t.current, t.since = app, now
	}
	t.idle = idle
	t.last = now
}

// Report builds what is sent to the server.
func (t *Tracker) Report() api.PresenceRequest {
	t.mu.Lock()
	defer t.mu.Unlock()
	apps := make(map[string]int64, len(t.Apps))
	for k, v := range t.Apps {
		if v > 0 {
			apps[k] = v
		}
	}
	r := api.PresenceRequest{App: t.current, Idle: t.idle, Day: t.Day, Usage: apps}
	if t.current != "" {
		r.Since = t.since.UTC()
	}
	return r
}

// Top returns today's apps sorted by time, at most n.
func (t *Tracker) Top(n int) []api.AppTime {
	t.mu.Lock()
	defer t.mu.Unlock()
	return TopApps(t.Apps, n)
}

// TopApps sorts a usage map by time (desc), then name.
func TopApps(m map[string]int64, n int) []api.AppTime {
	out := make([]api.AppTime, 0, len(m))
	for k, v := range m {
		if v > 0 {
			out = append(out, api.AppTime{App: k, Seconds: v})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Seconds != out[j].Seconds {
			return out[i].Seconds > out[j].Seconds
		}
		return out[i].App < out[j].App
	})
	if n > 0 && len(out) > n {
		out = out[:n]
	}
	return out
}

// Save persists today's totals so a restart doesn't lose them.
func (t *Tracker) Save() error {
	t.mu.Lock()
	b, _ := json.MarshalIndent(struct {
		Day  string           `json:"day"`
		Apps map[string]int64 `json:"apps"`
	}{t.Day, t.Apps}, "", "  ")
	t.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(t.path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(t.path, b, 0o600)
}
