// Package contour walks the boundaries between foreground and background
// pixels along pixel edges ("cracks") and groups the resulting loops into
// shapes with holes.
//
// This is an independent implementation. Turn-policy names follow the
// user-visible vocabulary of common tracers; the algorithm is our own.
package contour

import (
	"sort"

	"github.com/vexyl-ai/rekha/internal/bitmap"
	"github.com/vexyl-ai/rekha/internal/geom"
)

// TurnPolicy decides which way a walk turns at a saddle: a 2×2 block whose
// foreground pixels sit on one diagonal.
type TurnPolicy string

// Turn policies.
const (
	Minority TurnPolicy = "minority" // behave as Black when fg < bg pixels overall, else White
	Majority TurnPolicy = "majority" // the reverse of Minority
	Black    TurnPolicy = "black"    // keep the two diagonal foreground pixels in one shape
	White    TurnPolicy = "white"    // keep the background connected: two shapes
)

// Contour is a closed loop of lattice points along pixel edges. Pts[0] is
// the lowest (y, then x) vertex of the loop and the last point is not
// repeated. Area2 is twice the signed shoelace area: positive for an outer
// boundary, negative for a hole.
type Contour struct {
	Pts   []geom.Point
	Area2 int
}

// Directions, in the order they are scanned at a vertex.
const (
	dirE = iota
	dirS
	dirW
	dirN
)

var step = [4]geom.Point{{X: 1, Y: 0}, {X: 0, Y: 1}, {X: -1, Y: 0}, {X: 0, Y: -1}}

// Edges are directed so that foreground lies on the walker's right. With
// y pointing down that makes outer boundaries clockwise on screen and
// hence positive under geom.Area2, and holes negative.

// outgoing reports whether the edge leaving vertex (x, y) in direction d is
// a boundary edge with foreground on the right.
func outgoing(b *bitmap.Bitmap, x, y, d int) bool {
	nw := b.Get(x-1, y-1)
	ne := b.Get(x, y-1)
	sw := b.Get(x-1, y)
	se := b.Get(x, y)
	switch d {
	case dirE:
		return se && !ne
	case dirS:
		return sw && !se
	case dirW:
		return nw && !sw
	default: // dirN
		return ne && !nw
	}
}

// Trace returns every closed crack loop in b, sorted by Pts[0] (y, then x).
func Trace(b *bitmap.Bitmap, policy TurnPolicy) []Contour {
	turnLeft := resolvePolicy(b, policy)
	return TraceFunc(b, func(_, _ int) bool { return turnLeft })
}

// TraceFunc is Trace with the saddle decision made per vertex: turnLeft
// reports, for the vertex (x, y) of a 2×2 checkerboard, whether the two
// foreground pixels stay connected. Callers tracing one colour of many use
// it to resolve every saddle the same way from each colour's side.
func TraceFunc(b *bitmap.Bitmap, turnLeft func(x, y int) bool) []Contour {
	vw := b.W + 1
	visited := make([]bool, 4*vw*(b.H+1))
	key := func(x, y, d int) int { return 4*(y*vw+x) + d }

	var out []Contour
	for y := 0; y <= b.H; y++ {
		for x := 0; x <= b.W; x++ {
			for d := 0; d < 4; d++ {
				if visited[key(x, y, d)] || !outgoing(b, x, y, d) {
					continue
				}
				pts := walk(b, x, y, d, turnLeft, visited, key)
				out = append(out, Contour{Pts: pts, Area2: geom.Area2(pts)})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return less(out[i].Pts[0], out[j].Pts[0]) })
	return out
}

func less(a, b geom.Point) bool {
	if a.Y != b.Y {
		return a.Y < b.Y
	}
	return a.X < b.X
}

// resolvePolicy returns whether a saddle is resolved by turning left
// (foreground stays connected) or right (background stays connected).
func resolvePolicy(b *bitmap.Bitmap, p TurnPolicy) bool {
	switch p {
	case Black:
		return true
	case White:
		return false
	case Majority:
		return !(b.Count() < b.W*b.H-b.Count())
	default: // Minority
		return b.Count() < b.W*b.H-b.Count()
	}
}

func walk(b *bitmap.Bitmap, x, y, d int, turnLeft func(x, y int) bool, visited []bool, key func(x, y, d int) int) []geom.Point {
	var pts []geom.Point
	for {
		visited[key(x, y, d)] = true
		pts = append(pts, geom.Point{X: x, Y: y})
		x, y = x+step[d].X, y+step[d].Y

		// Candidate exits: straight, left, right (never back).
		left, right := (d+3)%4, (d+1)%4
		hasL, hasR, hasS := outgoing(b, x, y, left), outgoing(b, x, y, right), outgoing(b, x, y, d)
		switch {
		case hasL && hasR:
			if turnLeft(x, y) {
				d = left
			} else {
				d = right
			}
		case hasL:
			d = left
		case hasR:
			d = right
		case hasS:
			// d unchanged
		default:
			return pts // cannot happen on a consistent bitmap; guard anyway
		}
		if visited[key(x, y, d)] {
			return pts
		}
	}
}
