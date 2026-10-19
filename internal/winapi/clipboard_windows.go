//go:build windows

package winapi

import (
	"errors"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procOpenClipboard    = user32.NewProc("OpenClipboard")
	procCloseClipboard   = user32.NewProc("CloseClipboard")
	procEmptyClipboard   = user32.NewProc("EmptyClipboard")
	procSetClipboardData = user32.NewProc("SetClipboardData")
	procGetClipboardData = user32.NewProc("GetClipboardData")

	procGlobalAlloc  = kernel32.NewProc("GlobalAlloc")
	procGlobalFree   = kernel32.NewProc("GlobalFree")
	procGlobalLock   = kernel32.NewProc("GlobalLock")
	procGlobalUnlock = kernel32.NewProc("GlobalUnlock")
	// Memory copies go through the API: the global memory handle is a
	// uintptr that must never be turned into a Go pointer.
	procMoveMemory = kernel32.NewProc("RtlMoveMemory")
	procLstrlenW   = kernel32.NewProc("lstrlenW")
)

const (
	cfUnicodeText = 13
	gmemMoveable  = 0x0002
)

// openClipboard retries for a moment: another app may hold the clipboard.
func openClipboard() error {
	var err error
	for i := 0; i < 10; i++ {
		var r uintptr
		r, _, err = procOpenClipboard.Call(0)
		if r != 0 {
			return nil
		}
		time.Sleep(30 * time.Millisecond)
	}
	return errors.Join(errors.New("clipboard is busy"), err)
}

// SetClipboardText puts text on the clipboard as Unicode text.
func SetClipboardText(text string) error {
	u, err := windows.UTF16FromString(text)
	if err != nil {
		return err
	}
	if err := openClipboard(); err != nil {
		return err
	}
	defer procCloseClipboard.Call()
	if r, _, err := procEmptyClipboard.Call(); r == 0 {
		return err
	}
	size := uintptr(len(u)) * 2
	h, _, err := procGlobalAlloc.Call(gmemMoveable, size)
	if h == 0 {
		return err
	}
	p, _, err := procGlobalLock.Call(h)
	if p == 0 {
		procGlobalFree.Call(h)
		return err
	}
	procMoveMemory.Call(p, uintptr(unsafe.Pointer(&u[0])), size)
	procGlobalUnlock.Call(h)
	if r, _, err := procSetClipboardData.Call(cfUnicodeText, h); r == 0 {
		procGlobalFree.Call(h) // ownership passes to the system only on success
		return err
	}
	return nil
}

// ClipboardText returns the clipboard's Unicode text ("" if none).
func ClipboardText() (string, error) {
	if err := openClipboard(); err != nil {
		return "", err
	}
	defer procCloseClipboard.Call()
	h, _, _ := procGetClipboardData.Call(cfUnicodeText)
	if h == 0 {
		return "", nil
	}
	p, _, err := procGlobalLock.Call(h)
	if p == 0 {
		return "", err
	}
	defer procGlobalUnlock.Call(h)
	n, _, _ := procLstrlenW.Call(p)
	if n == 0 {
		return "", nil
	}
	buf := make([]uint16, n)
	procMoveMemory.Call(uintptr(unsafe.Pointer(&buf[0])), p, n*2)
	return windows.UTF16ToString(buf), nil
}
