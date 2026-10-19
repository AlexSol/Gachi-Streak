//go:build windows

package winapi

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"

	"golang.org/x/sys/windows"
)

// toastScript shows a Windows 10/11 toast through WinRT. The texts come
// from environment variables and are XML-escaped there, so nothing the
// server sends is ever spliced into the script. Shown under PowerShell's
// app ID: an unpackaged exe has none of its own to show toasts with.
const toastScript = `
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] | Out-Null
$e = [Security.SecurityElement]
$launch = ''
if ($env:GS_TOAST_LAUNCH) { $launch = ' activationType="protocol" launch="' + $e::Escape($env:GS_TOAST_LAUNCH) + '"' }
$xml = '<toast' + $launch + '><visual><binding template="ToastGeneric"><text>' + $e::Escape($env:GS_TOAST_TITLE) +
  '</text><text>' + $e::Escape($env:GS_TOAST_BODY) + '</text></binding></visual></toast>'
$doc = New-Object Windows.Data.Xml.Dom.XmlDocument
$doc.LoadXml($xml)
$app = '{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe'
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier($app).Show([Windows.UI.Notifications.ToastNotification]::new($doc))
`

// Toast shows a Windows notification; clicking it opens launch (a URL,
// optional). It blocks until PowerShell has handed the toast over (~1 s).
func Toast(title, body, launch string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "powershell.exe",
		"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
		"-EncodedCommand", encodePS(toastScript))
	cmd.Env = append(os.Environ(),
		"GS_TOAST_TITLE="+title, "GS_TOAST_BODY="+body, "GS_TOAST_LAUNCH="+launch)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("toast: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// encodePS encodes a script for -EncodedCommand (base64 of UTF-16LE).
func encodePS(s string) string {
	u := utf16.Encode([]rune(s))
	b := make([]byte, 2*len(u))
	for i, c := range u {
		b[2*i], b[2*i+1] = byte(c), byte(c>>8)
	}
	return base64.StdEncoding.EncodeToString(b)
}
