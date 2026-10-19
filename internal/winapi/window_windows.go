//go:build windows

package winapi

import (
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procSetWindowLongPtrW         = user32.NewProc("SetWindowLongPtrW")
	procGetWindowLongPtrW         = user32.NewProc("GetWindowLongPtrW")
	procCallWindowProcW           = user32.NewProc("CallWindowProcW")
	procSetWindowPos              = user32.NewProc("SetWindowPos")
	procGetWindowRect             = user32.NewProc("GetWindowRect")
	procGetCursorPos              = user32.NewProc("GetCursorPos")
	procReleaseCapture            = user32.NewProc("ReleaseCapture")
	procSendMessageW              = user32.NewProc("SendMessageW")
	procShowWindow                = user32.NewProc("ShowWindow")
	procSetProcessDpiAwarenessCtx = user32.NewProc("SetProcessDpiAwarenessContext")
	procGetDpiForSystem           = user32.NewProc("GetDpiForSystem")
	procMonitorFromPoint          = user32.NewProc("MonitorFromPoint")
	procGetMonitorInfoW           = user32.NewProc("GetMonitorInfoW")
	dwmapi                        = windows.NewLazySystemDLL("dwmapi.dll")
	procDwmSetWindowAttribute     = dwmapi.NewProc("DwmSetWindowAttribute")
)

const (
	gwlStyle    = ^uintptr(15) // -16
	gwlExStyle  = ^uintptr(19) // -20
	gwlpWndProc = ^uintptr(3)  // -4

	wsPopup      = 0x80000000
	wsVisible    = 0x10000000
	wsClipChild  = 0x02000000
	wsExToolWin  = 0x00000080
	wsExTopmost  = 0x00000008
	wsExNoActive = 0x08000000

	hwndTop       = 0
	hwndBottom    = 1
	hwndTopmost   = ^uintptr(0) // -1
	hwndNoTopmost = ^uintptr(1) // -2

	swpNoSize       = 0x0001
	swpNoMove       = 0x0002
	swpNoZOrder     = 0x0004
	swpNoActivate   = 0x0010
	swpFrameChanged = 0x0020
	swpShowWindow   = 0x0040

	wmWindowPosChanging = 0x0046
	wmMoving            = 0x0216
	wmEnterSizeMove     = 0x0231
	wmExitSizeMove      = 0x0232
	wmNCLButtonDown     = 0x00A1
	htCaption           = 2
	swHide              = 0
	swShowNoActivate    = 4

	dwmwaWindowCornerPreference = 33
	dwmwcpRound                 = 2
)

type windowPos struct {
	hwnd            uintptr
	hwndInsertAfter uintptr
	x, y, cx, cy    int32
	flags           uint32
}

// WidgetMode decides where the widget lives in the window stack.
type WidgetMode string

const (
	// WidgetDesktop keeps the window at the bottom, like a desktop gadget.
	WidgetDesktop WidgetMode = "desktop"
	// WidgetOverlay keeps the window above everything, like Discord's overlay.
	WidgetOverlay WidgetMode = "overlay"
)

type subclass struct {
	old     uintptr
	mode    WidgetMode
	snap    int32 // edge-snapping distance in px, 0 = off
	onMoved func(Rect)

	// drag start, so snapping works on the unsnapped position
	dragging    bool
	startRect   Rect
	startCursor Point
}

var (
	subMu   sync.Mutex
	subs    = map[uintptr]*subclass{}
	subProc = windows.NewCallback(widgetWndProc)
)

func widgetWndProc(hwnd, msg, wp, lp uintptr) uintptr {
	subMu.Lock()
	s := subs[hwnd]
	subMu.Unlock()
	if s == nil {
		return 0
	}
	switch msg {
	case wmWindowPosChanging:
		if s.mode == WidgetDesktop {
			// Whatever activated us, stay under every other window.
			// lp is a *WINDOWPOS owned by Windows for the duration of this call
			pos := *(**windowPos)(unsafe.Pointer(&lp))
			pos.hwndInsertAfter = hwndBottom
			pos.flags &^= swpNoZOrder
		}
	case wmEnterSizeMove:
		s.startRect = WindowRect(hwnd)
		s.startCursor = cursorPos()
		s.dragging = true
	case wmMoving:
		if s.snap > 0 && s.dragging {
			// Windows proposes "previous position + mouse delta"; after a snap
			// that would keep it stuck forever. Snap the unsnapped position instead.
			// lp is a *RECT owned by Windows for the duration of this call.
			r := *(**Rect)(unsafe.Pointer(&lp))
			raw := DragRect(s.startRect, s.startCursor, cursorPos())
			area := WorkArea((raw.Left+raw.Right)/2, (raw.Top+raw.Bottom)/2)
			*r = SnapRect(raw, area, s.snap)
		}
	case wmExitSizeMove:
		s.dragging = false
		if s.onMoved != nil {
			s.onMoved(WindowRect(hwnd))
		}
	}
	r, _, _ := procCallWindowProcW.Call(s.old, hwnd, msg, wp, lp)
	return r
}

// EnableDPIAwareness makes windows crisp on scaled displays (Win10 1703+).
// Must be called before any window is created; errors are ignored.
func EnableDPIAwareness() {
	const perMonitorAwareV2 = ^uintptr(3) // -4
	if procSetProcessDpiAwarenessCtx.Find() == nil {
		procSetProcessDpiAwarenessCtx.Call(perMonitorAwareV2)
	}
}

// SystemScale returns the display scale factor (1.0 = 96 DPI).
func SystemScale() float64 {
	if procGetDpiForSystem.Find() != nil {
		return 1
	}
	dpi, _, _ := procGetDpiForSystem.Call()
	if dpi == 0 {
		return 1
	}
	return float64(dpi) / 96
}

// WorkArea returns the usable area (without the taskbar) of the monitor
// that contains the given point.
func WorkArea(x, y int32) Rect {
	type monitorInfo struct {
		cbSize    uint32
		rcMonitor Rect
		rcWork    Rect
		dwFlags   uint32
	}
	const monitorDefaultToNearest = 2
	pt := uintptr(uint32(x)) | uintptr(uint32(y))<<32
	hmon, _, _ := procMonitorFromPoint.Call(pt, monitorDefaultToNearest)
	mi := monitorInfo{cbSize: uint32(unsafe.Sizeof(monitorInfo{}))}
	if r, _, _ := procGetMonitorInfoW.Call(hmon, uintptr(unsafe.Pointer(&mi))); r == 0 {
		return Rect{0, 0, 1920, 1080}
	}
	return mi.rcWork
}

func cursorPos() Point {
	var p Point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	return p
}

// WindowRect returns the window rectangle in screen coordinates.
func WindowRect(hwnd uintptr) Rect {
	var r Rect
	procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	return r
}

// MakeWidget turns a normal top-level window into a borderless tool window
// (no taskbar button, no Alt+Tab), places it and applies the mode.
// onMoved is called after the user finishes dragging it.
func MakeWidget(hwnd uintptr, mode WidgetMode, r Rect, snap int32, onMoved func(Rect)) {
	procSetWindowLongPtrW.Call(hwnd, gwlStyle, wsPopup|wsVisible|wsClipChild)
	procSetWindowLongPtrW.Call(hwnd, gwlExStyle, wsExToolWin)

	pref := uint32(dwmwcpRound) // rounded corners on Windows 11; ignored elsewhere
	if procDwmSetWindowAttribute.Find() == nil {
		procDwmSetWindowAttribute.Call(hwnd, dwmwaWindowCornerPreference, uintptr(unsafe.Pointer(&pref)), 4)
	}

	old, _, _ := procGetWindowLongPtrW.Call(hwnd, gwlpWndProc)
	subMu.Lock()
	subs[hwnd] = &subclass{old: old, mode: mode, snap: snap, onMoved: onMoved}
	subMu.Unlock()
	procSetWindowLongPtrW.Call(hwnd, gwlpWndProc, subProc)

	procSetWindowPos.Call(hwnd, zFor(mode), uintptr(r.Left), uintptr(r.Top),
		uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top), swpFrameChanged|swpShowWindow|swpNoActivate)
}

// SetWidgetMode switches a widget between desktop and overlay live.
func SetWidgetMode(hwnd uintptr, mode WidgetMode) {
	subMu.Lock()
	if s := subs[hwnd]; s != nil {
		s.mode = mode
	}
	subMu.Unlock()
	if mode == WidgetDesktop {
		// leave the topmost band first, then sink to the bottom
		procSetWindowPos.Call(hwnd, hwndNoTopmost, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)
	}
	procSetWindowPos.Call(hwnd, zFor(mode), 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)
}

// SetWidgetSnap changes the edge-snapping distance live (0 = off).
func SetWidgetSnap(hwnd uintptr, px int32) {
	subMu.Lock()
	if s := subs[hwnd]; s != nil {
		s.snap = px
	}
	subMu.Unlock()
}

// ForgetWidget drops the subclass bookkeeping once the window is gone.
func ForgetWidget(hwnd uintptr) {
	subMu.Lock()
	delete(subs, hwnd)
	subMu.Unlock()
}

func zFor(mode WidgetMode) uintptr {
	if mode == WidgetOverlay {
		return hwndTopmost
	}
	return hwndBottom
}

// StartDrag lets the user move a borderless window by dragging its body.
// Must be called on the window's thread while the mouse button is down.
func StartDrag(hwnd uintptr) {
	procReleaseCapture.Call()
	procSendMessageW.Call(hwnd, wmNCLButtonDown, htCaption, 0)
}
