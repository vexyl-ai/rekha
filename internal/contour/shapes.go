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

	// Bounding boxes reject almost every outer before the polygon test.
	boxes := make([]box, len(outers))
	for i := range outers {
		boxes[i] = bounds(outers[i].Outer.Pts)
	}
	for _, h := range holes {
		px, py := interiorPixel(h)
		for i := range outers {
			if boxes[i].hasPixel(px, py) && containsPixel(outers[i].Outer.Pts, px, py) {
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

// box is a half-open lattice rectangle [X0, X1) × [Y0, Y1).
type box struct{ X0, Y0, X1, Y1 int }

func bounds(poly []geom.Point) box {
	b := box{poly[0].X, poly[0].Y, poly[0].X, poly[0].Y}
	for _, p := range poly[1:] {
		b.X0, b.X1 = min(b.X0, p.X), max(b.X1, p.X)
		b.Y0, b.Y1 = min(b.Y0, p.Y), max(b.Y1, p.Y)
	}
	return b
}

// hasPixel reports whether pixel (px, py) lies inside the box.
func (b box) hasPixel(px, py int) bool {
	return px >= b.X0 && px < b.X1 && py >= b.Y0 && py < b.Y1
}

// containsPixel tests the centre of pixel (px, py) against a crack polygon
// by casting a ray to the right: only vertical edges can cross it, and a
// vertical edge at x crosses when it spans the centre's row and x lies
// beyond the centre. The centre is never on an edge, so no boundary case
// arises.
func containsPixel(poly []geom.Point, px, py int) bool {
	in := false
	n := len(poly)
	for i, j := 0, n-1; i < n; j, i = i, i+1 {
		a, b := poly[i], poly[j]
		if a.X == b.X && (a.Y > py) != (b.Y > py) && px < a.X {
			in = !in
		}
	}
	return in
}
