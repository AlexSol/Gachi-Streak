// Package assets embeds the default wallpapers and sounds into the client
// binary. All files are original works generated for this project
// (see assets/README.md), so they can be shipped freely.
package assets

import "embed"

// FS holds images/{warning,failed}/*.jpg and sounds/{warning,failed}/*.wav.
//
//go:embed images sounds
var FS embed.FS
