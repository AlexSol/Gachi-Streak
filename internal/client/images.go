package client

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var imageExts = map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".bmp": true}

// ListImages returns absolute paths of images in dir (non-recursive), sorted.
func ListImages(dir string) []string {
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !imageExts[strings.ToLower(filepath.Ext(e.Name()))] {
			continue
		}
		p, err := filepath.Abs(filepath.Join(dir, e.Name()))
		if err == nil {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// EnsurePlaceholder generates a loud striped wallpaper for a mode when the
// user has not put any pictures in the folder yet. Returns its path.
func EnsurePlaceholder(dataDir, mode string) (string, error) {
	path := filepath.Join(dataDir, "placeholder_"+mode+".png")
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	a, b := color.RGBA{255, 140, 0, 255}, color.RGBA{20, 20, 20, 255} // warning: orange/black
	if mode == string(ModeFailed) {
		a, b = color.RGBA{200, 0, 0, 255}, color.RGBA{0, 0, 0, 255} // failed: red/black
	}
	const w, h, stripe = 1920, 1080, 120
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := b
			if ((x+y)/stripe)%2 == 0 {
				c = a
			}
			img.SetRGBA(x, y, c)
		}
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return "", err
	}
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return "", err
	}
	return path, f.Close()
}
