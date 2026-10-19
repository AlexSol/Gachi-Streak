//go:build !windows

package client

import "os/exec"

func hideWindow(*exec.Cmd) {}
