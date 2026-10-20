package splash

import (
	"image"
	"testing"
	"time"
)

func TestSceneLifecycle(t *testing.T) {
	area := image.Rect(0, 0, 1920, 1040)
	for _, kind := range []Kind{Snow, Milk} {
		s := NewScene(kind, area, image.Pt(1700, 900), 1.5, 42)
		if d := s.Duration(); d < 3*time.Second || d > 9*time.Second {
			t.Fatalf("%s: duration %v, want a few seconds", kind, d)
		}
		seen, gone := map[int]bool{}, map[int]bool{}
		var last time.Duration
		for at := time.Duration(0); ; at += 16 * time.Millisecond {
			sprites, done := s.Step(at)
			for _, sp := range sprites {
				if gone[sp.ID] {
					t.Fatalf("%s: piece %d reported after it was gone", kind, sp.ID)
				}
				if sp.Gone {
					gone[sp.ID] = true
					continue
				}
				if !seen[sp.ID] && sp.Img == nil {
					t.Fatalf("%s: first frame of piece %d has no picture", kind, sp.ID)
				}
				seen[sp.ID] = true
			}
			last = at
			if done {
				break
			}
			if at > time.Minute {
				t.Fatalf("%s: scene never ends", kind)
			}
		}
		if len(seen) == 0 || len(gone) != len(seen) {
			t.Fatalf("%s: %d pieces shown, %d removed (at %v)", kind, len(seen), len(gone), last)
		}
	}
}

func TestSplatsLandInsideTheArea(t *testing.T) {
	area := image.Rect(100, 50, 2020, 1130) // a second monitor
	s := NewScene(Milk, area, image.Pt(1900, 1000), 1, 7)
	for _, p := range s.pieces {
		if !image.Pt(int(p.toX), int(p.toY)).In(area) {
			t.Fatalf("target %v,%v outside %v", p.toX, p.toY, area)
		}
	}
}

func TestPaintIsPremultiplied(t *testing.T) {
	s := NewScene(Snow, image.Rect(0, 0, 800, 600), image.Pt(700, 500), 1, 1)
	p := s.pieces[0]
	sp := p.splat(time.Second, s)
	if sp.Img == nil {
		t.Fatal("splat was not drawn")
	}
	opaque := 0
	for i := 0; i < len(sp.Img.Pix); i += 4 {
		px := sp.Img.Pix[i : i+4]
		if px[0] > px[3] || px[1] > px[3] || px[2] > px[3] {
			t.Fatalf("pixel %v is not premultiplied", px)
		}
		if px[3] == 255 {
			opaque++
		}
	}
	if opaque < 1000 {
		t.Fatalf("splat is almost empty: %d opaque pixels", opaque)
	}
}

func BenchmarkMilkPop(b *testing.B) { // the full redraw while a splat pops in
	s := NewScene(Milk, image.Rect(0, 0, 3840, 2100), image.Pt(3600, 1900), 2, 3)
	p := s.pieces[0]
	for b.Loop() {
		p.drawn = false
		p.splat(time.Second, s)
	}
}

func BenchmarkMilkDrips(b *testing.B) { // a growing-drips frame, 4K at 200%
	s := NewScene(Milk, image.Rect(0, 0, 3840, 2100), image.Pt(3600, 1900), 2, 3)
	p := s.pieces[0]
	p.splat(0, s)
	at := popTime + dripStep
	for b.Loop() { // one 50 ms step of growth per frame, over and over
		at += dripStep
		if at > p.hold {
			at = popTime + 2*dripStep
			for i := range p.drips {
				p.drips[i].drawn = 0
			}
		}
		p.lastDraw = 0
		p.splat(at, s)
	}
}
