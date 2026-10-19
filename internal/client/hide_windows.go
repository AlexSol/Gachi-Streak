//go:build windows

package client

import (
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000

// hideWindow stops git.exe from flashing a console window from a GUI app.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}
