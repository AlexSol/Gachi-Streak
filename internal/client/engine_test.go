package client

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/AlexSol/gachi-streak/internal/api"
)

type fakeDesk struct {
	backups, restores int
	wallpaper         string
	sounds            []string
}

func (f *fakeDesk) BackupWallpaper() error      { f.backups++; return nil }
func (f *fakeDesk) RestoreWallpaper() error     { f.restores++; f.wallpaper = "original"; return nil }
func (f *fakeDesk) SetWallpaper(p string) error { f.wallpaper = p; return nil }
func (f *fakeDesk) PlaySound(s string)          { f.sounds = append(f.sounds, s) }

func newTestEngine(t *testing.T) (*Engine, *fakeDesk) {
	dir := t.TempDir()
	cfg := DefaultConfig(dir)
	cfg.RotateMinutes = 30
	os.MkdirAll(cfg.FailedImagesDir, 0o755)
	os.WriteFile(filepath.Join(cfg.FailedImagesDir, "a.jpg"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(cfg.FailedImagesDir, "b.png"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(cfg.FailedImagesDir, "notes.txt"), []byte("x"), 0o644)
	d := &fakeDesk{wallpaper: "original"}
	return NewEngine(cfg, d, dir), d
}

func TestEngineLifecycle(t *testing.T) {
	e, d := newTestEngine(t)
	t0 := time.Date(2026, 10, 19, 19, 0, 0, 0, time.UTC)

	// pending → nothing happens
	if ch, _ := e.Apply(api.UserStatus{State: "pending"}, t0); ch {
		t.Fatal("pending must not change anything")
	}

	// warning → backup + placeholder (warning dir empty) + warning sound
	e.Apply(api.UserStatus{State: "warning"}, t0)
	if d.backups != 1 || filepath.Base(d.wallpaper) != "placeholder_warning.png" || len(d.sounds) != 1 {
		t.Fatalf("warning: %+v", d)
	}
	if _, err := os.Stat(d.wallpaper); err != nil {
		t.Fatal("placeholder not generated")
	}

	// still warning 10 min later → no change; 31 min later → rotate, no new backup, no sound
	if ch, _ := e.Apply(api.UserStatus{State: "warning"}, t0.Add(10*time.Minute)); ch {
		t.Fatal("no rotation yet")
	}
	e.Apply(api.UserStatus{State: "warning"}, t0.Add(31*time.Minute))
	if d.backups != 1 || len(d.sounds) != 1 {
		t.Fatalf("rotation must not backup or beep: %+v", d)
	}

	// failed with new failure id → failed image + failed sound
	fail := &api.Failure{ID: 7, Day: "2026-10-19"}
	t1 := t0.Add(5 * time.Hour)
	e.Apply(api.UserStatus{State: "failed", LastFailure: fail}, t1)
	if ext := filepath.Ext(d.wallpaper); ext != ".jpg" && ext != ".png" {
		t.Fatalf("failed wallpaper: %s", d.wallpaper)
	}
	if d.sounds[len(d.sounds)-1] != e.Cfg.FailedSound {
		t.Fatalf("sounds %v", d.sounds)
	}
	// same failure again → no second sound
	n := len(d.sounds)
	e.Apply(api.UserStatus{State: "failed", LastFailure: fail}, t1.Add(time.Minute))
	if len(d.sounds) != n {
		t.Fatal("sound replayed for the same failure")
	}

	// state survives a restart
	e2 := NewEngine(e.Cfg, d, e.DataDir)
	if e2.State.Mode != ModeFailed || e2.State.LastFailureID != 7 {
		t.Fatalf("state not persisted: %+v", e2.State)
	}

	// ping → safe → restore
	e2.Apply(api.UserStatus{State: "safe", LastFailure: fail}, t1.Add(2*time.Minute))
	if d.restores != 1 || d.wallpaper != "original" || e2.State.Mode != ModeNone {
		t.Fatalf("restore: %+v %+v", d, e2.State)
	}
}

func TestSnooze(t *testing.T) {
	e, d := newTestEngine(t)
	t0 := time.Date(2026, 10, 19, 19, 0, 0, 0, time.UTC)
	e.Apply(api.UserStatus{State: "warning"}, t0)
	e.Snooze(t0.Add(15 * time.Minute))
	if d.wallpaper != "original" {
		t.Fatal("snooze must restore")
	}
	e.Apply(api.UserStatus{State: "warning"}, t0.Add(5*time.Minute))
	if d.wallpaper != "original" {
		t.Fatal("snoozed engine must not punish")
	}
	e.Apply(api.UserStatus{State: "warning"}, t0.Add(16*time.Minute))
	if d.wallpaper == "original" {
		t.Fatal("punishment must come back after snooze")
	}
}

func TestStateIconIsValidICO(t *testing.T) {
	ico := StateIcon("warning")
	if binary.LittleEndian.Uint16(ico[2:]) != 1 || binary.LittleEndian.Uint16(ico[4:]) != 1 {
		t.Fatal("bad ICONDIR")
	}
	size := binary.LittleEndian.Uint32(ico[14:])
	off := binary.LittleEndian.Uint32(ico[18:])
	if int(off+size) != len(ico) {
		t.Fatalf("entry size/offset mismatch: %d+%d != %d", off, size, len(ico))
	}
	if want := 40 + 32*32*4 + 4*32; int(size) != want {
		t.Fatalf("bmp size %d want %d", size, want)
	}
}

func TestFormatLeft(t *testing.T) {
	if s := FormatLeft(4*time.Hour + 5*time.Minute); s != "4г 05хв" {
		t.Fatal(s)
	}
}
