package client

import (
	"runtime/debug"
	"time"
)

// ProjectURL is the project's home page.
const ProjectURL = "https://github.com/AlexSol/Gachi-Streak"

// BuildInfo describes this binary.
type BuildInfo struct {
	Version   string    // release version, set at build time (-X main.version)
	Commit    string    // git hash, embedded by `go build` inside the repo
	Dirty     bool      // built from a working tree with uncommitted changes
	Time      time.Time // commit time
	GoVersion string
}

// ReadBuildInfo combines the given version with the VCS data Go stamps
// into binaries built in a git checkout (missing under `go run`).
func ReadBuildInfo(version string) BuildInfo {
	bi := BuildInfo{Version: version}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return bi
	}
	bi.GoVersion = info.GoVersion
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			bi.Commit = s.Value
		case "vcs.modified":
			bi.Dirty = s.Value == "true"
		case "vcs.time":
			bi.Time, _ = time.Parse(time.RFC3339, s.Value)
		}
	}
	return bi
}

// ShortCommit is the 7-char hash plus "-dirty", or "невідомо".
func (b BuildInfo) ShortCommit() string {
	if b.Commit == "" {
		return "невідомо"
	}
	c := b.Commit[:min(7, len(b.Commit))]
	if b.Dirty {
		c += "-dirty"
	}
	return c
}

// String is "v0.1.0 (6d8a8df)", or "dev (6d8a8df)" for a dev build.
func (b BuildInfo) String() string {
	v := b.Version
	if IsRelease(v) {
		v = "v" + v
	}
	return v + " (" + b.ShortCommit() + ")"
}
