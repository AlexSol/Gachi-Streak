//go:build !windows

package splash

import (
	"errors"
	"image"
)

// Run is only implemented on Windows.
func Run(kind Kind, area image.Rectangle, from image.Point, scale float64) error {
	return errors.New("splash: screen effects are only supported on Windows")
}
