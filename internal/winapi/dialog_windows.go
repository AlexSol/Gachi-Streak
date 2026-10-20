//go:build windows

package winapi

import (
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	comdlg32                = windows.NewLazySystemDLL("comdlg32.dll")
	procGetOpenFileNameW    = comdlg32.NewProc("GetOpenFileNameW")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procIsIconic            = user32.NewProc("IsIconic")
)

// openFileName is OPENFILENAMEW; Go's natural alignment matches the C layout.
type openFileName struct {
	structSize    uint32
	owner         uintptr
	instance      uintptr
	filter        *uint16
	customFilter  *uint16
	maxCustFilter uint32
	filterIndex   uint32
	file          *uint16
	maxFile       uint32
	fileTitle     *uint16
	maxFileTitle  uint32
	initialDir    *uint16
	title         *uint16
	flags         uint32
	fileOffset    uint16
	fileExtension uint16
	defExt        *uint16
	custData      uintptr
	hook          uintptr
	templateName  *uint16
	reserved      uintptr
	reserved2     uint32
	flagsEx       uint32
}

const (
	ofnNoChangeDir     = 0x00000008
	ofnExplorer        = 0x00080000
	ofnPathMustExist   = 0x00000800
	ofnFileMustExist   = 0x00001000
	ofnHideReadOnly    = 0x00000004
	ofnDontAddToRecent = 0x02000000
)

// FileFilter is one line of the "file type" box, e.g. {"Videos", "*.webm;*.mp4"}.
type FileFilter struct {
	Name, Pattern string
}

// OpenFileDialog shows the standard "Open" dialog (modal to owner, may be 0).
// Returns "" when cancelled.
func OpenFileDialog(owner uintptr, title, initialDir string, filters []FileFilter) string {
	var f []rune
	for _, x := range filters {
		f = append(f, []rune(x.Name)...)
		f = append(f, 0)
		f = append(f, []rune(x.Pattern)...)
		f = append(f, 0)
	}
	filter := utf16.Encode(append(f, 0))
	buf := make([]uint16, windows.MAX_LONG_PATH)
	t, _ := windows.UTF16PtrFromString(title)
	var dir *uint16
	if initialDir != "" {
		dir, _ = windows.UTF16PtrFromString(initialDir)
	}
	ofn := openFileName{
		owner:      owner,
		filter:     &filter[0],
		file:       &buf[0],
		maxFile:    uint32(len(buf)),
		initialDir: dir,
		title:      t,
		flags:      ofnExplorer | ofnPathMustExist | ofnFileMustExist | ofnNoChangeDir | ofnHideReadOnly | ofnDontAddToRecent,
	}
	ofn.structSize = uint32(unsafe.Sizeof(ofn))
	if r, _, _ := procGetOpenFileNameW.Call(uintptr(unsafe.Pointer(&ofn))); r == 0 {
		return ""
	}
	return windows.UTF16ToString(buf)
}

// BringToFront restores (if minimized) and focuses a window.
func BringToFront(hwnd uintptr) {
	if r, _, _ := procIsIconic.Call(hwnd); r != 0 {
		const swRestore = 9
		procShowWindow.Call(hwnd, swRestore)
	}
	procSetForegroundWindow.Call(hwnd)
}
