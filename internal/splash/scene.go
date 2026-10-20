// Package splash throws snowballs or splashes milk over the whole screen:
// pieces fly out of the widget character, splat, drip and fade away within
// a few seconds. The scene is OS-independent (testable anywhere); Run shows
// it on screen (click-through layered windows on Windows).
package splash

import (
	"image"
	"math"
	"math/rand/v2"
	"time"
)

// Kind is what gets thrown.
type Kind string

const (
	Snow Kind = "snow"
	Milk Kind = "milk"
)

// Valid reports whether k is a known kind.
func (k Kind) Valid() bool { return k == Snow || k == Milk }

// Sprite is one piece on screen at some moment (one window on Windows).
type Sprite struct {
	ID    int
	X, Y  int         // top-left corner, screen pixels
	Img   *image.RGBA // premultiplied; nil = same picture as last time
	Alpha uint8       // opacity of the whole sprite
	Gone  bool        // the piece is finished: remove it
}

// Scene is one attack: a volley of pieces, each flying, splatting and fading.
type Scene struct {
	kind   Kind
	scale  float64
	pieces []*piece
	end    time.Duration
}

type piece struct {
	id                         int
	launch, flight, hold, fade time.Duration
	fromX, fromY, toX, toY     float64
	arc                        float64 // flight curve height
	slide                      float64 // how far a splat slides down while melting

	// flying sprite
	ball, tail []shape // around (0, 0); tail = milk drop's trail
	ballImg    *image.RGBA
	ballLy     *layers
	ballC      float64 // ball canvas center

	// splat sprite: static shapes + growing drips, in canvas coordinates
	body, lights []shape
	drips        []drip
	cx, cy       float64 // splat center in the canvas
	img          *image.RGBA
	ly           *layers
	lastDraw     time.Duration
	drawn, gone  bool
}

type drip struct {
	x, y0, r, length float64
	drawn            float64 // length already in the masks
}

const (
	popTime  = 130 * time.Millisecond // splat grows to full size
	dripStep = 50 * time.Millisecond  // redraw growing drips at ~20 fps
)

// NewScene plans an attack inside area (screen pixels), thrown from `from`.
// scale is the display scale (1 = 96 DPI).
func NewScene(kind Kind, area image.Rectangle, from image.Point, scale float64, seed uint64) *Scene {
	if scale <= 0 {
		scale = 1
	}
	rnd := rand.New(rand.NewPCG(seed, 0x9e3779b97f4a7c15))
	between := func(a, b float64) float64 { return a + rnd.Float64()*(b-a) }
	ms := func(a, b float64) time.Duration { return time.Duration(between(a, b) * float64(time.Millisecond)) }

	s := &Scene{kind: kind, scale: scale}
	n, launch := 5+rnd.IntN(3), time.Duration(0)
	if kind == Snow {
		n = 9 + rnd.IntN(5)
	}
	var placed [][2]float64
	for i := range n {
		p := &piece{id: i + 1, launch: launch, fromX: float64(from.X), fromY: float64(from.Y), fade: 800 * time.Millisecond}
		var R float64
		if kind == Snow {
			R = between(55, 80) * scale
			p.flight, p.hold = ms(420, 620), ms(2600, 3400)
			p.slide = between(6, 14) * scale
			launch += ms(140, 260)
		} else {
			R = between(80, 115) * scale
			p.flight, p.hold = ms(520, 720), ms(3600, 4600)
			launch += ms(280, 440)
		}
		// spread the targets: retry a few times when too close to another one
		inner := image.Rect(area.Min.X+int(R), area.Min.Y+int(R), area.Max.X-int(R), area.Max.Y-int(R))
		if inner.Empty() {
			inner = area
		}
		for try := 0; ; try++ {
			p.toX = between(float64(inner.Min.X), float64(inner.Max.X))
			p.toY = between(float64(inner.Min.Y), float64(inner.Max.Y))
			ok := true
			for _, q := range placed {
				if math.Hypot(q[0]-p.toX, q[1]-p.toY) < 1.6*R {
					ok = false
				}
			}
			if ok || try == 12 {
				break
			}
		}
		placed = append(placed, [2]float64{p.toX, p.toY})
		p.arc = math.Hypot(p.toX-p.fromX, p.toY-p.fromY) * between(.12, .28)

		if kind == Snow {
			p.ball, p.body, p.lights, p.drips = snowShapes(rnd, R, scale)
		} else {
			p.ball, p.body, p.lights, p.drips = milkShapes(rnd, R, scale)
			// a milk drop trails behind, away from where it flies
			dx, dy := p.toX-p.fromX, p.toY-p.fromY
			if l := math.Hypot(dx, dy); l > 0 {
				r := p.ball[0].r
				p.tail = []shape{capsule(0, 0, -dx/l*r*1.6, -dy/l*r*1.6, r*.55)}
			}
		}
		p.layout(scale)
		s.pieces = append(s.pieces, p)
		s.end = max(s.end, p.launch+p.flight+p.hold+p.fade)
	}
	return s
}

// Duration is how long the whole attack lasts.
func (s *Scene) Duration() time.Duration { return s.end }

// Step returns the sprites at time t since the start; done once all are gone.
func (s *Scene) Step(t time.Duration) (sprites []Sprite, done bool) {
	for _, p := range s.pieces {
		if t < p.launch || p.gone {
			continue
		}
		if t >= p.launch+p.flight+p.hold+p.fade {
			p.gone = true
			sprites = append(sprites, Sprite{ID: p.id, Gone: true})
			continue
		}
		if t < p.launch+p.flight {
			sprites = append(sprites, p.flying(float64(t-p.launch)/float64(p.flight), s))
		} else {
			sprites = append(sprites, p.splat(t-p.launch-p.flight, s))
		}
	}
	return sprites, t >= s.end
}

// flying: u goes 0…1; the piece grows as it comes at the viewer.
func (p *piece) flying(u float64, s *Scene) Sprite {
	x := p.fromX + (p.toX-p.fromX)*u
	y := p.fromY + (p.toY-p.fromY)*u - p.arc*4*u*(1-u)
	k := .45 + .75*u
	p.ballLy.clear()
	c := p.ballC
	shift := func(sh []shape) []shape {
		out := make([]shape, len(sh))
		for i, v := range sh {
			out[i] = shape{v.ax + c, v.ay + c, v.bx + c, v.by + c, v.r}
		}
		return out
	}
	body := shift(append(append([]shape{}, p.ball...), p.tail...))
	r := p.ball[0].r
	lights := shift([]shape{circle(-r*.35, -r*.38, r*.28)})
	p.ballLy.draw(body, lights, s.scale, k, c, c)
	p.ballLy.paint(p.ballImg, s.look(), p.ballImg.Rect)
	return Sprite{ID: p.id, X: int(x - c), Y: int(y - c), Img: p.ballImg, Alpha: 255}
}

func (p *piece) splat(st time.Duration, s *Scene) Sprite {
	k := 1.0
	if st < popTime {
		u := float64(st) / float64(popTime)
		k = .6 + .4*(1-(1-u)*(1-u))
	}
	grow := min(1, float64(st)/float64(p.hold+p.fade/2))
	grow = 1 - (1-grow)*(1-grow) // drips slow down as they run out

	sp := Sprite{ID: p.id, Alpha: 255}
	if fadeAt := p.hold; st > fadeAt {
		sp.Alpha = uint8(255 * max(0, 1-float64(st-fadeAt)/float64(p.fade)))
	}
	sp.X = int(p.toX - p.cx)
	sp.Y = int(p.toY - p.cy + p.slide*grow)

	switch {
	case !p.drawn || st < popTime+dripStep: // popping in: the whole picture changes
		body := append([]shape{}, p.body...)
		for i := range p.drips {
			d := &p.drips[i]
			d.drawn = d.length * grow
			body = append(body, capsule(d.x, d.y0, d.x, d.y0+d.drawn, d.r))
		}
		p.ly.clear()
		p.ly.draw(body, p.lights, s.scale, k, p.cx, p.cy)
		p.ly.paint(p.img, s.look(), p.img.Rect)
	case len(p.drips) > 0 && grow < 1 && st-p.lastDraw >= dripStep:
		// drips only grow: adding the new bit of each one to the masks is
		// enough, and only around those bits the picture changes
		var bits []shape
		for i := range p.drips {
			d := &p.drips[i]
			if l := d.length * grow; l > d.drawn {
				bits = append(bits, capsule(d.x, d.y0+d.drawn, d.x, d.y0+l, d.r))
				d.drawn = l
			}
		}
		if len(bits) == 0 {
			return sp
		}
		p.ly.draw(bits, nil, s.scale, 1, p.cx, p.cy)
		x0, y0, x1, y1 := bounds(bits)
		m := 8 * s.scale // rim, shadow offset, antialiasing
		p.ly.paint(p.img, s.look(), image.Rect(int(x0-m), int(y0-m), int(x1+m), int(y1+m)))
	default:
		return sp
	}
	p.drawn, p.lastDraw = true, st
	sp.Img = p.img
	return sp
}

func (s *Scene) look() look {
	if s.kind == Snow {
		return snowLook
	}
	return milkLook
}

// layout moves the splat shapes into a canvas that fits them (with full
// drips, shadow and pop overshoot) and allocates the pictures.
func (p *piece) layout(scale float64) {
	all := append([]shape{}, p.body...)
	for _, d := range p.drips {
		all = append(all, capsule(d.x, d.y0, d.x, d.y0+d.length, d.r))
	}
	x0, y0, x1, y1 := bounds(all)
	pad := 8 * scale
	ox, oy := pad-x0, pad-y0
	move := func(sh []shape) {
		for i := range sh {
			sh[i] = shape{sh[i].ax + ox, sh[i].ay + oy, sh[i].bx + ox, sh[i].by + oy, sh[i].r}
		}
	}
	move(p.body)
	move(p.lights)
	for i := range p.drips {
		p.drips[i].x += ox
		p.drips[i].y0 += oy
	}
	p.cx, p.cy = ox, oy
	w, h := int(x1-x0+2*pad), int(y1-y0+2*pad)
	p.img = image.NewRGBA(image.Rect(0, 0, w, h))
	p.ly = newLayers(w, h)

	bx0, by0, bx1, by1 := bounds(append(append([]shape{}, p.ball...), p.tail...))
	side := 2*math.Max(math.Max(-bx0, bx1), math.Max(-by0, by1))*1.25 + 2*pad
	p.ballC = side / 2
	p.ballImg = image.NewRGBA(image.Rect(0, 0, int(side), int(side)))
	p.ballLy = newLayers(int(side), int(side))
}

func bounds(sh []shape) (x0, y0, x1, y1 float64) {
	x0, y0, x1, y1 = math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, s := range sh {
		x0 = math.Min(x0, math.Min(s.ax, s.bx)-s.r)
		y0 = math.Min(y0, math.Min(s.ay, s.by)-s.r)
		x1 = math.Max(x1, math.Max(s.ax, s.bx)+s.r)
		y1 = math.Max(y1, math.Max(s.ay, s.by)+s.r)
	}
	return
}

// milkShapes: a round splash with lobes, flung droplets and long drips.
func milkShapes(rnd *rand.Rand, R, scale float64) (ball, body, lights []shape, drips []drip) {
	between := func(a, b float64) float64 { return a + rnd.Float64()*(b-a) }
	ball = []shape{circle(0, 0, 15*scale)}
	// a smooth puddle: few big overlapping lobes read as liquid, many small ones as a cloud
	body = []shape{circle(0, 0, R*.55)}
	n := 5 + rnd.IntN(3)
	for i := range n {
		a := 2*math.Pi*float64(i)/float64(n) + between(-.35, .35)
		d := R * between(.25, .45)
		body = append(body, circle(math.Cos(a)*d, math.Sin(a)*d, R*between(.3, .42)))
	}
	for range 2 + rnd.IntN(3) { // a splash arm ending in a fat drop
		a := between(0, 2*math.Pi)
		d := R * between(.85, 1.15)
		ex, ey := math.Cos(a)*d, math.Sin(a)*d
		body = append(body, capsule(0, 0, ex*.88, ey*.88, R*between(.07, .1)), circle(ex, ey, R*between(.11, .15)))
	}
	for range 4 + rnd.IntN(4) {
		a := between(0, 2*math.Pi)
		d := R * between(.85, 1.35)
		body = append(body, circle(math.Cos(a)*d, math.Sin(a)*d, R*between(.035, .07)))
	}
	lights = []shape{capsule(-R*.32, -R*.22, -R*.12, -R*.36, R*.07), circle(R*.05, -R*.4, R*.045)}
	for range 3 + rnd.IntN(3) {
		drips = append(drips, drip{x: R * between(-.55, .55), y0: R * between(.15, .4), r: R * between(.06, .11), length: R * between(.6, 2)})
	}
	return
}

// snowShapes: a packed core, flying chunks and short melt drips.
func snowShapes(rnd *rand.Rand, R, scale float64) (ball, body, lights []shape, drips []drip) {
	between := func(a, b float64) float64 { return a + rnd.Float64()*(b-a) }
	r := 16 * scale
	ball = []shape{circle(0, 0, r), circle(r*.5, -r*.45, r*.5), circle(-r*.55, r*.35, r*.45)}
	for range 5 {
		a, d := between(0, 2*math.Pi), R*between(0, .25)
		body = append(body, circle(math.Cos(a)*d, math.Sin(a)*d, R*between(.3, .42)))
	}
	for range 8 + rnd.IntN(5) { // lumps stuck around the core
		a := between(0, 2*math.Pi)
		d := R * between(.38, .62)
		body = append(body, circle(math.Cos(a)*d, math.Sin(a)*d, R*between(.12, .22)))
	}
	for range 10 + rnd.IntN(8) { // crumbs flung further out
		a := between(0, 2*math.Pi)
		d := R * between(.7, 1.2)
		body = append(body, circle(math.Cos(a)*d, math.Sin(a)*d, R*between(.03, .1)))
	}
	lights = []shape{circle(-R*.16, -R*.2, R*.1), circle(R*.18, -R*.28, R*.05), circle(-R*.3, R*.05, R*.04)}
	for range 1 + rnd.IntN(3) {
		drips = append(drips, drip{x: R * between(-.3, .3), y0: R * between(.2, .35), r: R * between(.05, .07), length: R * between(.25, .5)})
	}
	return
}
