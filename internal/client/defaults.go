package client

import (
	"io/fs"
	"math/rand"
	"os"
	"path"
	"path/filepath"
	"strings"
)

var soundExts = map[string]bool{".wav": true, ".mp3": true}

// InstallDefaults copies files from src (a directory inside fsys) into dst,
// but only if dst has no files of the accepted kind yet — user's own
// pictures/sounds are never overwritten or mixed with defaults.
// Returns how many files were copied.
func InstallDefaults(fsys fs.FS, src, dst string, exts map[string]bool) (int, error) {
	if hasFiles(dst, exts) {
		return 0, nil
	}
	entries, err := fs.ReadDir(fsys, src)
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return 0, err
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() || !exts[strings.ToLower(path.Ext(e.Name()))] {
			continue
		}
		b, err := fs.ReadFile(fsys, path.Join(src, e.Name()))
		if err != nil {
			return n, err
		}
		if err := os.WriteFile(filepath.Join(dst, e.Name()), b, 0o644); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// ImageExts and SoundExts are the accepted file kinds.
func ImageExts() map[string]bool { return imageExts }
func SoundExts() map[string]bool { return soundExts }

func hasFiles(dir string, exts map[string]bool) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && exts[strings.ToLower(filepath.Ext(e.Name()))] {
			return true
		}
	}
	return false
}

// ResolveSound turns a sound spec into something PlaySound understands:
// a folder → a random .wav/.mp3 from it; a file or a system alias → as is.
func ResolveSound(spec string, rnd *rand.Rand) string {
	spec = strings.TrimSpace(spec)
	st, err := os.Stat(spec)
	if err != nil || !st.IsDir() {
		return spec
	}
	entries, err := os.ReadDir(spec)
	if err != nil {
		return ""
	}
	var wavs []string
	for _, e := range entries {
		if !e.IsDir() && soundExts[strings.ToLower(filepath.Ext(e.Name()))] {
			wavs = append(wavs, filepath.Join(spec, e.Name()))
		}
	}
	if len(wavs) == 0 {
		return ""
	}
	return wavs[rnd.Intn(len(wavs))]
}
