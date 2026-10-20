// Package client contains the OS-independent part of the desktop agent:
// config, API client, punishment engine, images and tray icons.
// The Windows-specific bits live in internal/winapi.
package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Config is stored as JSON in the data dir (%APPDATA%\GachiStreak\config.json).
type Config struct {
	ServerURL string `json:"server_url"` // e.g. http://my-vps:8080
	Token     string `json:"token"`      // from `gachi-server user add`

	PollSeconds int `json:"poll_seconds"` // how often to ask the server, default 60

	// Folders with images (jpg/jpeg/png/bmp). Empty or missing → generated placeholder.
	WarningImagesDir string `json:"warning_images_dir"`
	FailedImagesDir  string `json:"failed_images_dir"`

	// Re-pick a random image every N minutes while punished (0 = never).
	RotateMinutes int `json:"rotate_minutes"`

	// A folder with .wav/.mp3 files (a random one is played), a path to a .wav/.mp3
	// file, or a Windows sound alias like "SystemHand", "SystemExclamation".
	// Empty = silence.
	WarningSound string `json:"warning_sound"`
	FailedSound  string `json:"failed_sound"`

	// Local git repositories to watch. A commit authored today (any branch)
	// counts as today's activity — no button press needed. Requires git in PATH.
	LocalRepos []string `json:"local_repos"`
	// Author emails that count. Empty = each repo's own `git config user.email`.
	LocalAuthorEmails []string `json:"local_author_emails"`

	// Foreground-program statistics (program names only, never window titles).
	ActivityTracking bool `json:"activity_tracking"`
	// Share the current program and today's stats with the partner (like Discord).
	ShareActivity bool `json:"share_activity"`
	// Programs that are never tracked, e.g. ["Telegram.exe", "KeePassXC.exe"].
	ActivityIgnore []string `json:"activity_ignore"`

	// Desktop widget: shown on start, and where it lives —
	// "desktop" (a gadget under all windows) or "overlay" (always on top, like Discord).
	ShowWidget bool   `json:"show_widget"`
	WidgetMode string `json:"widget_mode"`
	// Stick the widget to screen edges when dragged closer than this (px at 100% scale; 0 = off).
	WidgetSnapPx int `json:"widget_snap_px"`
}

// DefaultConfig returns a config with defaults filled in, relative to dataDir.
func DefaultConfig(dataDir string) Config {
	return Config{
		ServerURL:         "http://localhost:8080",
		Token:             "",
		PollSeconds:       60,
		WarningImagesDir:  filepath.Join(dataDir, "images", "warning"),
		FailedImagesDir:   filepath.Join(dataDir, "images", "failed"),
		RotateMinutes:     30,
		WarningSound:      filepath.Join(dataDir, "sounds", "warning"),
		FailedSound:       filepath.Join(dataDir, "sounds", "failed"),
		LocalRepos:        []string{},
		LocalAuthorEmails: []string{},
		ActivityTracking:  true,
		ShareActivity:     true,
		ActivityIgnore:    []string{},
		ShowWidget:        true,
		WidgetMode:        WidgetModeDesktop,
		WidgetSnapPx:      20,
	}
}

// Widget modes.
const (
	WidgetModeDesktop = "desktop"
	WidgetModeOverlay = "overlay"
)

// NormalizedWidgetMode returns a valid widget mode (desktop by default).
func (c Config) NormalizedWidgetMode() string {
	if c.WidgetMode == WidgetModeOverlay {
		return WidgetModeOverlay
	}
	return WidgetModeDesktop
}

// SetConfigFields changes a few top-level keys in config.json, keeping
// everything else (including keys this version doesn't know) as is.
// It refuses to touch a file that is not valid JSON.
func SetConfigFields(path string, fields map[string]any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return fmt.Errorf("config is not valid JSON, not changing it: %w", err)
	}
	for k, v := range fields {
		m[k] = v
	}
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// PollInterval returns the poll interval with a sane minimum.
func (c Config) PollInterval() time.Duration {
	if c.PollSeconds < 10 {
		return 10 * time.Second
	}
	return time.Duration(c.PollSeconds) * time.Second
}

// Validate reports what must be filled in before the agent can work.
func (c Config) Validate() error {
	if c.ServerURL == "" {
		return errors.New("server_url is empty")
	}
	if c.Token == "" {
		return errors.New("token is empty — get it with `gachi-server user add`")
	}
	return nil
}

// ErrConfigCreated means a template config was written and needs editing.
var ErrConfigCreated = errors.New("config template created")

// LoadConfig reads the config; if it does not exist, writes a template
// and returns ErrConfigCreated.
func LoadConfig(path, dataDir string) (Config, error) {
	cfg := DefaultConfig(dataDir)
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return cfg, err
		}
		for _, d := range []string{cfg.WarningImagesDir, cfg.FailedImagesDir} {
			_ = os.MkdirAll(d, 0o755)
		}
		out, _ := json.MarshalIndent(cfg, "", "  ")
		if err := os.WriteFile(path, out, 0o600); err != nil {
			return cfg, err
		}
		return cfg, ErrConfigCreated
	}
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return cfg, fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg, nil
}
