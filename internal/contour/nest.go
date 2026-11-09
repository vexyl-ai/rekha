package contour

import "sort"

// Nest returns, for each shape, the index of its parent: the smallest shape
// of larger area whose outer contour contains the shape's first interior
// pixel, or -1 when no shape encloses it. Shapes may come from different
// colours, so a parent is typically a shape whose hole the child fills.
func Nest(shapes []Shape) []int {
	n := len(shapes)
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return shapes[order[a]].Outer.Area2 < shapes[order[b]].Outer.Area2 })

	boxes := make([]box, n)
	for i := range shapes {
		boxes[i] = bounds(shapes[i].Outer.Pts)
	}
	parent := make([]int, n)
	for k, i := range order {
		parent[i] = -1
		px, py := ownPixel(shapes[i].Outer)
		for _, j := range order[k+1:] {
			if shapes[j].Outer.Area2 > shapes[i].Outer.Area2 && boxes[j].hasPixel(px, py) && containsPixel(shapes[j].Outer.Pts, px, py) {
				parent[i] = j
				break
			}
		}
	}
	return parent
}

// ownPixel returns a foreground pixel of the outer contour c: the pixel on
// the right of its first edge.
func ownPixel(c Contour) (int, int) {
	a, b := c.Pts[0], c.Pts[1%len(c.Pts)]
	switch {
	case b.X > a.X: // east: right is below
		return a.X, a.Y
	case b.Y > a.Y: // south: right is west
		return a.X - 1, a.Y
	case b.X < a.X: // west: right is above
		return a.X - 1, a.Y - 1
	default: // north: right is east
		return a.X, a.Y - 1
	}
}
