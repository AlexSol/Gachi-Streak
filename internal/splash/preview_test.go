package splash

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestPreview renders an attack at a few moments over a fake desktop, to
// look at the effect without Windows: SPLASH_PREVIEW=/some/dir go test -run Preview
func TestPreview(t *testing.T) {
	dir := os.Getenv("SPLASH_PREVIEW")
	if dir == "" {
		t.Skip("set SPLASH_PREVIEW=<dir> to render preview PNGs")
	}
	area := image.Rect(0, 0, 1280, 720)
	for _, kind := range []Kind{Snow, Milk} {
		s := NewScene(kind, area, image.Pt(1180, 640), 1, 11)
		live := map[int]Sprite{}
		var shots []time.Duration
		for _, sec := range []float64{.45, 1.2, 2.5, 4.5} {
			shots = append(shots, time.Duration(sec*float64(time.Second)))
		}
		for at, i := time.Duration(0), 0; i < len(shots); at += 16 * time.Millisecond {
			sprites, _ := s.Step(at)
			for _, sp := range sprites {
				prev := live[sp.ID]
				if sp.Img == nil {
					sp.Img = prev.Img
				} else { // the scene reuses its buffers, keep a copy like a window would
					sp.Img = &image.RGBA{Pix: append([]uint8{}, sp.Img.Pix...), Stride: sp.Img.Stride, Rect: sp.Img.Rect}
				}
				if sp.Gone {
					delete(live, sp.ID)
				} else {
					live[sp.ID] = sp
				}
			}
			if at < shots[i] {
				continue
			}
			desk := image.NewRGBA(area)
			draw.Draw(desk, image.Rect(0, 0, 640, 720), image.NewUniform(color.RGBA{250, 250, 250, 255}), image.Point{}, draw.Src)
			draw.Draw(desk, image.Rect(640, 0, 1280, 720), image.NewUniform(color.RGBA{32, 34, 40, 255}), image.Point{}, draw.Src)
			for _, sp := range live {
				r := sp.Img.Rect.Add(image.Pt(sp.X, sp.Y))
				m := image.NewUniform(color.Alpha{sp.Alpha})
				draw.DrawMask(desk, r, sp.Img, image.Point{}, m, image.Point{}, draw.Over)
			}
			f, err := os.Create(filepath.Join(dir, string(kind)+"-"+shots[i].String()+".png"))
			if err != nil {
				t.Fatal(err)
			}
			_ = png.Encode(f, desk)
			f.Close()
			i++
		}
	}
}
