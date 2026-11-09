package geom

// RDP simplifies an open polyline with the Ramer–Douglas–Peucker
// algorithm. The first and last points are always kept.
func RDP(pts []Vec2, tol float64) []Vec2 {
	return pickVec(pts, RDPIndices(pts, tol))
}

// RDPIndices returns, in ascending order, the indices of the points RDP
// keeps.
func RDPIndices(pts []Vec2, tol float64) []int {
	out := make([]int, 0, len(pts))
	if len(pts) <= 2 {
		for i := range pts {
			out = append(out, i)
		}
		return out
	}
	keep := make([]bool, len(pts))
	keep[0], keep[len(pts)-1] = true, true
	rdpMark(pts, 0, len(pts)-1, tol, keep)
	for i, k := range keep {
		if k {
			out = append(out, i)
		}
	}
	return out
}

func pickVec(pts []Vec2, idx []int) []Vec2 {
	out := make([]Vec2, len(idx))
	for k, i := range idx {
		out[k] = pts[i]
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
// polylines and rejoins them without duplicating the split points. A
// polygon that would simplify to fewer than three vertices (a sliver no
// wider than tol) keeps every non-collinear vertex instead, so it never
// collapses to a zero-area chord.
func RDPClosed(pts []Vec2, tol float64) []Vec2 {
	return pickVec(pts, RDPClosedIndices(pts, tol))
}

// RDPClosedIndices returns, in ascending order, the indices of the
// vertices RDPClosed keeps.
func RDPClosedIndices(pts []Vec2, tol float64) []int {
	n := len(pts)
	if n <= 3 {
		return RDPIndices(pts, -1)
	}
	far, farD := 0, -1.0
	for i := 1; i < n; i++ {
		if d := pts[i].Sub(pts[0]).Len(); d > farD {
			far, farD = i, d
		}
	}
	a := RDPIndices(pts[:far+1], tol)
	b := RDPIndices(append(append([]Vec2(nil), pts[far:]...), pts[0]), tol)
	out := make([]int, 0, len(a)+len(b))
	out = append(out, a...)
	for _, i := range b[1 : len(b)-1] {
		out = append(out, far+i)
	}
	if len(out) < 3 && tol > 0 {
		return RDPClosedIndices(pts, 0)
	}
	return out
}
