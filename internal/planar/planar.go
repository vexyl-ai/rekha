// Package planar builds the shared-boundary map of a label image: the
// crack loops of every colour, cut at the vertices where three colours
// meet (or two meet diagonally) into arcs, each arc shared by the two
// regions on its sides. Fitting an arc once and giving both regions the
// same curve is what keeps cutout output free of seams.
package planar

import (
	"sort"

	"github.com/vexyl-ai/rekha/internal/bitmap"
	"github.com/vexyl-ai/rekha/internal/contour"
	"github.com/vexyl-ai/rekha/internal/geom"
)

// None is the label of the outside of the image.
const None uint8 = 255

// Arc is a run of crack edges between two junctions, shared by the
// regions on its two sides. Pts runs in the direction that keeps Right on
// the walker's right (y down); a Closed arc is an island loop with no
// junction, whose first and last points are not repeated.
type Arc struct {
	Pts    []geom.Point
	Left   uint8 // label on the walker's left; None outside the image
	Right  uint8 // label on the walker's right: the colour that walked it forward
	Closed bool
}

// Ref is one arc of a loop, walked backwards when Reverse is set.
type Ref struct {
	Arc     int
	Reverse bool
}

// Loop is one crack loop of one colour, as contour.TraceFunc produces it,
// expressed as arcs.
type Loop struct {
	Colour uint8
	Area2  int
	Refs   []Ref
	// Shift is the index, in the traced loop, of the junction the first
	// arc starts at; Assemble rotates back so the loop begins where
	// TraceFunc began it.
	Shift int
}

// Map holds the arcs and the loops that reference them. Loops are in the
// order Segmentation.layers traces them: colour, then (y, x) of the
// loop's first point.
type Map struct {
	Arcs  []Arc
	Loops []Loop
}

// Build traces every colour in colours from labels (w×h) with the saddle
// rule saddle(c) and cuts the loops into shared arcs.
func Build(labels []uint8, w, h int, colours []uint8, saddle func(c uint8) func(x, y int) bool) Map {
	at := func(x, y int) uint8 {
		if x < 0 || y < 0 || x >= w || y >= h {
			return None
		}
		return labels[y*w+x]
	}
	// junction reports whether the crack vertex (x, y) ends arcs: three
	// or more labels around it, or two in the diagonal (saddle) pattern.
	junction := func(x, y int) bool {
		nw, ne, sw, se := at(x-1, y-1), at(x, y-1), at(x-1, y), at(x, y)
		set := [4]uint8{nw, ne, sw, se}
		distinct := 0
		for i, a := range set {
			dup := false
			for j := 0; j < i; j++ {
				if set[j] == a {
					dup = true
				}
			}
			if !dup {
				distinct++
			}
		}
		if distinct >= 3 {
			return true
		}
		return distinct == 2 && nw == se && ne == sw
	}
	// leftOf returns the label on the left of the directed edge a→b.
	leftOf := func(a, b geom.Point) uint8 {
		switch {
		case b.X > a.X: // east: left is the pixel above
			return at(a.X, a.Y-1)
		case b.X < a.X: // west: left is the pixel below
			return at(b.X, b.Y)
		case b.Y > a.Y: // south: left is the pixel to the right
			return at(a.X, a.Y)
		default: // north: left is the pixel to the west of the edge
			return at(a.X-1, a.Y-1)
		}
	}
	vw := w + 1
	edgeID := func(a, b geom.Point) int {
		if b.X < a.X || b.Y < a.Y {
			a, b = b, a
		}
		id := 2 * (a.Y*vw + a.X)
		if b.Y > a.Y {
			id++
		}
		return id
	}

	var m Map
	type key struct{ lo, hi, n int }
	index := map[key]int{}
	bm := bitmap.New(w, h)
	for _, c := range colours {
		bm.Clear()
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				if labels[y*w+x] == c {
					bm.Set(x, y, true)
				}
			}
		}
		for _, loop := range contour.TraceFunc(bm, saddle(c)) {
			pts := loop.Pts
			n := len(pts)
			var cuts []int
			for i, p := range pts {
				if junction(p.X, p.Y) {
					cuts = append(cuts, i)
				}
			}
			l := Loop{Colour: c, Area2: loop.Area2}
			if len(cuts) == 0 {
				lo, hi := -1, -1
				for i := range pts {
					id := edgeID(pts[i], pts[(i+1)%n])
					if lo < 0 || id < lo {
						lo = id
					}
					if id > hi {
						hi = id
					}
				}
				k := key{lo, hi, n}
				if j, ok := index[k]; ok {
					l.Refs = append(l.Refs, Ref{Arc: j, Reverse: true})
				} else {
					index[k] = len(m.Arcs)
					l.Refs = append(l.Refs, Ref{Arc: len(m.Arcs)})
					m.Arcs = append(m.Arcs, Arc{Pts: append([]geom.Point(nil), pts...), Left: leftOf(pts[0], pts[1%n]), Right: c, Closed: true})
				}
				m.Loops = append(m.Loops, l)
				continue
			}
			// Arcs run from each junction to the next, the last one
			// wrapping round to the first; the loop remembers where its
			// traced start sits.
			l.Shift = cuts[0]
			for k := range cuts {
				from := cuts[k]
				to := cuts[0] + n
				if k+1 < len(cuts) {
					to = cuts[k+1]
				}
				// Arc points from..to inclusive, wrapping to index 0 at n.
				arc := make([]geom.Point, 0, to-from+1)
				for i := from; i <= to; i++ {
					arc = append(arc, pts[i%n])
				}
				if len(arc) < 2 {
					continue
				}
				lo, hi := -1, -1
				for i := 0; i+1 < len(arc); i++ {
					id := edgeID(arc[i], arc[i+1])
					if lo < 0 || id < lo {
						lo = id
					}
					if id > hi {
						hi = id
					}
				}
				kk := key{lo, hi, len(arc) - 1}
				if j, ok := index[kk]; ok {
					// The partner walks the same edges the other way: it
					// starts where this arc ends and its second point is
					// this arc's second-to-last (an arc from a junction back
					// to itself has equal ends, so the first point alone
					// cannot tell).
					q := m.Arcs[j].Pts
					l.Refs = append(l.Refs, Ref{Arc: j, Reverse: arc[0] == q[len(q)-1] && arc[1] == q[len(q)-2]})
				} else {
					index[kk] = len(m.Arcs)
					l.Refs = append(l.Refs, Ref{Arc: len(m.Arcs)})
					m.Arcs = append(m.Arcs, Arc{Pts: arc, Left: leftOf(arc[0], arc[1]), Right: c})
				}
			}
			m.Loops = append(m.Loops, l)
		}
	}
	return m
}

// Assemble returns the loop's crack polygon: its arcs concatenated,
// reversed where marked, without repeating the shared junctions. The
// result equals the contour TraceFunc produced for the loop.
func (m Map) Assemble(l Loop) []geom.Point {
	var out []geom.Point
	for k, r := range l.Refs {
		a := m.Arcs[r.Arc]
		pts := a.Pts
		if a.Closed {
			if !r.Reverse {
				return append(out, pts...)
			}
			// The other side starts at the same lowest vertex and walks
			// the other way round.
			out = append(out, pts[0])
			for i := len(pts) - 1; i >= 1; i-- {
				out = append(out, pts[i])
			}
			return out
		}
		if r.Reverse {
			for i := len(pts) - 1; i >= 0; i-- {
				if i == len(pts)-1 && k > 0 {
					continue
				}
				out = append(out, pts[i])
			}
			continue
		}
		start := 0
		if k > 0 {
			start = 1
		}
		out = append(out, pts[start:]...)
	}
	// The last arc ends where the first began.
	if n := len(out); n > 1 && out[n-1] == out[0] {
		out = out[:n-1]
	}
	if n := len(out); l.Shift > 0 && n > 0 {
		k := (n - l.Shift) % n
		out = append(out[k:], out[:k]...)
	}
	return out
}

// Shapes groups the loops into shapes as contour.BuildShapes does for one
// colour's loops: per shape, the outer loop's index followed by its
// holes', in BuildShapes order (colour, then the outer's first point).
func (m Map) Shapes() [][]int {
	var out [][]int
	byColour := map[uint8][]int{}
	var colours []uint8
	for i, l := range m.Loops {
		if _, ok := byColour[l.Colour]; !ok {
			colours = append(colours, l.Colour)
		}
		byColour[l.Colour] = append(byColour[l.Colour], i)
	}
	sort.Slice(colours, func(a, b int) bool { return colours[a] < colours[b] })
	for _, c := range colours {
		idx := byColour[c]
		cs := make([]contour.Contour, len(idx))
		for k, i := range idx {
			cs[k] = contour.Contour{Pts: m.Assemble(m.Loops[i]), Area2: m.Loops[i].Area2}
		}
		// Identify contours by the backing array of their points.
		byPts := make(map[*geom.Point]int, len(cs))
		for k := range cs {
			if len(cs[k].Pts) > 0 {
				byPts[&cs[k].Pts[0]] = idx[k]
			}
		}
		which := func(c contour.Contour) int {
			if len(c.Pts) == 0 {
				return -1
			}
			if i, ok := byPts[&c.Pts[0]]; ok {
				return i
			}
			return -1
		}
		for _, sh := range contour.BuildShapes(cs, 0) {
			s := []int{which(sh.Outer)}
			for _, h := range sh.Holes {
				s = append(s, which(h))
			}
			out = append(out, s)
		}
	}
	return out
}
