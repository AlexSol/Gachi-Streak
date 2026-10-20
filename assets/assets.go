// Package assets embeds the default wallpapers and sounds into the client
// binary. All files are original works generated for this project
// (see assets/README.md), so they can be shipped freely.
package assets

import "embed"

// FS holds images/warning/*, sounds/{warning,failed}/*.{wav,mp3} and
// widget/{safe,pending,warning,failed}/* (widget backgrounds, built from
// assets-src/widget by its convert.sh).
//
//go:embed images sounds widget
var FS embed.FS
