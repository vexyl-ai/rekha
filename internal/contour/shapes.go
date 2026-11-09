package contour

import (
	"sort"

	"github.com/vexyl-ai/rekha/internal/geom"
)

// Shape is an outer contour with the holes it contains.
type Shape struct {
	Outer Contour
	Holes []Contour
}

// BuildShapes assigns each hole to the smallest outer that contains it and
// drops any contour (and, for outers, its holes) whose pixel area is below
// minArea. Shapes are sorted by Outer.Pts[0] (y, then x).
func BuildShapes(cs []Contour, minArea int) []Shape {
	var outers []Shape
	var holes []Contour
	for _, c := range cs {
		if c.Area2 > 0 {
			outers = append(outers, Shape{Outer: c})
		} else if c.Area2 < 0 {
			holes = append(holes, c)
		}
	}
	// Smallest outers first so the first containing outer is the smallest.
	sort.SliceStable(outers, func(i, j int) bool { return outers[i].Outer.Area2 < outers[j].Outer.Area2 })

	for _, h := range holes {
		px, py := interiorPixel(h)
		for i := range outers {
			if containsPixel(outers[i].Outer.Pts, px, py) {
				outers[i].Holes = append(outers[i].Holes, h)
				break
			}
		}
	}

	var out []Shape
	for _, s := range outers {
		if s.Outer.Area2/2 < minArea {
			continue
		}
		kept := s.Holes[:0]
		for _, h := range s.Holes {
			if -h.Area2/2 >= minArea {
				kept = append(kept, h)
			}
		}
		s.Holes = kept
		sort.Slice(s.Holes, func(i, j int) bool { return less(s.Holes[i].Pts[0], s.Holes[j].Pts[0]) })
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return less(out[i].Outer.Pts[0], out[j].Outer.Pts[0]) })
	return out
}

// interiorPixel returns a background pixel just inside the hole: the pixel
// on the left of the hole's first edge (foreground is on the right).
func interiorPixel(h Contour) (int, int) {
	a, b := h.Pts[0], h.Pts[1%len(h.Pts)]
	switch {
	case b.X > a.X: // east: left is above
		return a.X, a.Y - 1
	case b.Y > a.Y: // south: left is east
		return a.X, a.Y
	case b.X < a.X: // west: left is below
		return a.X - 1, a.Y
	default: // north: left is west
		return a.X - 1, a.Y - 1
	}
}

// containsPixel tests the centre of pixel (px, py) against a lattice polygon
// by doubling coordinates so the centre becomes a lattice point.
func containsPixel(poly []geom.Point, px, py int) bool {
	d := make([]geom.Point, len(poly))
	for i, p := range poly {
		d[i] = geom.Point{X: 2 * p.X, Y: 2 * p.Y}
	}
	return geom.Contains(d, geom.Point{X: 2*px + 1, Y: 2*py + 1})
}
