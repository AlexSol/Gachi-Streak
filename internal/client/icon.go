package client

import (
	"bytes"
	"encoding/binary"
	"image/color"
)

// Tray icon colors per server state.
var stateColors = map[string]color.RGBA{
	"safe":    {46, 204, 64, 255},   // green
	"pending": {170, 170, 170, 255}, // gray
	"warning": {255, 140, 0, 255},   // orange
	"failed":  {220, 20, 20, 255},   // red
	"offline": {90, 90, 90, 255},    // dark gray
}

// StateIcon returns a .ico (32x32, 32bpp BMP-encoded) with a filled circle
// in the color of the state. Generated in code — no binary assets needed.
func StateIcon(state string) []byte {
	c, ok := stateColors[state]
	if !ok {
		c = stateColors["offline"]
	}
	return circleICO(32, c)
}

func circleICO(size int, c color.RGBA) []byte {
	// Pixel data: BGRA, bottom-up rows.
	pixels := make([]byte, 0, size*size*4)
	center := float64(size-1) / 2
	r := float64(size)/2 - 1.5
	for y := size - 1; y >= 0; y-- {
		for x := 0; x < size; x++ {
			dx, dy := float64(x)-center, float64(y)-center
			d := dx*dx + dy*dy
			var a byte
			switch {
			case d <= (r-1)*(r-1):
				a = 255
			case d <= r*r:
				a = 128 // cheap anti-aliasing on the edge
			}
			if d <= (r-4)*(r-4) && d >= (r-6)*(r-6) {
				// thin light ring for contrast on dark taskbars
				pixels = append(pixels, 255, 255, 255, a)
				continue
			}
			pixels = append(pixels, c.B, c.G, c.R, a)
		}
	}
	// AND mask: 1 bit per pixel, rows padded to 32 bits; all zeros (alpha is used).
	maskRow := ((size + 31) / 32) * 4
	mask := make([]byte, maskRow*size)

	var bmp bytes.Buffer
	// BITMAPINFOHEADER (height is doubled: XOR + AND).
	le := binary.LittleEndian
	binary.Write(&bmp, le, uint32(40))
	binary.Write(&bmp, le, int32(size))
	binary.Write(&bmp, le, int32(size*2))
	binary.Write(&bmp, le, uint16(1))  // planes
	binary.Write(&bmp, le, uint16(32)) // bpp
	binary.Write(&bmp, le, uint32(0))  // BI_RGB
	binary.Write(&bmp, le, uint32(len(pixels)+len(mask)))
	binary.Write(&bmp, le, int32(0))
	binary.Write(&bmp, le, int32(0))
	binary.Write(&bmp, le, uint32(0))
	binary.Write(&bmp, le, uint32(0))
	bmp.Write(pixels)
	bmp.Write(mask)

	var ico bytes.Buffer
	// ICONDIR
	binary.Write(&ico, le, uint16(0)) // reserved
	binary.Write(&ico, le, uint16(1)) // type: icon
	binary.Write(&ico, le, uint16(1)) // count
	// ICONDIRENTRY
	ico.WriteByte(byte(size))
	ico.WriteByte(byte(size))
	ico.WriteByte(0)                   // palette
	ico.WriteByte(0)                   // reserved
	binary.Write(&ico, le, uint16(1))  // planes
	binary.Write(&ico, le, uint16(32)) // bpp
	binary.Write(&ico, le, uint32(bmp.Len()))
	binary.Write(&ico, le, uint32(6+16)) // offset
	ico.Write(bmp.Bytes())
	return ico.Bytes()
}
