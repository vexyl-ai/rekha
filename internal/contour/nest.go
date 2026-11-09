package contour

import (
	"sort"

	"github.com/vexyl-ai/rekha/internal/geom"
)

// Nest returns, for each shape, the index of its parent: the smallest shape
// of larger area whose outer contour contains the shape's first interior
// pixel, or -1 when no shape encloses it. Shapes may come from different
// colours, so a parent is typically a shape whose hole the child fills.
// w and h are the image size the shapes were traced on.
func Nest(shapes []Shape, w, h int) []int {
	parents, _ := NestRaster(shapes, w, h)
	return parents
}

// NestRaster is Nest and also returns, per pixel, the index of the
// innermost shape whose outer contour covers it (-1 for none). Outers are
// filled largest first, so when a shape is reached the raster holds, at
// its own pixel, the smallest enclosing shape: its parent. Cost is the
// summed area of the outers, that is pixels times nesting depth.
func NestRaster(shapes []Shape, w, h int) (parents []int, ids []int32) {
	n := len(shapes)
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return shapes[order[a]].Outer.Area2 > shapes[order[b]].Outer.Area2 })
	ids = make([]int32, w*h)
	for i := range ids {
		ids[i] = -1
	}
	parents = make([]int, n)
	for _, i := range order {
		parents[i] = -1
		if px, py := ownPixel(shapes[i].Outer); px >= 0 && py >= 0 && px < w && py < h {
			parents[i] = int(ids[py*w+px])
		}
		FillOuter(shapes[i].Outer.Pts, w, h, func(x, y int) { ids[y*w+x] = int32(i) })
	}
	return parents, ids
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

// FillOuter calls set for every pixel inside the crack polygon pts under
// the even-odd rule, by bucketing its vertical edges per row.
func FillOuter(pts []geom.Point, w, h int, set func(x, y int)) {
	y0, y1 := h, 0
	for _, p := range pts {
		y0, y1 = min(y0, p.Y), max(y1, p.Y)
	}
	y0, y1 = max(y0, 0), min(y1, h)
	if y0 >= y1 {
		return
	}
	rows := make([][]int, y1-y0)
	for i, a := range pts {
		b := pts[(i+1)%len(pts)]
		if a.X != b.X {
			continue
		}
		lo, hi := max(min(a.Y, b.Y), y0), min(max(a.Y, b.Y), y1)
		for y := lo; y < hi; y++ {
			rows[y-y0] = append(rows[y-y0], a.X)
		}
	}
	for i, xs := range rows {
		sort.Ints(xs)
		for k := 0; k+1 < len(xs); k += 2 {
			for x := max(xs[k], 0); x < min(xs[k+1], w); x++ {
				set(x, y0+i)
			}
		}
	}
}
