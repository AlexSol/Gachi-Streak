//go:build windows

package splash

import (
	"image"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                  = windows.NewLazySystemDLL("user32.dll")
	gdi32                   = windows.NewLazySystemDLL("gdi32.dll")
	procRegisterClassExW    = user32.NewProc("RegisterClassExW")
	procCreateWindowExW     = user32.NewProc("CreateWindowExW")
	procDestroyWindow       = user32.NewProc("DestroyWindow")
	procShowWindow          = user32.NewProc("ShowWindow")
	procUpdateLayeredWindow = user32.NewProc("UpdateLayeredWindow")
	procDefWindowProcW      = user32.NewProc("DefWindowProcW")
	procPeekMessageW        = user32.NewProc("PeekMessageW")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procDispatchMessageW    = user32.NewProc("DispatchMessageW")
	procGetDC               = user32.NewProc("GetDC")
	procReleaseDC           = user32.NewProc("ReleaseDC")
	procCreateCompatibleDC  = gdi32.NewProc("CreateCompatibleDC")
	procDeleteDC            = gdi32.NewProc("DeleteDC")
	procCreateDIBSection    = gdi32.NewProc("CreateDIBSection")
	procSelectObject        = gdi32.NewProc("SelectObject")
	procDeleteObject        = gdi32.NewProc("DeleteObject")
)

const (
	wsPopup          = 0x80000000
	wsExLayered      = 0x00080000
	wsExTransparent  = 0x00000020 // clicks go through
	wsExTopmost      = 0x00000008
	wsExToolWindow   = 0x00000080 // no taskbar button
	wsExNoActivate   = 0x08000000 // never steals focus
	swShowNoActivate = 4
	ulwAlpha         = 2
	acSrcAlpha       = 1
	pmRemove         = 1
)

type wndClassEx struct {
	size       uint32
	style      uint32
	wndProc    uintptr
	clsExtra   int32
	wndExtra   int32
	instance   windows.Handle
	icon       uintptr
	cursor     uintptr
	background uintptr
	menuName   *uint16
	className  *uint16
	iconSm     uintptr
}

type bitmapInfoHeader struct {
	size                         uint32
	width, height                int32
	planes, bitCount             uint16
	compression, sizeImage       uint32
	xPelsPerMeter, yPelsPerMeter int32
	clrUsed, clrImportant        uint32
}

type point struct{ x, y int32 }
type size struct{ cx, cy int32 }

type blendFunction struct {
	op, flags, constAlpha, alphaFormat byte
}

type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      point
	private uint32
}

var (
	classOnce sync.Once
	className *uint16
	classErr  error
	busy      sync.Mutex // one attack at a time
)

func registerClass() {
	className, _ = windows.UTF16PtrFromString("GachiStreakSplash")
	var inst windows.Handle
	if classErr = windows.GetModuleHandleEx(0, nil, &inst); classErr != nil {
		return
	}
	wc := wndClassEx{wndProc: procDefWindowProcW.Addr(), instance: inst, className: className}
	wc.size = uint32(unsafe.Sizeof(wc))
	if r, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		classErr = err
	}
}

// piece window: a layered popup showing one sprite.
type layered struct {
	hwnd, memDC, bmp uintptr
	bits             unsafe.Pointer
	w, h             int
	shown            bool
}

func newLayered() (*layered, error) {
	hwnd, _, err := procCreateWindowExW.Call(
		wsExLayered|wsExTransparent|wsExTopmost|wsExToolWindow|wsExNoActivate,
		uintptr(unsafe.Pointer(className)), 0, wsPopup, 0, 0, 1, 1, 0, 0, 0, 0)
	if hwnd == 0 {
		return nil, err
	}
	screen, _, _ := procGetDC.Call(0)
	dc, _, _ := procCreateCompatibleDC.Call(screen)
	procReleaseDC.Call(0, screen)
	return &layered{hwnd: hwnd, memDC: dc}, nil
}

// picture copies img (premultiplied RGBA) into the window's 32-bit DIB.
func (l *layered) picture(img *image.RGBA) {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	if w != l.w || h != l.h || l.bmp == 0 {
		if l.bmp != 0 {
			procDeleteObject.Call(l.bmp)
		}
		bi := bitmapInfoHeader{width: int32(w), height: -int32(h), planes: 1, bitCount: 32} // top-down BGRA
		bi.size = uint32(unsafe.Sizeof(bi))
		l.bmp, _, _ = procCreateDIBSection.Call(l.memDC, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&l.bits)), 0, 0)
		procSelectObject.Call(l.memDC, l.bmp)
		l.w, l.h = w, h
	}
	if l.bmp == 0 {
		return
	}
	dst := unsafe.Slice((*byte)(l.bits), w*h*4)
	for i := 0; i < len(dst); i += 4 {
		dst[i], dst[i+1], dst[i+2], dst[i+3] = img.Pix[i+2], img.Pix[i+1], img.Pix[i], img.Pix[i+3]
	}
}

// update moves the window and, if repaint, uploads the DIB; alpha fades it.
func (l *layered) update(x, y int, alpha uint8, repaint bool) {
	pt := point{int32(x), int32(y)}
	blend := blendFunction{constAlpha: alpha, alphaFormat: acSrcAlpha}
	if repaint && l.bmp != 0 {
		sz, src := size{int32(l.w), int32(l.h)}, point{}
		procUpdateLayeredWindow.Call(l.hwnd, 0, uintptr(unsafe.Pointer(&pt)), uintptr(unsafe.Pointer(&sz)),
			l.memDC, uintptr(unsafe.Pointer(&src)), 0, uintptr(unsafe.Pointer(&blend)), ulwAlpha)
	} else { // same picture: just move / fade
		procUpdateLayeredWindow.Call(l.hwnd, 0, uintptr(unsafe.Pointer(&pt)), 0, 0, 0, 0, uintptr(unsafe.Pointer(&blend)), ulwAlpha)
	}
	if !l.shown {
		procShowWindow.Call(l.hwnd, swShowNoActivate)
		l.shown = true
	}
}

func (l *layered) close() {
	procDestroyWindow.Call(l.hwnd)
	if l.bmp != 0 {
		procDeleteObject.Call(l.bmp)
	}
	procDeleteDC.Call(l.memDC)
}

// Run plays one attack over area (a monitor's work area, physical pixels),
// thrown from `from`, and returns when every piece is gone (a few seconds).
// A second call while one is running returns at once.
func Run(kind Kind, area image.Rectangle, from image.Point, scale float64) error {
	if !busy.TryLock() {
		return nil
	}
	defer busy.Unlock()
	// windows belong to the thread that created them and need its message loop
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	classOnce.Do(registerClass)
	if classErr != nil {
		return classErr
	}

	scene := NewScene(kind, area, from, scale, uint64(time.Now().UnixNano()))
	wins := map[int]*layered{}
	defer func() {
		for _, w := range wins {
			w.close()
		}
	}()
	start := time.Now()
	tick := time.NewTicker(time.Second / 60)
	defer tick.Stop()
	for {
		sprites, done := scene.Step(time.Since(start))
		for _, sp := range sprites {
			w := wins[sp.ID]
			if sp.Gone {
				if w != nil {
					w.close()
					delete(wins, sp.ID)
				}
				continue
			}
			if w == nil {
				var err error
				if w, err = newLayered(); err != nil {
					return err
				}
				wins[sp.ID] = w
			}
			if sp.Img != nil {
				w.picture(sp.Img)
			}
			w.update(sp.X, sp.Y, sp.Alpha, sp.Img != nil)
		}
		pump()
		if done {
			return nil
		}
		<-tick.C
	}
}

// pump answers whatever Windows sent to our windows (e.g. broadcasts), so
// nobody waits on them and they never look "not responding".
func pump() {
	var m msg
	for {
		r, _, _ := procPeekMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, pmRemove)
		if r == 0 {
			return
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}
