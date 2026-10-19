package winapi

// SnapRect moves r (keeping its size) so that any edge closer than dist
// to the matching edge of area sticks to it. dist <= 0 disables snapping.
// Pure geometry, so it is tested on any OS.
func SnapRect(r, area Rect, dist int32) Rect {
	if dist <= 0 {
		return r
	}
	w, h := r.Right-r.Left, r.Bottom-r.Top
	abs := func(v int32) int32 {
		if v < 0 {
			return -v
		}
		return v
	}
	switch {
	case abs(r.Left-area.Left) <= dist:
		r.Left = area.Left
	case abs(r.Right-area.Right) <= dist:
		r.Left = area.Right - w
	}
	switch {
	case abs(r.Top-area.Top) <= dist:
		r.Top = area.Top
	case abs(r.Bottom-area.Bottom) <= dist:
		r.Top = area.Bottom - h
	}
	r.Right, r.Bottom = r.Left+w, r.Top+h
	return r
}

// Point is a screen point in physical pixels.
type Point struct{ X, Y int32 }

// DragRect is where the window would be without any snapping: its rect at
// the start of the drag shifted by how far the cursor has moved since.
// Snapping must be applied to this, not to the previous (already snapped)
// position — otherwise a snapped window can never be pulled away again.
func DragRect(start Rect, startCursor, cursor Point) Rect {
	dx, dy := cursor.X-startCursor.X, cursor.Y-startCursor.Y
	return Rect{start.Left + dx, start.Top + dy, start.Right + dx, start.Bottom + dy}
}
