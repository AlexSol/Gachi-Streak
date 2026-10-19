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

	// Path to a .wav file, or a Windows sound alias like "SystemHand",
	// "SystemExclamation", "SystemAsterisk". Empty = silence.
	WarningSound string `json:"warning_sound"`
	FailedSound  string `json:"failed_sound"`

	// Local git repositories to watch. A commit authored today (any branch)
	// counts as today's activity — no button press needed. Requires git in PATH.
	LocalRepos []string `json:"local_repos"`
	// Author emails that count. Empty = each repo's own `git config user.email`.
	LocalAuthorEmails []string `json:"local_author_emails"`
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
		WarningSound:      "SystemExclamation",
		FailedSound:       "SystemHand",
		LocalRepos:        []string{},
		LocalAuthorEmails: []string{},
	}
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
