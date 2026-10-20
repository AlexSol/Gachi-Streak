package client

import (
	"os"
	"strconv"
	"strings"

	"github.com/AlexSol/gachi-streak/internal/api"
)

// Update is a newer client release the user should install.
type Update struct {
	Latest   string // "0.3.0"
	URL      string // release page
	Required bool   // the running version is below the server's minimum
}

// ParseVersion parses a release version "1.2.3" (a leading "v" is fine).
// Anything else, e.g. "dev" or "0.3.0-4-gabc", is a development build.
func ParseVersion(s string) ([3]int, bool) {
	var v [3]int
	parts := strings.Split(strings.TrimPrefix(s, "v"), ".")
	if len(parts) != 3 {
		return v, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || p != strconv.Itoa(n) {
			return v, false
		}
		v[i] = n
	}
	return v, true
}

// IsRelease reports whether version is a release (not a dev build).
func IsRelease(version string) bool {
	_, ok := ParseVersion(version)
	return ok
}

// versionLess reports a < b; false if either is not a release version.
func versionLess(a, b string) bool {
	va, ok1 := ParseVersion(a)
	vb, ok2 := ParseVersion(b)
	if !ok1 || !ok2 {
		return false
	}
	for i := range va {
		if va[i] != vb[i] {
			return va[i] < vb[i]
		}
	}
	return false
}

// CheckUpdate compares the running version with the server's answer.
// Development builds never get updates.
func CheckUpdate(current string, v api.ClientVersion) (Update, bool) {
	if !IsRelease(current) {
		return Update{}, false
	}
	u := Update{Latest: v.Latest, URL: v.URL, Required: versionLess(current, v.Min)}
	if u.URL == "" {
		u.URL = ProjectURL + "/releases/latest"
	}
	return u, u.Required || versionLess(current, v.Latest)
}

// UpdateSeen returns the version last announced with a notification.
func UpdateSeen(path string) string {
	b, _ := os.ReadFile(path)
	return strings.TrimSpace(string(b))
}

// SetUpdateSeen remembers that version has been announced.
func SetUpdateSeen(path, version string) error {
	return os.WriteFile(path, []byte(version+"\n"), 0o600)
}

// UpdateMessage is the notification text for u.
func UpdateMessage(u Update, current string) (title, body string) {
	title = "Gachi Streak: нова версія"
	if u.Latest != "" {
		title += " v" + u.Latest
	}
	body = "У тебе v" + current + ". Натисни, щоб завантажити."
	if u.Required {
		body = "Твоя v" + current + " більше не підтримується сервером — онови клієнт."
	}
	return title, body
}

// UpdateMenuTitle is the tray menu line for u.
func UpdateMenuTitle(u Update, current string) string {
	latest := ""
	if u.Latest != "" {
		latest = " v" + u.Latest
	}
	if u.Required {
		return "⛔ Онови клієнт" + latest + " (у тебе v" + current + " — застаріла)"
	}
	return "⬆️ Доступна нова версія" + latest + " (у тебе v" + current + ")"
}
