package refine

import (
	"math"
	"testing"

	"github.com/vexyl-ai/rekha/internal/fit"
	"github.com/vexyl-ai/rekha/internal/geom"
)

// nudgedArc is the arc of (centre, r) from a0 to a1 as n cubics whose
// anchors are moved radially by d, alternately out and in, handles
// along the circle's tangents.
func nudgedArc(centre geom.Vec2, r, a0, a1 float64, n int, d float64) []fit.Segment {
	step := (a1 - a0) / float64(n)
	k := 4.0 / 3 * math.Tan(math.Abs(step)/4) * r
	anchor := func(i int) geom.Vec2 {
		a := a0 + float64(i)*step
		rr := r + d
		if i%2 == 1 {
			rr = r - d
		}
		return v(centre.X+rr*math.Cos(a), centre.Y+rr*math.Sin(a))
	}
	tangent := func(i int) geom.Vec2 {
		a := a0 + float64(i)*step
		t := v(-math.Sin(a), math.Cos(a))
		if step < 0 {
			t = t.Scale(-1)
		}
		return t
	}
	var segs []fit.Segment
	for i := 0; i < n; i++ {
		p0, p1 := anchor(i), anchor(i+1)
		segs = append(segs, cubic(p0, p0.Add(tangent(i).Scale(k)), p1.Sub(tangent(i+1).Scale(k)), p1))
	}
	return segs
}

// halfDisc is the right half of the disc of r = 80 about (128,128): a
// vertical line down its flat side and the arc as four 45° cubics with
// anchors nudged by d.
func halfDisc(d float64) Bezigon {
	segs := []fit.Segment{ln(v(128, 208), v(128, 48))}
	segs = append(segs, nudgedArc(v(128, 128), 80, -math.Pi/2, math.Pi/2, 4, d)...)
	return FromSegments(segs)
}

func arcPieceIndex(t *testing.T, ps []piece, r float64) int {
	t.Helper()
	for i, p := range ps {
		if p.kind == pieceArc && math.Abs(p.r-r) <= 0.02*r {
			return i
		}
	}
	t.Fatalf("no arc piece of r≈%v in %s", r, kindsOf(ps))
	return -1
}

func TestArcEndsTangentBothSides(t *testing.T) {
	b, o, ps := loopPieces(quarterTwice(), 0.5, Options{})
	i := arcPieceIndex(t, ps, 20)
	ends := arcEnds(b, o, ps, i, 0.5)
	if ends[0].kind != endTangent || ends[1].kind != endTangent {
		t.Fatalf("ends %+v", ends)
	}
	c, ok := solveArc(ends, 20, ps[i].centre)
	if !ok || !near(c, v(50, 30), 1e-6) {
		t.Fatalf("centre %v ok=%v", c, ok)
	}
}

// TestArcEndsCornerOnLine: the half-disc's arc meets its line at two
// corners; arcEnds offers both as tangents, and snapPiece, finding no
// tangent circle that holds the arc, demotes both to points at the
// anchors' projections.
func TestArcEndsCornerOnLine(t *testing.T) {
	b, o, ps := loopPieces(halfDisc(0.2), 0.5, Options{})
	i := arcPieceIndex(t, ps, 80)
	if ends := arcEnds(b, o, ps, i, 0.5); ends[0].kind != endTangent || ends[1].kind != endTangent {
		t.Fatalf("typed ends %+v", ends)
	}
	sn, ok := snapPiece(b, o, ps, i, ps[i].r, 0, 0.5)
	if !ok {
		t.Fatal("no snap")
	}
	ends := sn.ends
	if ends[0].kind != endFree || ends[1].kind != endFree {
		t.Fatalf("solved ends %+v", ends)
	}
	for _, e := range ends {
		if !near(e.at, v(128, 48), 0.2) && !near(e.at, v(128, 208), 0.2) {
			t.Fatalf("end at %v", e.at)
		}
	}
}

func TestArcEndsFixedIsPoint(t *testing.T) {
	q := quarterTwice()
	fixed := make([]bool, q.N())
	fixed[1] = true
	b, o, ps := loopPieces(q, 0.5, Options{Fixed: fixed})
	i := arcPieceIndex(t, ps, 20)
	ends := arcEnds(b, o, ps, i, 0.5)
	if ends[0].kind != endPoint || ends[0].at != b.Pts[3*ps[i].first] {
		t.Fatalf("fixed end %+v", ends[0])
	}
	if ends[1].kind != endTangent {
		t.Fatalf("free end %+v", ends[1])
	}
}

// TestArcEndsArcNeighbourIsFree: beside an unsnapped arc the end is
// free, and beside a snapped one a point at the anchor.
func TestArcEndsArcNeighbourIsFree(t *testing.T) {
	b, o, ps := loopPieces(filletD(), 0.5, Options{})
	i := arcPieceIndex(t, ps, 100)
	j := arcPieceIndex(t, ps, 12)
	anchor := b.Pts[3*((ps[i].first+ps[i].count)%b.N())]
	if ends := arcEnds(b, o, ps, i, 0.5); ends[1].kind != endFree || ends[1].at != anchor {
		t.Fatalf("end beside the fillet %+v", ends[1])
	}
	ps[j].snapped = true
	if ends := arcEnds(b, o, ps, i, 0.5); ends[1].kind != endPoint || ends[1].at != anchor {
		t.Fatalf("end beside the snapped fillet %+v", ends[1])
	}
}

func TestSolveArcFreeEnds(t *testing.T) {
	kasa := v(50, 30)
	// Both free: the Kåsa centre itself.
	if c, ok := solveArc([2]arcEnd{{kind: endFree}, {kind: endFree}}, 20, kasa); !ok || c != kasa {
		t.Fatalf("free/free %v %v", c, ok)
	}
	// Through (50,10) with the other end free: the centre nearest near
	// on the circle of r = 20 about the point.
	if c, ok := solveArc([2]arcEnd{{kind: endPoint, at: v(50, 10)}, {kind: endFree}}, 20, v(50, 33)); !ok || !near(c, v(50, 30), 1e-9) {
		t.Fatalf("point/free %v %v", c, ok)
	}
	// Tangent to the line y = 10 with the other end free: the centre
	// nearest near on the line y = 30.
	if c, ok := solveArc([2]arcEnd{{kind: endFree}, {kind: endTangent, at: v(50, 10), far: v(10, 10)}}, 20, v(52, 33)); !ok || !near(c, v(52, 30), 1e-9) {
		t.Fatalf("free/tangent %v %v", c, ok)
	}
}

func TestSolveArcChordBeyondDiameter(t *testing.T) {
	ends := [2]arcEnd{{kind: endPoint, at: v(0, 0)}, {kind: endPoint, at: v(100, 0)}}
	if _, ok := solveArc(ends, 40, v(50, 10)); ok {
		t.Fatal("a chord of 100 fits no circle of r = 40")
	}
}

// TestArcEndsPointBesideFrozenSegment: a frozen segment's control points
// never move, so an arc end beside one is a point, whatever the
// neighbour's kind (the half-disc's flat side is frozen here; a tangent
// end would re-lay it to the foot).
func TestArcEndsPointBesideFrozenSegment(t *testing.T) {
	b, o, ps := loopPieces(halfDisc(0.2), 0.5, Options{Frozen: []bool{true, false, false, false, false}})
	i := arcPieceIndex(t, ps, 80)
	for side, e := range arcEnds(b, o, ps, i, 0.5) {
		if e.kind != endPoint {
			t.Errorf("side %d typed %d, want a point beside the frozen line", side, e.kind)
		}
	}
}
