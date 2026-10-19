package winapi

import "testing"

func TestSnapRect(t *testing.T) {
	area := Rect{0, 0, 1920, 1040} // work area above a 40px taskbar
	size := func(r Rect) (int32, int32) { return r.Right - r.Left, r.Bottom - r.Top }
	cases := []struct {
		name string
		in   Rect
		want Rect
	}{
		{"near top-right corner", Rect{1590, 12, 1910, 312}, Rect{1600, 0, 1920, 300}},
		{"near bottom-left (taskbar edge)", Rect{15, 725, 335, 1025}, Rect{0, 740, 320, 1040}},
		{"middle of the screen stays", Rect{800, 400, 1120, 700}, Rect{800, 400, 1120, 700}},
		{"slightly outside snaps back in", Rect{-10, 300, 310, 600}, Rect{0, 300, 320, 600}},
		{"just beyond the distance stays", Rect{21, 21, 341, 321}, Rect{21, 21, 341, 321}},
	}
	for _, c := range cases {
		got := SnapRect(c.in, area, 20)
		if got != c.want {
			t.Errorf("%s: got %+v want %+v", c.name, got, c.want)
		}
		if w, h := size(got); w != 320 || h != 300 {
			t.Errorf("%s: size changed to %dx%d", c.name, w, h)
		}
	}
	if got := SnapRect(Rect{5, 5, 325, 305}, area, 0); got.Left != 5 {
		t.Error("dist 0 must disable snapping")
	}
}

// Regression: once snapped, the widget must come off the edge as soon as
// the cursor has moved further than the snap distance.
func TestSnappedWidgetCanBePulledAway(t *testing.T) {
	area := Rect{0, 0, 1920, 1040}
	start := Rect{0, 300, 320, 600} // already stuck to the left edge
	cur0 := Point{100, 320}         // where the user grabbed the header

	pos := start
	for dx := int32(1); dx <= 40; dx++ {
		pos = SnapRect(DragRect(start, cur0, Point{cur0.X + dx, cur0.Y}), area, 20)
		if dx <= 20 && pos.Left != 0 {
			t.Fatalf("dx=%d: should still be stuck, left=%d", dx, pos.Left)
		}
		if dx > 20 && pos.Left != dx {
			t.Fatalf("dx=%d: should follow the cursor, left=%d", dx, pos.Left)
		}
	}
	if pos.Right-pos.Left != 320 {
		t.Fatal("size must not change while dragging")
	}
}
