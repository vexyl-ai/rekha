package fit

import (
	"math"

	"github.com/vexyl-ai/rekha/internal/geom"
)

// FitLine fits a straight line to pts by total least squares and returns
// a point on it (the centroid) and its unit direction, oriented from the
// first point towards the last.
func FitLine(pts []geom.Vec2) (p, dir geom.Vec2) {
	var mean geom.Vec2
	for _, q := range pts {
		mean = mean.Add(q)
	}
	mean = mean.Scale(1 / float64(len(pts)))
	return mean, lineDirection(pts)
}

// Intersect returns the intersection of the lines p1+s·d1 and p2+t·d2.
// ok is false when the lines are parallel.
func Intersect(p1, d1, p2, d2 geom.Vec2) (geom.Vec2, bool) {
	den := geom.Cross(d1, d2)
	if math.Abs(den) < 1e-9 {
		return geom.Vec2{}, false
	}
	w := p2.Sub(p1)
	s := geom.Cross(w, d2) / den
	return p1.Add(d1.Scale(s)), true
}

// smoothPoint returns the centroid of the points within tangentWindow of
// index i, wrapping when closed. Crack vertices sit up to half a pixel off
// the edge they sample; their local mean does not.
func smoothPoint(pts []geom.Vec2, i int, closed bool) geom.Vec2 {
	n := len(pts)
	k := min(tangentWindow/2, n/2)
	if !closed {
		k = min(k, i, n-1-i)
	}
	var sum geom.Vec2
	cnt := 0
	for d := -k; d <= k; d++ {
		j := i + d
		if closed {
			j = ((j % n) + n) % n
		}
		sum = sum.Add(pts[j])
		cnt++
	}
	return sum.Scale(1 / float64(cnt))
}
