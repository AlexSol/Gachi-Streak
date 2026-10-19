//go:build windows

// Package winapi implements the OS side of the agent on Windows through
// plain Win32 calls (no CGO): SystemParametersInfoW for the wallpaper,
// the registry for its style, PlaySoundW for sounds.
package winapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	user32                   = windows.NewLazySystemDLL("user32.dll")
	procSystemParametersInfo = user32.NewProc("SystemParametersInfoW")
)

const (
	spiGetDeskWallpaper  = 0x0073
	spiSetDeskWallpaper  = 0x0014
	spifUpdateIniFile    = 0x01
	spifSendWinIniChange = 0x02
	desktopKey           = `Control Panel\Desktop`
	styleFill            = "10" // WallpaperStyle: 10 = Fill, 6 = Fit, 2 = Stretch, 0 = Center/Tile
)

// GetWallpaper returns the path of the current wallpaper ("" = solid color).
func GetWallpaper() (string, error) {
	buf := make([]uint16, 1024)
	r, _, err := procSystemParametersInfo.Call(
		spiGetDeskWallpaper, uintptr(len(buf)), uintptr(unsafe.Pointer(&buf[0])), 0)
	if r == 0 {
		return "", fmt.Errorf("SystemParametersInfoW(GET): %w", err)
	}
	return windows.UTF16ToString(buf), nil
}

// SetWallpaper sets the wallpaper image and its style ("" path = no image).
func SetWallpaper(path, style, tile string) error {
	if k, err := registry.OpenKey(registry.CURRENT_USER, desktopKey, registry.SET_VALUE); err == nil {
		_ = k.SetStringValue("WallpaperStyle", style)
		_ = k.SetStringValue("TileWallpaper", tile)
		k.Close()
	}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	r, _, callErr := procSystemParametersInfo.Call(
		spiSetDeskWallpaper, 0, uintptr(unsafe.Pointer(p)), spifUpdateIniFile|spifSendWinIniChange)
	if r == 0 {
		return fmt.Errorf("SystemParametersInfoW(SET %q): %w", path, callErr)
	}
	return nil
}

func readStyle() (style, tile string) {
	k, err := registry.OpenKey(registry.CURRENT_USER, desktopKey, registry.QUERY_VALUE)
	if err != nil {
		return styleFill, "0"
	}
	defer k.Close()
	style, _, _ = k.GetStringValue("WallpaperStyle")
	tile, _, _ = k.GetStringValue("TileWallpaper")
	if style == "" {
		style = styleFill
	}
	if tile == "" {
		tile = "0"
	}
	return style, tile
}

// Desktop implements client.Desktop on Windows.
type Desktop struct {
	DataDir string   // %APPDATA%\GachiStreak
	Managed []string // folders whose images are "ours" (never backed up)
}

type backupInfo struct {
	Path  string `json:"path"`  // original wallpaper path as Windows reported it
	Copy  string `json:"copy"`  // our copy of that file, taken before punishing
	Style string `json:"style"` // WallpaperStyle
	Tile  string `json:"tile"`  // TileWallpaper
}

func (d *Desktop) backupFile() string { return filepath.Join(d.DataDir, "backup", "backup.json") }

func (d *Desktop) isManaged(path string) bool {
	if path == "" {
		return false
	}
	p := strings.ToLower(filepath.Clean(path))
	for _, dir := range append([]string{d.DataDir}, d.Managed...) {
		if dir == "" {
			continue
		}
		prefix := strings.ToLower(filepath.Clean(dir)) + string(filepath.Separator)
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return false
}

// BackupWallpaper saves the current wallpaper (path, a copy of the file and
// its style) unless the current wallpaper is already one of ours.
func (d *Desktop) BackupWallpaper() error {
	cur, err := GetWallpaper()
	if err != nil {
		return err
	}
	if d.isManaged(cur) {
		return nil // already punished; keep the existing backup
	}
	style, tile := readStyle()
	info := backupInfo{Path: cur, Style: style, Tile: tile}

	dir := filepath.Dir(d.backupFile())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if cur != "" {
		// A copy is essential: Windows often reports ...\Themes\TranscodedWallpaper,
		// which gets overwritten the moment we set our own image.
		ext := strings.ToLower(filepath.Ext(cur))
		if ext == "" {
			ext = ".jpg"
		}
		dst := filepath.Join(dir, "original"+ext)
		if err := copyFile(cur, dst); err == nil {
			info.Copy = dst
		}
	}
	b, _ := json.MarshalIndent(info, "", "  ")
	return os.WriteFile(d.backupFile(), b, 0o600)
}

// RestoreWallpaper puts the backed up wallpaper back.
func (d *Desktop) RestoreWallpaper() error {
	b, err := os.ReadFile(d.backupFile())
	if errors.Is(err, os.ErrNotExist) {
		return nil // nothing was ever backed up
	}
	if err != nil {
		return err
	}
	var info backupInfo
	if err := json.Unmarshal(b, &info); err != nil {
		return err
	}
	path := ""
	switch {
	case info.Copy != "" && exists(info.Copy):
		path = info.Copy
	case info.Path != "" && exists(info.Path) && !d.isManaged(info.Path):
		path = info.Path
	}
	return SetWallpaper(path, info.Style, info.Tile)
}

// SetWallpaper implements client.Desktop: image stretched to fill the screen.
func (d *Desktop) SetWallpaper(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	return SetWallpaper(abs, styleFill, "0")
}

// PlaySound implements client.Desktop.
func (d *Desktop) PlaySound(spec string) { PlaySound(spec) }

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
