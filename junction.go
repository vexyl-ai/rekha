package rekha

import (
	"github.com/vexyl-ai/rekha/internal/geom"
	"github.com/vexyl-ai/rekha/internal/planar"
)

// maxFlankShift bounds the coverage correction of a flank line, in
// pixels: a label boundary is at most half a pixel from the edge it
// quantised, whichever way the half-covered pixels went.
const maxFlankShift = 0.5

// share is the coverage of palette colour c the segmentation measured in
// pixel (x, y): its blend when c is the pixel's label, the complement
// when c is its second colour, else none. Outside the image it is none.
func (s *Segmentation) share(c uint8, x, y int) float64 {
	if x < 0 || y < 0 || x >= s.Width || y >= s.Height {
		return 0
	}
	p := y*s.Width + x
	switch {
	case s.Labels[p] == c:
		return float64(s.Blend[p]) / 255
	case s.Second[p] == c:
		return 1 - float64(s.Blend[p])/255
	}
	return 0
}

// shiftFlanks moves every flank line off the crack vertices onto the
// edge the image shows. Quantising anti-aliased pixels to labels puts the
// label boundary up to half a pixel from the true edge (the half-covered
// pixels all fall one way), and the crack vertices follow the labels.
// The coverage the segmentation measured says how much area the arc's
// right colour has beyond the crack boundary in the pixels along the
// flank; divided by the flank's length that is the line's offset. The
// pixels round the junction itself are left out: their coverage belongs
// to the other arcs too.
func (s *Segmentation) shiftFlanks(pm *planar.Map, preps []arcPrep) {
	for i, a := range pm.Arcs {
		if a.Closed {
			continue
		}
		n := len(a.Pts)
		for e := 0; e < 2; e++ {
			f := &preps[i].flank[e]
			span := min(f.span, n-1)
			if span < 2 {
				continue
			}
			at, step := 0, 1
			if e == 1 {
				at, step = n-1, -1
			}
			junction := a.Pts[at]
			nearJunction := func(p geom.Point) bool {
				return (p.X == junction.X || p.X == junction.X-1) && (p.Y == junction.Y || p.Y == junction.Y-1)
			}
			seen := map[geom.Point]bool{}
			excess := 0.0
			for k := 0; k < span; k++ {
				// Edges in the arc's own direction, so right stays right.
				p, q := a.Pts[at+k*step], a.Pts[at+(k+1)*step]
				if e == 1 {
					p, q = q, p
				}
				right, left := sidePixels(p, q)
				for _, c := range []struct {
					p    geom.Point
					hard float64
				}{{right, 1}, {left, 0}} {
					if seen[c.p] || nearJunction(c.p) {
						continue
					}
					seen[c.p] = true
					excess += s.share(a.Right, c.p.X, c.p.Y) - c.hard
				}
			}
			far := a.Pts[at+span*step].Vec()
			length := far.Sub(junction.Vec()).Dot(f.d)
			if length < 0 {
				length = -length
			}
			length = max(length-1, 1) // the junction pixel is not counted
			shift := max(-maxFlankShift, min(maxFlankShift, excess/length))
			// Left of the walking direction is where the right colour's
			// extra area lies when the excess is positive.
			f.p = f.p.Add(geom.Vec2{X: f.d.Y, Y: -f.d.X}.Scale(shift))
		}
	}
}

// sidePixels returns the pixels right and left of the directed crack
// edge a→b as planar's walker sees them (y down).
func sidePixels(a, b geom.Point) (right, left geom.Point) {
	switch {
	case b.X > a.X: // east
		return geom.Point{X: a.X, Y: a.Y}, geom.Point{X: a.X, Y: a.Y - 1}
	case b.X < a.X: // west
		return geom.Point{X: b.X, Y: b.Y - 1}, geom.Point{X: b.X, Y: b.Y}
	case b.Y > a.Y: // south
		return geom.Point{X: a.X - 1, Y: a.Y}, geom.Point{X: a.X, Y: a.Y}
	default: // north
		return geom.Point{X: a.X, Y: a.Y - 1}, geom.Point{X: a.X - 1, Y: a.Y - 1}
	}
}

// junctionPositions places every junction at the point nearest, in the
// least-squares sense, to the flank lines of the arcs ending there: a
// straight boundary then meets its neighbours where its own line does,
// not at the crack vertex up to 0.7 px away. A small pull towards the
// crack vertex settles the direction parallel lines leave free, and a
// junction that would move more than cornerMaxMove stays put.
func junctionPositions(pm *planar.Map, preps []arcPrep) map[geom.Point]geom.Vec2 {
	type normal struct{ a, b, c, dx, dy float64 } // [[a b] [b c]]·x = (dx, dy)
	sums := map[geom.Point]*normal{}
	add := func(at geom.Point, f flank) {
		s := sums[at]
		if s == nil {
			s = &normal{}
			sums[at] = s
		}
		nx, ny := -f.d.Y, f.d.X
		rhs := float64(nx*f.p.X) + float64(ny*f.p.Y)
		s.a += float64(nx * nx)
		s.b += float64(nx * ny)
		s.c += float64(ny * ny)
		s.dx += float64(nx * rhs)
		s.dy += float64(ny * rhs)
	}
	for i, a := range pm.Arcs {
		if a.Closed {
			continue
		}
		add(a.Pts[0], preps[i].flank[0])
		add(a.Pts[len(a.Pts)-1], preps[i].flank[1])
	}
	const pull = 1e-3
	out := make(map[geom.Point]geom.Vec2, len(sums))
	for at, s := range sums {
		v := at.Vec()
		a, b, c := s.a+pull, s.b, s.c+pull
		dx, dy := s.dx+float64(pull*v.X), s.dy+float64(pull*v.Y)
		det := float64(a*c) - float64(b*b)
		x := geom.Vec2{X: (float64(c*dx) - float64(b*dy)) / det, Y: (float64(a*dy) - float64(b*dx)) / det}
		if d := x.Sub(v).Len(); !(d <= cornerMaxMove) {
			x = v
		}
		out[at] = x
	}
	return out
}
