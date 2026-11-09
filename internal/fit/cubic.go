// Package fit turns dense boundary points into cubic Bézier segments using
// the least-squares scheme described by Schneider in Graphics Gems (1990).
package fit

import "github.com/vexyl-ai/rekha/internal/geom"

// Cubic is a cubic Bézier: P0 start, P1 and P2 handles, P3 end.
type Cubic [4]geom.Vec2

// At evaluates the curve at t in [0,1].
func (c Cubic) At(t float64) geom.Vec2 {
	mt := 1 - t
	a := mt * mt * mt
	b := 3 * mt * mt * t
	cc := 3 * mt * t * t
	d := t * t * t
	return geom.Vec2{
		X: a*c[0].X + b*c[1].X + cc*c[2].X + d*c[3].X,
		Y: a*c[0].Y + b*c[1].Y + cc*c[2].Y + d*c[3].Y,
	}
}

// d1 is the first derivative at t.
func (c Cubic) d1(t float64) geom.Vec2 {
	mt := 1 - t
	return c[1].Sub(c[0]).Scale(3 * mt * mt).
		Add(c[2].Sub(c[1]).Scale(6 * mt * t)).
		Add(c[3].Sub(c[2]).Scale(3 * t * t))
}

// d2 is the second derivative at t.
func (c Cubic) d2(t float64) geom.Vec2 {
	mt := 1 - t
	return c[2].Sub(c[1].Scale(2)).Add(c[0]).Scale(6 * mt).
		Add(c[3].Sub(c[2].Scale(2)).Add(c[1]).Scale(6 * t))
}

// Segment is one piece of a fitted path. When Line is true the segment is
// the straight line P[0]→P[3] and P[1], P[2] are unused.
type Segment struct {
	Line bool
	P    Cubic
}

func line(a, b geom.Vec2) Segment { return Segment{Line: true, P: Cubic{a, a, b, b}} }
