package geom

import "math"

// Corners returns the indices of vertices of a closed polygon whose
// absolute turn angle exceeds thresholdDeg, in ascending order.
func Corners(poly []Vec2, thresholdDeg float64) []int {
	n := len(poly)
	var out []int
	for i := 0; i < n; i++ {
		prev, cur, next := poly[(i+n-1)%n], poly[i], poly[(i+1)%n]
		a := cur.Sub(prev)
		b := next.Sub(cur)
		cross := a.X*b.Y - a.Y*b.X
		dot := a.Dot(b)
		deg := math.Abs(math.Atan2(cross, dot)) * 180 / math.Pi
		if deg > thresholdDeg {
			out = append(out, i)
		}
	}
	return out
}

// NearestIndex returns the index of the dense point closest to p; ties go
// to the lower index.
func NearestIndex(dense []Point, p Vec2) int {
	best, bestD := 0, math.Inf(1)
	for i, d := range dense {
		dx, dy := float64(d.X)-p.X, float64(d.Y)-p.Y
		if dd := dx*dx + dy*dy; dd < bestD {
			best, bestD = i, dd
		}
	}
	return best
}
