package client

import (
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexSol/gachi-streak/assets"
)

func TestInstallDefaultsAndResolveSound(t *testing.T) {
	dir := t.TempDir()
	img := filepath.Join(dir, "images", "failed")
	n, err := InstallDefaults(assets.FS, "images/failed", img, ImageExts())
	if err != nil || n < 4 {
		t.Fatalf("installed %d images: %v", n, err)
	}
	if len(ListImages(img)) != n {
		t.Fatal("installed images are not listed")
	}
	// second run must not touch a folder that already has pictures
	if n2, _ := InstallDefaults(assets.FS, "images/failed", img, ImageExts()); n2 != 0 {
		t.Fatalf("re-install copied %d files", n2)
	}
	// user's own folder with one picture stays untouched
	own := filepath.Join(dir, "own")
	os.MkdirAll(own, 0o755)
	os.WriteFile(filepath.Join(own, "mine.png"), []byte("x"), 0o644)
	if n3, _ := InstallDefaults(assets.FS, "images/failed", own, ImageExts()); n3 != 0 {
		t.Fatal("must not mix defaults into user's folder")
	}

	snd := filepath.Join(dir, "sounds", "warning")
	if n, err := InstallDefaults(assets.FS, "sounds/warning", snd, SoundExts()); err != nil || n < 3 {
		t.Fatalf("installed %d sounds: %v", n, err)
	}
	rnd := rand.New(rand.NewSource(1))
	if s := ResolveSound(snd, rnd); !strings.HasSuffix(s, ".wav") {
		t.Fatalf("folder must resolve to a wav, got %q", s)
	}
	if s := ResolveSound("SystemHand", rnd); s != "SystemHand" {
		t.Fatalf("alias must pass through, got %q", s)
	}
}
