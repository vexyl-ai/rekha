// Package geom holds the lattice and vector primitives shared by the
// contour, simplification and fitting stages.
package geom

import "math"

// Point is a lattice point. Contour vertices live on pixel corners, so
// they are always integers.
type Point struct{ X, Y int }

// Vec returns p as a float vector.
func (p Point) Vec() Vec2 { return Vec2{float64(p.X), float64(p.Y)} }

// Vec2 is a 2-D float vector.
type Vec2 struct{ X, Y float64 }

// Add returns a+b.
func (a Vec2) Add(b Vec2) Vec2 { return Vec2{a.X + b.X, a.Y + b.Y} }

// Sub returns a-b.
func (a Vec2) Sub(b Vec2) Vec2 { return Vec2{a.X - b.X, a.Y - b.Y} }

// Scale returns a*k.
func (a Vec2) Scale(k float64) Vec2 { return Vec2{a.X * k, a.Y * k} }

// Dot returns a·b.
//
// Products are rounded explicitly before they are summed, here and in the
// other arithmetic helpers, so the compiler cannot fuse them into a
// multiply-add on arm64 and give results that differ in the last bit from
// amd64: tie-breaks on distances and turns must come out the same on every
// architecture.
func (a Vec2) Dot(b Vec2) float64 { return float64(a.X*b.X) + float64(a.Y*b.Y) }

// Cross returns the z component of a × b.
func Cross(a, b Vec2) float64 { return float64(a.X*b.Y) - float64(a.Y*b.X) }

// LenSq returns |a|².
func (a Vec2) LenSq() float64 { return float64(a.X*a.X) + float64(a.Y*a.Y) }

// Len returns |a|.
func (a Vec2) Len() float64 { return math.Sqrt(a.LenSq()) }

// Norm returns a/|a|, or the zero vector when a is zero.
func (a Vec2) Norm() Vec2 {
	l := a.Len()
	if l == 0 {
		return Vec2{}
	}
	return Vec2{a.X / l, a.Y / l}
}

// Area2 returns twice the signed shoelace area of a closed lattice
// polygon. With y pointing down, a clockwise-on-screen polygon is positive.
func Area2(poly []Point) int {
	n := len(poly)
	s := 0
	for i := 0; i < n; i++ {
		a, b := poly[i], poly[(i+1)%n]
		s += a.X*b.Y - b.X*a.Y
	}
	return s
}

// Contains reports whether p is inside the closed polygon by the even-odd
// rule. Points exactly on an edge count as inside.
func Contains(poly []Point, p Point) bool {
	n := len(poly)
	in := false
	for i, j := 0, n-1; i < n; j, i = i, i+1 {
		a, b := poly[i], poly[j]
		if onSegment(p, a, b) {
			return true
		}
		if (a.Y > p.Y) != (b.Y > p.Y) {
			// x of the edge at p.Y, compared without division:
			// p.X < a.X + (p.Y-a.Y)*(b.X-a.X)/(b.Y-a.Y)
			num := (p.Y - a.Y) * (b.X - a.X)
			den := b.Y - a.Y
			lhs := (p.X - a.X) * den
			if den > 0 {
				if lhs < num {
					in = !in
				}
			} else if lhs > num {
				in = !in
			}
		}
	}
	return in
}

func onSegment(p, a, b Point) bool {
	cross := (b.X-a.X)*(p.Y-a.Y) - (b.Y-a.Y)*(p.X-a.X)
	if cross != 0 {
		return false
	}
	return min(a.X, b.X) <= p.X && p.X <= max(a.X, b.X) &&
		min(a.Y, b.Y) <= p.Y && p.Y <= max(a.Y, b.Y)
}

// PerpDist returns the distance from p to the infinite line through a and
// b. When a == b it returns |p-a|.
func PerpDist(p, a, b Vec2) float64 {
	d := b.Sub(a)
	l := d.Len()
	if l == 0 {
		return p.Sub(a).Len()
	}
	v := p.Sub(a)
	return math.Abs(Cross(d, v)) / l
}
