package geom

// RDP simplifies an open polyline with the Ramer–Douglas–Peucker
// algorithm. The first and last points are always kept.
func RDP(pts []Vec2, tol float64) []Vec2 {
	if len(pts) <= 2 {
		return append([]Vec2(nil), pts...)
	}
	keep := make([]bool, len(pts))
	keep[0], keep[len(pts)-1] = true, true
	rdpMark(pts, 0, len(pts)-1, tol, keep)
	out := make([]Vec2, 0, len(pts))
	for i, k := range keep {
		if k {
			out = append(out, pts[i])
		}
	}
	return out
}

func rdpMark(pts []Vec2, lo, hi int, tol float64, keep []bool) {
	if hi-lo < 2 {
		return
	}
	best, bestD := -1, tol
	for i := lo + 1; i < hi; i++ {
		if d := PerpDist(pts[i], pts[lo], pts[hi]); d > bestD {
			best, bestD = i, d
		}
	}
	if best < 0 {
		return
	}
	keep[best] = true
	rdpMark(pts, lo, best, tol, keep)
	rdpMark(pts, best, hi, tol, keep)
}

// RDPClosed simplifies a closed polygon. It splits the loop at index 0 and
// at the vertex farthest from pts[0], simplifies both halves as open
// polylines and rejoins them without duplicating the split points.
func RDPClosed(pts []Vec2, tol float64) []Vec2 {
	n := len(pts)
	if n <= 3 {
		return append([]Vec2(nil), pts...)
	}
	far, farD := 0, -1.0
	for i := 1; i < n; i++ {
		if d := pts[i].Sub(pts[0]).Len(); d > farD {
			far, farD = i, d
		}
	}
	a := RDP(pts[:far+1], tol)
	b := RDP(append(append([]Vec2(nil), pts[far:]...), pts[0]), tol)
	out := make([]Vec2, 0, len(a)+len(b))
	out = append(out, a...)
	out = append(out, b[1:len(b)-1]...)
	return out
}
