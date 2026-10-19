//go:build !windows

// The desktop agent currently supports Windows only (v1).
// Linux (gsettings) and macOS (osascript) are planned.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "gachi-client: v1 supports Windows only. Build with GOOS=windows.")
	os.Exit(1)
}
