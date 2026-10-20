//go:build windows

package main

import (
	"image"
	"log"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/AlexSol/gachi-streak/internal/splash"
	"github.com/AlexSol/gachi-streak/internal/winapi"
)

// heroRelease is when the character's throw/pour animation lets go,
// so the pieces leave the widget right as his arm swings.
const heroRelease = 650 * time.Millisecond

// attacker decides when the widget character throws snowballs or spills
// milk over the screen; internal/splash draws it.
type attacker struct {
	a *app

	mu    sync.Mutex
	state string    // last seen state of "me" (or the demo)
	next  time.Time // next periodic attack, zero = not planned
}

// kindFor: warning → snowballs, failed → milk, otherwise either.
func kindFor(state string) splash.Kind {
	switch state {
	case "warning":
		return splash.Snow
	case "failed":
		return splash.Milk
	}
	if rand.IntN(2) == 0 {
		return splash.Snow
	}
	return splash.Milk
}

// observe is called on every widget push (with a.mu held, must not block):
// turning warning or failed earns an attack right away.
func (t *attacker) observe(state string) {
	t.mu.Lock()
	prev := t.state
	t.state = state
	t.mu.Unlock()
	if prev != "" && prev != state && (state == "warning" || state == "failed") {
		go t.launch(kindFor(state), false)
	}
}

// loop runs the periodic attacks.
func (t *attacker) loop() {
	for range time.Tick(30 * time.Second) {
		cfg := t.a.cfg
		now := time.Now()
		t.mu.Lock()
		if !cfg.ScreenAttacks || cfg.ScreenAttackMinutes <= 0 {
			t.next = time.Time{}
			t.mu.Unlock()
			continue
		}
		every := time.Duration(float64(cfg.ScreenAttackMinutes) * (.6 + .8*rand.Float64()) * float64(time.Minute))
		if t.next.IsZero() || t.next.Sub(now) > 2*time.Duration(cfg.ScreenAttackMinutes)*time.Minute {
			t.next = now.Add(every) // first plan, or the interval was shortened
		}
		due, state := now.After(t.next), t.state
		if due {
			t.next = now.Add(every)
		}
		t.mu.Unlock()
		if due {
			t.launch(kindFor(state), false)
		}
	}
}

// launch plays one attack. Real ones respect the settings and stay away
// from fullscreen apps, presentations, the snooze and an empty desk;
// demo ones (tray / settings buttons) always run.
func (t *attacker) launch(kind splash.Kind, demo bool) {
	a := t.a
	if !demo {
		if !a.cfg.ScreenAttacks || winapi.UserBusy() || winapi.IdleDuration() > 5*time.Minute {
			return
		}
		snoozed := false
		a.locked(func() { snoozed = time.Now().Before(a.engine.State.SnoozeUntil) })
		if snoozed {
			return
		}
	}
	scale := winapi.SystemScale()
	from, ok := a.widget.HeroPoint()
	if ok {
		a.widget.HeroAttack(kind)
		time.Sleep(heroRelease)
	} else { // no widget: from the bottom-right corner of the main screen
		work := winapi.WorkArea(0, 0)
		from = image.Pt(int(work.Right)-int(60*scale), int(work.Bottom)-int(60*scale))
	}
	w := winapi.WorkArea(int32(from.X), int32(from.Y))
	if err := splash.Run(kind, image.Rect(int(w.Left), int(w.Top), int(w.Right), int(w.Bottom)), from, scale); err != nil {
		log.Println("screen attack:", err)
	}
}
