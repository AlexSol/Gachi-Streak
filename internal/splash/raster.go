package splash

import (
	"image"
	"math"
)

// shape is a circle (A == B) or a capsule (a segment with round ends).
type shape struct {
	ax, ay, bx, by, r float64
}

func circle(x, y, r float64) shape            { return shape{x, y, x, y, r} }
func capsule(ax, ay, bx, by, r float64) shape { return shape{ax, ay, bx, by, r} }

// dist is the signed distance from (x, y) to the shape edge (< 0 inside).
func (s shape) dist(x, y float64) float64 {
	dx, dy := s.bx-s.ax, s.by-s.ay
	px, py := x-s.ax, y-s.ay
	t := 0.0
	if l := dx*dx + dy*dy; l > 0 {
		t = math.Max(0, math.Min(1, (px*dx+py*dy)/l))
	}
	return math.Hypot(px-t*dx, py-t*dy) - s.r
}

// mask is an antialiased coverage buffer, 0…1 per pixel.
type mask struct {
	w, h int
	a    []float32
}

func newMask(w, h int) *mask { return &mask{w: w, h: h, a: make([]float32, w*h)} }

func (m *mask) clear() { clear(m.a) }

// add unions shapes into the mask; grow widens them (negative shrinks),
// (ox, oy) shifts them, k scales them around (cx, cy).
func (m *mask) add(shapes []shape, grow, ox, oy, k, cx, cy float64) {
	for _, s := range shapes {
		s = shape{
			cx + (s.ax-cx)*k + ox, cy + (s.ay-cy)*k + oy,
			cx + (s.bx-cx)*k + ox, cy + (s.by-cy)*k + oy,
			s.r*k + grow,
		}
		if s.r <= 0 {
			continue
		}
		x0 := max(0, int(math.Min(s.ax, s.bx)-s.r-1))
		x1 := min(m.w, int(math.Max(s.ax, s.bx)+s.r+2))
		y0 := max(0, int(math.Min(s.ay, s.by)-s.r-1))
		y1 := min(m.h, int(math.Max(s.ay, s.by)+s.r+2))
		for y := y0; y < y1; y++ {
			row := m.a[y*m.w:]
			for x := x0; x < x1; x++ {
				c := float32(0.5 - s.dist(float64(x)+0.5, float64(y)+0.5))
				if c > row[x] {
					row[x] = min(c, 1)
				}
			}
		}
	}
}

// look is how a substance is painted.
type look struct {
	body, rim, light [3]float32 // 0…1 RGB
	shadow           float32    // drop shadow opacity
}

var (
	milkLook = look{body: rgb(0xf6, 0xf3, 0xea), rim: rgb(0xc4, 0xbe, 0xae), light: rgb(0xff, 0xff, 0xff), shadow: .3}
	snowLook = look{body: rgb(0xf2, 0xf7, 0xfc), rim: rgb(0xa9, 0xbd, 0xd4), light: rgb(0xff, 0xff, 0xff), shadow: .3}
)

func rgb(r, g, b uint8) [3]float32 {
	return [3]float32{float32(r) / 255, float32(g) / 255, float32(b) / 255}
}

// layers are the masks one picture is painted from.
type layers struct {
	outer, inner, light, shadow *mask
}

func newLayers(w, h int) *layers {
	return &layers{newMask(w, h), newMask(w, h), newMask(w, h), newMask(w, h)}
}

func (l *layers) clear() {
	l.outer.clear()
	l.inner.clear()
	l.light.clear()
	l.shadow.clear()
}

// draw renders body shapes (with rim and drop shadow) and highlights.
func (l *layers) draw(body, lights []shape, scale, k, cx, cy float64) {
	rim := 2.2 * scale
	l.shadow.add(body, scale, 2*scale, 4*scale, k, cx, cy)
	l.outer.add(body, 0, 0, 0, k, cx, cy)
	l.inner.add(body, -rim, 0, 0, k, cx, cy)
	l.light.add(lights, 0, 0, 0, k, cx, cy)
}

// paint writes premultiplied RGBA (what UpdateLayeredWindow wants) inside r.
func (l *layers) paint(dst *image.RGBA, lk look, r image.Rectangle) {
	r = r.Intersect(dst.Rect)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			l.paintAt(dst, lk, y*l.outer.w+x)
		}
	}
}

func (l *layers) paintAt(dst *image.RGBA, lk look, i int) {
	o, in, li, sh := l.outer.a[i], l.inner.a[i], l.light.a[i]*.9, l.shadow.a[i]*lk.shadow
	var c [3]float32
	for j := range c {
		c[j] = lk.rim[j] + (lk.body[j]-lk.rim[j])*in
		c[j] += (lk.light[j] - c[j]) * li
	}
	a := o + sh*(1-o) // body over a black shadow
	p := dst.Pix[i*4 : i*4+4 : i*4+4]
	p[0] = uint8(c[0]*o*255 + .5)
	p[1] = uint8(c[1]*o*255 + .5)
	p[2] = uint8(c[2]*o*255 + .5)
	p[3] = uint8(a*255 + .5)
}
