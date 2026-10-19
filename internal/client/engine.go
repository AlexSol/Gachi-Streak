package client

import (
	"encoding/json"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/AlexSol/gachi-streak/internal/api"
)

// Desktop is what the engine needs from the OS. Implemented in internal/winapi.
type Desktop interface {
	// BackupWallpaper remembers the user's current wallpaper (skips if the
	// current one is ours).
	BackupWallpaper() error
	// RestoreWallpaper puts the remembered wallpaper back.
	RestoreWallpaper() error
	// SetWallpaper sets an image file as wallpaper (stretched to fill).
	SetWallpaper(path string) error
	// PlaySound plays a .wav path or a system sound alias, asynchronously.
	PlaySound(spec string)
}

// Mode is what the agent is currently doing to the desktop.
type Mode string

const (
	ModeNone    Mode = "none"
	ModeWarning Mode = "warning"
	ModeFailed  Mode = "failed"
)

// LocalState survives restarts so the agent does not re-backup a punished
// wallpaper or replay a sound for an old failure.
type LocalState struct {
	Mode          Mode      `json:"mode"`
	LastFailureID int64     `json:"last_failure_id"`
	Image         string    `json:"image"`
	SetAt         time.Time `json:"set_at"`
	SnoozeUntil   time.Time `json:"snooze_until"`
}

// Engine maps server state onto desktop punishments.
type Engine struct {
	Cfg       Config
	Desk      Desktop
	DataDir   string
	StatePath string
	State     LocalState
	Rand      *rand.Rand

	// Demo is true while a punishment was forced from the demo menu.
	// Server polls do not touch the desktop until the demo is stopped.
	// Kept in memory only: after a restart the real state wins.
	Demo Mode
}

// NewEngine loads local state from dataDir.
func NewEngine(cfg Config, desk Desktop, dataDir string) *Engine {
	e := &Engine{
		Cfg:       cfg,
		Desk:      desk,
		DataDir:   dataDir,
		StatePath: filepath.Join(dataDir, "state.json"),
		State:     LocalState{Mode: ModeNone},
		Rand:      rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	if b, err := os.ReadFile(e.StatePath); err == nil {
		_ = json.Unmarshal(b, &e.State)
	}
	if e.State.Mode == "" {
		e.State.Mode = ModeNone
	}
	return e
}

func modeFor(state string) Mode {
	switch state {
	case "warning":
		return ModeWarning
	case "failed":
		return ModeFailed
	default: // safe, pending
		return ModeNone
	}
}

// Apply reacts to the caller's own status. Returns whether anything changed.
func (e *Engine) Apply(me api.UserStatus, now time.Time) (bool, error) {
	if e.Demo != "" {
		return false, nil // demo owns the desktop right now
	}
	want := modeFor(me.State)
	changed := false

	// New failure since last time → sound (only while actually failed).
	if me.LastFailure != nil && me.LastFailure.ID > e.State.LastFailureID {
		e.State.LastFailureID = me.LastFailure.ID
		changed = true
		if want == ModeFailed {
			e.Desk.PlaySound(ResolveSound(e.Cfg.FailedSound, e.Rand))
		}
	}

	switch {
	case want == ModeNone && e.State.Mode != ModeNone:
		if err := e.Desk.RestoreWallpaper(); err != nil {
			return changed, err
		}
		e.State.Mode, e.State.Image = ModeNone, ""
		changed = true

	case want != ModeNone && now.Before(e.State.SnoozeUntil):
		// snoozed: leave the desktop alone for now

	case want != ModeNone:
		entering := e.State.Mode != want
		rotate := e.Cfg.RotateMinutes > 0 &&
			now.Sub(e.State.SetAt) >= time.Duration(e.Cfg.RotateMinutes)*time.Minute
		if !entering && !rotate {
			break
		}
		if e.State.Mode == ModeNone {
			// Only back up a "clean" wallpaper, never our own punishment.
			if err := e.Desk.BackupWallpaper(); err != nil {
				return changed, err
			}
		}
		img, err := e.pickImage(want)
		if err != nil {
			return changed, err
		}
		if err := e.Desk.SetWallpaper(img); err != nil {
			return changed, err
		}
		if entering && want == ModeWarning {
			e.Desk.PlaySound(ResolveSound(e.Cfg.WarningSound, e.Rand))
		}
		e.State.Mode, e.State.Image, e.State.SetAt = want, img, now
		changed = true
	}

	if changed {
		return true, e.save()
	}
	return false, nil
}

// DemoShow forces a punishment mode regardless of the server: sets a
// random picture of that mode and plays its sound. Calling it again with
// the same mode shows the next picture.
func (e *Engine) DemoShow(m Mode, now time.Time) error {
	if m != ModeWarning && m != ModeFailed {
		return e.DemoStop()
	}
	if e.State.Mode == ModeNone {
		if err := e.Desk.BackupWallpaper(); err != nil {
			return err
		}
	}
	img, err := e.pickImage(m)
	if err != nil {
		return err
	}
	if err := e.Desk.SetWallpaper(img); err != nil {
		return err
	}
	e.DemoSound(m)
	e.Demo = m
	e.State.Mode, e.State.Image, e.State.SetAt = m, img, now
	return e.save()
}

// DemoSound plays a random sound of a mode without touching the wallpaper.
func (e *Engine) DemoSound(m Mode) {
	spec := e.Cfg.WarningSound
	if m == ModeFailed {
		spec = e.Cfg.FailedSound
	}
	e.Desk.PlaySound(ResolveSound(spec, e.Rand))
}

// DemoStop ends the demo: restores the wallpaper; the next poll applies
// the real server state again.
func (e *Engine) DemoStop() error {
	e.Demo = ""
	if e.State.Mode == ModeNone {
		return nil
	}
	if err := e.Desk.RestoreWallpaper(); err != nil {
		return err
	}
	e.State.Mode, e.State.Image = ModeNone, ""
	return e.save()
}

// Snooze puts the original wallpaper back and leaves the desktop alone
// until `until` (e.g. for a screen-share). The punishment comes back after.
func (e *Engine) Snooze(until time.Time) error {
	if e.State.Mode != ModeNone {
		if err := e.Desk.RestoreWallpaper(); err != nil {
			return err
		}
	}
	e.State.Mode, e.State.Image, e.State.SnoozeUntil = ModeNone, "", until
	return e.save()
}

func (e *Engine) pickImage(m Mode) (string, error) {
	dir := e.Cfg.WarningImagesDir
	if m == ModeFailed {
		dir = e.Cfg.FailedImagesDir
	}
	imgs := ListImages(dir)
	// Prefer a different picture than the current one.
	if len(imgs) > 1 {
		filtered := imgs[:0:0]
		for _, p := range imgs {
			if p != e.State.Image {
				filtered = append(filtered, p)
			}
		}
		imgs = filtered
	}
	if len(imgs) > 0 {
		return imgs[e.Rand.Intn(len(imgs))], nil
	}
	return EnsurePlaceholder(e.DataDir, string(m))
}

func (e *Engine) save() error {
	b, _ := json.MarshalIndent(e.State, "", "  ")
	if err := os.MkdirAll(filepath.Dir(e.StatePath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(e.StatePath, b, 0o600)
}
