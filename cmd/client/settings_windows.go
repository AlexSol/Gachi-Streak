//go:build windows

package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	webview2 "github.com/jchv/go-webview2"

	"github.com/AlexSol/gachi-streak/internal/client"
	"github.com/AlexSol/gachi-streak/internal/winapi"
)

//go:embed settings.html
var settingsHTML string

// settingsKeys are the config.json keys the settings window may write;
// everything else in the file is left untouched.
var settingsKeys = map[string]bool{
	"server_url": true, "token": true, "poll_seconds": true,
	"show_widget": true, "widget_mode": true, "widget_theme": true, "widget_snap_px": true,
	"widget_character": true, "widget_background": true, "widget_background_dim": true,
	"rotate_minutes": true, "warning_images_dir": true, "failed_images_dir": true,
	"warning_sound": true, "failed_sound": true,
	"local_repos": true, "local_author_emails": true,
	"activity_tracking": true, "share_activity": true, "activity_ignore": true,
	"achievement_notifications": true,
}

// settingsState is what the page gets from gsLoad.
type settingsState struct {
	Config  client.Config     `json:"config"` // widget_background per state, already resolved
	Themes  []settingsOption  `json:"themes"`
	Library []settingsMedia   `json:"library"`
	Folders map[string]string `json:"folders"` // state → its background folder
	CfgPath string            `json:"cfg_path"`
	Version string            `json:"version"`
}

type settingsOption struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type settingsMedia struct {
	Path  string `json:"path"`
	URL   string `json:"url"`
	Video bool   `json:"video"`
}

// Settings is the settings window: a regular WebView2 window over config.json.
type Settings struct {
	a *app

	mu   sync.Mutex
	open bool
	wv   webview2.WebView
	hwnd uintptr
}

// Show opens the window or brings the open one to the front.
func (s *Settings) Show() {
	s.mu.Lock()
	if s.open {
		hwnd := s.hwnd
		s.mu.Unlock()
		if hwnd != 0 {
			winapi.BringToFront(hwnd)
		}
		return
	}
	s.open = true
	s.mu.Unlock()
	go s.run()
}

func (s *Settings) run() {
	// WebView2 and its message loop must stay on one OS thread.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer func() {
		s.mu.Lock()
		s.open, s.wv, s.hwnd = false, nil, 0
		s.mu.Unlock()
	}()

	scale := winapi.SystemScale()
	wv := webview2.NewWithOptions(webview2.WebViewOptions{
		DataPath:  filepath.Join(s.a.dataDir, "webview2"),
		AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title: title + " — налаштування", Width: uint(780 * scale), Height: uint(760 * scale), Center: true,
		},
	})
	if wv == nil {
		log.Println("settings: WebView2 runtime is not available")
		_ = winapi.Open(s.a.cfgPath) // fall back to editing the file
		return
	}
	hwnd := uintptr(wv.Window())
	s.mu.Lock()
	s.wv, s.hwnd = wv, hwnd
	s.mu.Unlock()

	_ = wv.Bind("gsLoad", s.load)
	_ = wv.Bind("gsSave", s.save)
	_ = wv.Bind("gsPickBackground", func() *settingsMedia {
		path := winapi.OpenFileDialog(hwnd, "Фон віджета", "", []winapi.FileFilter{
			{Name: "Фон віджета (webm, webp, jpg, png)", Pattern: "*.webm;*.webp;*.jpg;*.jpeg;*.png"},
		})
		if path == "" || len(client.ListBackgrounds(path)) == 0 {
			return nil
		}
		return s.media(path)
	})
	_ = wv.Bind("gsOpen", func(path string) { _ = winapi.Open(path) })
	_ = wv.Bind("gsClose", func() { wv.Destroy() })
	wv.SetHtml(pageHTML(settingsHTML, s.a.cfg.NormalizedWidgetTheme()))
	winapi.BringToFront(hwnd)
	wv.Run()
}

func (s *Settings) media(path string) *settingsMedia {
	return &settingsMedia{Path: path, URL: s.a.media.URL(path), Video: client.IsBackgroundVideo(path)}
}

func (s *Settings) load() settingsState {
	a := s.a
	cfg := a.cfg
	bg := client.WidgetBackground{}
	folders := map[string]string{}
	var chosen []string
	for _, st := range client.BackgroundStates {
		src := cfg.WidgetBackground.For(st)
		*bgField(&bg, st) = src
		chosen = append(chosen, src)
		folders[st] = client.BackgroundDir(a.dataDir, st)
	}
	cfg.WidgetBackground = bg
	cfg.WidgetMode = cfg.NormalizedWidgetMode()
	cfg.WidgetTheme = cfg.NormalizedWidgetTheme()

	st := settingsState{Config: cfg, Folders: folders, CfgPath: a.cfgPath, Version: client.ReadBuildInfo(version).String()}
	for _, t := range client.WidgetThemes {
		st.Themes = append(st.Themes, settingsOption{ID: t.ID, Title: t.Title})
	}
	for _, p := range client.BackgroundLibrary(a.dataDir, chosen...) {
		st.Library = append(st.Library, *s.media(p))
	}
	return st
}

func bgField(b *client.WidgetBackground, state string) *string {
	switch state {
	case "safe":
		return &b.Safe
	case "pending":
		return &b.Pending
	case "warning":
		return &b.Warning
	default:
		return &b.Failed
	}
}

// save writes the changed keys and applies them. The returned string is a
// warning to show (e.g. the token is still empty), "" when all is fine.
func (s *Settings) save(fields map[string]any) (string, error) {
	a := s.a
	for k := range fields {
		if !settingsKeys[k] {
			return "", fmt.Errorf("невідоме налаштування %q", k)
		}
	}
	// type-check by decoding onto the current config
	cfg := a.cfg
	b, _ := json.Marshal(fields)
	if err := json.Unmarshal(b, &cfg); err != nil {
		return "", fmt.Errorf("неправильне значення: %w", err)
	}
	if err := client.SetConfigFields(a.cfgPath, fields); err != nil {
		return "", err
	}
	go func() {
		a.reload(true)
		a.locked(func() { a.pushWidget("") })
		a.kick()
	}()
	if err := cfg.Validate(); err != nil {
		return "Збережено, але сервер не буде опитуватись: " + strings.TrimSpace(err.Error()), nil
	}
	return "", nil
}
