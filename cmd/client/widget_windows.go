//go:build windows

package main

import (
	_ "embed"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	webview2 "github.com/jchv/go-webview2"

	"github.com/AlexSol/gachi-streak/internal/winapi"
)

//go:embed widget.html
var widgetHTML string

// widgetData is what the widget page renders (window.gsUpdate).
type widgetData struct {
	Users      []widgetUser  `json:"users"`
	Error      string        `json:"error,omitempty"` // "" | "offline" | "config"
	Demo       string        `json:"demo,omitempty"`
	TodayTotal int64         `json:"today_total"`
	TodayTop   string        `json:"today_top,omitempty"`
	Update     *widgetUpdate `json:"update,omitempty"` // newer client release
	Background *widgetBg     `json:"background,omitempty"`
}

type widgetBg struct {
	URL   string  `json:"url"`
	Video bool    `json:"video"`
	Dim   float64 `json:"dim"` // 0…1, theme color over the background
}

type widgetUpdate struct {
	Version  string `json:"version"`
	Required bool   `json:"required"`
}

type widgetUser struct {
	Name      string `json:"name"`
	Me        bool   `json:"me"`
	Streak    int    `json:"streak"`
	State     string `json:"state"`
	Deadline  string `json:"deadline"`
	DoneToday bool   `json:"done_today"`
	Presence  any    `json:"presence,omitempty"`
}

type widgetPos struct {
	X, Y int32
}

// Widget is a small WebView2 window: a desktop gadget by default, or an
// always-on-top overlay (like Discord's) when switched in the settings.
// It runs on its own OS thread with its own message loop, separate from
// the tray.
type Widget struct {
	dataDir  string
	onPing   func()
	onWeb    func()
	onUpdate func()
	onTheme  func()
	onClose  func()

	mu    sync.Mutex
	wv    webview2.WebView
	hwnd  uintptr
	mode  winapi.WidgetMode
	snap  int    // edge snapping distance at 100% scale
	theme string // client.WidgetThemes ID
	last  []byte // last JSON, replayed when the page (re)loads
}

func newWidget(dataDir string, onPing, onWeb, onUpdate, onTheme, onClose func()) *Widget {
	return &Widget{dataDir: dataDir, onPing: onPing, onWeb: onWeb, onUpdate: onUpdate, onTheme: onTheme, onClose: onClose, mode: winapi.WidgetDesktop}
}

func (w *Widget) posFile() string { return filepath.Join(w.dataDir, "widget.json") }

func (w *Widget) loadPos() (widgetPos, bool) {
	b, err := os.ReadFile(w.posFile())
	if err != nil {
		return widgetPos{}, false
	}
	var p widgetPos
	return p, json.Unmarshal(b, &p) == nil
}

func (w *Widget) savePos(r winapi.Rect) {
	b, _ := json.Marshal(widgetPos{X: r.Left, Y: r.Top})
	if err := os.WriteFile(w.posFile(), b, 0o600); err != nil {
		log.Println("widget: save position:", err)
	}
}

// IsOpen reports whether the widget window exists.
func (w *Widget) IsOpen() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.wv != nil
}

// Show opens the widget (no-op if already open).
func (w *Widget) Show(mode winapi.WidgetMode) {
	w.mu.Lock()
	if w.wv != nil {
		w.mu.Unlock()
		w.SetMode(mode)
		return
	}
	w.mode = mode
	w.mu.Unlock()

	ready := make(chan struct{})
	go w.run(ready)
	<-ready
}

func (w *Widget) run(ready chan struct{}) {
	// WebView2 and its message loop must stay on one OS thread.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	scale := winapi.SystemScale()
	width, height := int32(320*scale), int32(300*scale)

	wv := webview2.NewWithOptions(webview2.WebViewOptions{
		DataPath:  filepath.Join(w.dataDir, "webview2"),
		AutoFocus: false,
		WindowOptions: webview2.WindowOptions{
			Title: "Gachi Streak", Width: uint(width), Height: uint(height),
		},
	})
	if wv == nil {
		close(ready)
		log.Println("widget: WebView2 runtime is not available")
		go winapi.MessageBox("Gachi Streak",
			"Не вдалося відкрити віджет: потрібен Microsoft Edge WebView2 Runtime.\n"+
				"Його можна встановити з https://go.microsoft.com/fwlink/p/?LinkId=2124703", winapi.MBWarning)
		return
	}
	hwnd := uintptr(wv.Window())

	// position: saved one, or the top-right corner of the work area
	pos, ok := w.loadPos()
	work := winapi.WorkArea(pos.X, pos.Y)
	if !ok || pos.X < work.Left-width/2 || pos.X > work.Right-width/2 || pos.Y < work.Top || pos.Y > work.Bottom-height/2 {
		work = winapi.WorkArea(0, 0)
		pos = widgetPos{X: work.Right - width - int32(24*scale), Y: work.Top + int32(24*scale)}
	}

	w.mu.Lock()
	mode, snap, theme := w.mode, w.snap, w.theme
	w.wv, w.hwnd = wv, hwnd
	w.mu.Unlock()

	r := winapi.Rect{Left: pos.X, Top: pos.Y, Right: pos.X + width, Bottom: pos.Y + height}
	winapi.MakeWidget(hwnd, mode, r, int32(float64(snap)*scale), w.savePos)

	_ = wv.Bind("gsReady", func() {
		w.mu.Lock()
		last := w.last
		w.mu.Unlock()
		if last != nil {
			wv.Eval("window.gsUpdate(" + string(last) + ")")
		}
	})
	_ = wv.Bind("gsDrag", func() { winapi.StartDrag(hwnd) })
	_ = wv.Bind("gsPing", func() { go w.onPing() })
	_ = wv.Bind("gsOpenWeb", func() { go w.onWeb() })
	_ = wv.Bind("gsOpenUpdate", func() { go w.onUpdate() })
	_ = wv.Bind("gsNextTheme", func() { go w.onTheme() })
	_ = wv.Bind("gsClose", func() { wv.Destroy() })
	// the theme goes straight into the markup, so the page never flashes the default one
	wv.SetHtml(strings.Replace(widgetHTML, "<html ", "<html data-theme="+strconv.Quote(theme)+" ", 1))
	close(ready)

	wv.Run() // returns when the window is closed

	winapi.ForgetWidget(hwnd)
	w.mu.Lock()
	w.wv, w.hwnd = nil, 0
	w.mu.Unlock()
	if w.onClose != nil {
		w.onClose()
	}
}

// Close hides (destroys) the widget window.
func (w *Widget) Close() {
	w.mu.Lock()
	wv := w.wv
	w.mu.Unlock()
	if wv != nil {
		wv.Destroy()
	}
}

// SetMode switches between desktop gadget and overlay live.
func (w *Widget) SetMode(mode winapi.WidgetMode) {
	w.mu.Lock()
	w.mode = mode
	wv, hwnd := w.wv, w.hwnd
	w.mu.Unlock()
	if wv != nil {
		wv.Dispatch(func() { winapi.SetWidgetMode(hwnd, mode) })
	}
}

// SetSnap changes the edge-snapping distance (px at 100% scale, 0 = off).
func (w *Widget) SetSnap(px int) {
	if px < 0 {
		px = 0
	}
	w.mu.Lock()
	w.snap = px
	hwnd := w.hwnd
	w.mu.Unlock()
	if hwnd != 0 {
		winapi.SetWidgetSnap(hwnd, int32(float64(px)*winapi.SystemScale()))
	}
}

// SetTheme switches the color theme live (id from client.WidgetThemes).
func (w *Widget) SetTheme(id string) {
	w.mu.Lock()
	w.theme = id
	wv := w.wv
	w.mu.Unlock()
	if wv != nil {
		wv.Dispatch(func() { wv.Eval("window.gsTheme && window.gsTheme(" + strconv.Quote(id) + ")") })
	}
}

// Update pushes fresh data to the page (safe from any goroutine).
func (w *Widget) Update(d widgetData) {
	b, err := json.Marshal(d)
	if err != nil {
		return
	}
	w.mu.Lock()
	w.last = b
	wv := w.wv
	w.mu.Unlock()
	if wv != nil {
		wv.Dispatch(func() { wv.Eval("window.gsUpdate && window.gsUpdate(" + string(b) + ")") })
	}
}
