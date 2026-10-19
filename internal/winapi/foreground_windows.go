//go:build windows

package winapi

import (
	"errors"
	"path/filepath"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	procGetLastInputInfo         = user32.NewProc("GetLastInputInfo")
	kernel32                     = windows.NewLazySystemDLL("kernel32.dll")
	procGetTickCount             = kernel32.NewProc("GetTickCount")
)

// ForegroundExe returns the executable file name (e.g. "goland64.exe") of
// the window the user is looking at. Window titles are never read.
func ForegroundExe() (string, error) {
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return "", errors.New("no foreground window")
	}
	var pid uint32
	procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid == 0 {
		return "", errors.New("no process for foreground window")
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", err // e.g. elevated processes when we are not elevated
	}
	defer windows.CloseHandle(h)

	buf := make([]uint16, windows.MAX_LONG_PATH)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
		return "", err
	}
	return filepath.Base(windows.UTF16ToString(buf[:size])), nil
}

type lastInputInfo struct {
	cbSize uint32
	dwTime uint32
}

// IdleDuration is the time since the last keyboard or mouse input.
func IdleDuration() time.Duration {
	lii := lastInputInfo{cbSize: uint32(unsafe.Sizeof(lastInputInfo{}))}
	r, _, _ := procGetLastInputInfo.Call(uintptr(unsafe.Pointer(&lii)))
	if r == 0 {
		return 0
	}
	now, _, _ := procGetTickCount.Call()
	// both are 32-bit millisecond tick counts; unsigned subtraction handles wrap-around
	return time.Duration(uint32(now)-lii.dwTime) * time.Millisecond
}
