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

func TestArcEndsCornerOnLine(t *testing.T) {
	b, o, ps := loopPieces(halfDisc(0.2), 0.5, Options{})
	i := arcPieceIndex(t, ps, 80)
	ends := arcEnds(b, o, ps, i, 0.5)
	if ends[0].kind != endPoint || ends[1].kind != endPoint {
		t.Fatalf("ends %+v", ends)
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

func TestArcEndsArcArcMeanProjection(t *testing.T) {
	b, o, ps := loopPieces(filletD(), 0.5, Options{})
	i := arcPieceIndex(t, ps, 100)
	j := arcPieceIndex(t, ps, 12)
	ends := arcEnds(b, o, ps, i, 0.5)
	if ends[1].kind != endPoint {
		t.Fatalf("end beside the fillet %+v", ends[1])
	}
	anchor := b.Pts[3*((ps[i].first+ps[i].count)%b.N())]
	proj := func(p piece) geom.Vec2 {
		return p.centre.Add(anchor.Sub(p.centre).Scale(p.r / anchor.Sub(p.centre).Len()))
	}
	want := proj(ps[i]).Add(proj(ps[j])).Scale(0.5)
	if !near(ends[1].at, want, 1e-9) {
		t.Fatalf("at %v, want %v", ends[1].at, want)
	}
}

func TestSolveArcChordBeyondDiameter(t *testing.T) {
	ends := [2]arcEnd{{kind: endPoint, at: v(0, 0)}, {kind: endPoint, at: v(100, 0)}}
	if _, ok := solveArc(ends, 40, v(50, 10)); ok {
		t.Fatal("a chord of 100 fits no circle of r = 40")
	}
}
