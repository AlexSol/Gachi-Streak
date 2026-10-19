//go:build windows

package winapi

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	winmm         = windows.NewLazySystemDLL("winmm.dll")
	procPlaySound = winmm.NewProc("PlaySoundW")
)

const (
	sndAsync     = 0x0001
	sndNoDefault = 0x0002
	sndAlias     = 0x00010000
	sndFilename  = 0x00020000
)

// PlaySound plays a .wav file or a system alias ("SystemHand",
// "SystemExclamation", ...) asynchronously. Empty spec = silence.
func PlaySound(spec string) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return
	}
	flags := uintptr(sndAsync | sndNoDefault | sndAlias)
	if strings.HasSuffix(strings.ToLower(spec), ".wav") {
		flags = sndAsync | sndNoDefault | sndFilename
	}
	p, err := windows.UTF16PtrFromString(spec)
	if err != nil {
		return
	}
	procPlaySound.Call(uintptr(unsafe.Pointer(p)), 0, flags)
}

// MessageBox icons.
const (
	MBInfo    = windows.MB_ICONINFORMATION
	MBWarning = windows.MB_ICONWARNING
	MBError   = windows.MB_ICONERROR
)

// MessageBox shows a modal message box.
func MessageBox(title, text string, icon uint32) {
	t, _ := windows.UTF16PtrFromString(title)
	m, _ := windows.UTF16PtrFromString(text)
	windows.MessageBox(0, m, t, windows.MB_OK|icon|windows.MB_SETFOREGROUND|windows.MB_TOPMOST)
}

// Open opens a file or folder with its default handler (Explorer, Notepad...).
func Open(path string) error {
	verb, _ := windows.UTF16PtrFromString("open")
	file, _ := windows.UTF16PtrFromString(path)
	return windows.ShellExecute(0, verb, file, nil, nil, windows.SW_SHOWNORMAL)
}

// DataDir returns %APPDATA%\GachiStreak.
func DataDir() (string, error) {
	base, err := os.UserConfigDir() // %APPDATA% on Windows
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "GachiStreak"), nil
}

// SingleInstance returns false if another copy of the agent is running.
// The mutex is held for the process lifetime.
func SingleInstance(name string) bool {
	n, _ := windows.UTF16PtrFromString(`Local\` + name)
	_, err := windows.CreateMutex(nil, false, n)
	return !errors.Is(err, windows.ERROR_ALREADY_EXISTS)
}

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`

// AutostartEnabled reports whether the agent starts with Windows.
func AutostartEnabled(name string) bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue(name)
	return err == nil
}

// SetAutostart adds or removes the current executable from HKCU\...\Run.
func SetAutostart(name string, on bool) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if !on {
		err := k.DeleteValue(name)
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return k.SetStringValue(name, `"`+exe+`"`)
}
