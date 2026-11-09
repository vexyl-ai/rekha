// Package refine moves the control points of fitted curves to match the
// input's anti-aliasing: a bezigon is rasterised by exact area coverage
// and optimised against a soft coverage mask with the priors of Yang et
// al. 2016 (direct bezigon optimisation).
package refine

import (
	"github.com/vexyl-ai/rekha/internal/fit"
	"github.com/vexyl-ai/rekha/internal/geom"
)

// Flatten is the number of uniform-t line segments each cubic is cut into
// for rasterisation and the length and self-intersection priors. It is
// fixed so that moving a control point changes the polygon smoothly.
const Flatten = 16

// Bezigon is a closed chain of N cubic Béziers. Pts has 3N entries: for
// segment j, Pts[3j] is its start anchor, Pts[3j+1] and Pts[3j+2] its
// handles, and Pts[3(j+1) mod 3N] its end anchor.
type Bezigon struct {
	Pts []geom.Vec2
}

// FromSegments builds a bezigon from a closed run of fitted segments. A
// line becomes a cubic with its handles a third of the way along.
func FromSegments(segs []fit.Segment) Bezigon {
	b := Bezigon{Pts: make([]geom.Vec2, 0, 3*len(segs))}
	for _, s := range segs {
		a, d := s.P[0], s.P[3]
		if s.Line {
			b.Pts = append(b.Pts, a, a.Add(d.Sub(a).Scale(1.0/3)), a.Add(d.Sub(a).Scale(2.0/3)))
		} else {
			b.Pts = append(b.Pts, a, s.P[1], s.P[2])
		}
	}
	return b
}

// N is the number of segments.
func (b Bezigon) N() int { return len(b.Pts) / 3 }

// Segment returns cubic j (mod N).
func (b Bezigon) Segment(j int) fit.Cubic {
	n := len(b.Pts)
	i := ((3*j)%n + n) % n
	return fit.Cubic{b.Pts[i], b.Pts[(i+1)%n], b.Pts[(i+2)%n], b.Pts[(i+3)%n]}
}

// Segments converts back to fitted segments. A cubic whose handles both
// lie within lineTol of its chord, between the endpoints, becomes a line.
func (b Bezigon) Segments(lineTol float64) []fit.Segment {
	out := make([]fit.Segment, b.N())
	for j := range out {
		c := b.Segment(j)
		if straightCubic(c, lineTol) {
			out[j] = fit.Segment{Line: true, P: fit.Cubic{c[0], c[0], c[3], c[3]}}
		} else {
			out[j] = fit.Segment{P: c}
		}
	}
	return out
}

func straightCubic(c fit.Cubic, tol float64) bool {
	chord := c[3].Sub(c[0])
	l2 := chord.LenSq()
	if l2 == 0 {
		return true
	}
	for _, h := range c[1:3] {
		u := h.Sub(c[0]).Dot(chord) / l2
		if u < 0 || u > 1 || geom.PerpDist(h, c[0], c[3]) > tol {
			return false
		}
	}
	return true
}

// Polyline flattens the bezigon to Flatten points per segment (t = i/Flatten,
// i < Flatten), closed implicitly: the last point joins the first.
func (b Bezigon) Polyline() []geom.Vec2 {
	return polylineInto(make([]geom.Vec2, 0, b.N()*Flatten), b)
}

// polylineInto appends b's Polyline to out.
func polylineInto(out []geom.Vec2, b Bezigon) []geom.Vec2 {
	for j := 0; j < b.N(); j++ {
		out = flattenInto(out, b.Segment(j))
	}
	return out
}

// flattenInto appends Flatten points of c (excluding its end point).
func flattenInto(out []geom.Vec2, c fit.Cubic) []geom.Vec2 {
	for i := 0; i < Flatten; i++ {
		out = append(out, c.At(float64(i)/Flatten))
	}
	return out
}

// Length is the closed polyline length.
func (b Bezigon) Length() float64 { return polylineLength(b.Polyline(), true) }

func polylineLength(pts []geom.Vec2, closed bool) float64 {
	l := 0.0
	for i := 1; i < len(pts); i++ {
		l += pts[i].Sub(pts[i-1]).Len()
	}
	if closed && len(pts) > 1 {
		l += pts[0].Sub(pts[len(pts)-1]).Len()
	}
	return l
}
