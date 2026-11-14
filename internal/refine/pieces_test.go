package refine

import (
	"math"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/vexyl-ai/rekha/internal/fit"
	"github.com/vexyl-ai/rekha/internal/geom"
)

// filletD is a D whose straight top runs into a quarter circle of r = 100
// that flows smoothly into a fillet of r = 12 and on into the bottom
// line; only its two left corners turn. Segments: line (10,10)→(110,10),
// arc r=100 about (110,110) from −90° to 0°, arc r=12 about (198,110)
// from 0° to 90°, line (198,122)→(10,122), line (10,122)→(10,10).
func filletD() Bezigon {
	return FromSegments([]fit.Segment{
		ln(v(10, 10), v(110, 10)),
		arcSeg(v(110, 110), 100, -math.Pi/2, 0),
		arcSeg(v(198, 110), 12, 0, math.Pi/2),
		ln(v(198, 122), v(10, 122)),
		ln(v(10, 122), v(10, 10)),
	})
}

// arcThenFillet is a 180° arc of r = 80 about (120,100) as four 45°
// cubics, a fillet of r = 8 as two 45° cubics flowing on from it, a line
// up, and a diagonal back to the arc's start; the two joints of the
// diagonal turn 45°.
func arcThenFillet() Bezigon {
	c, f := v(120, 100), v(120, 172)
	return FromSegments([]fit.Segment{
		arcSeg(c, 80, -math.Pi/2, -math.Pi/4), arcSeg(c, 80, -math.Pi/4, 0),
		arcSeg(c, 80, 0, math.Pi/4), arcSeg(c, 80, math.Pi/4, math.Pi/2),
		arcSeg(f, 8, math.Pi/2, 3*math.Pi/4), arcSeg(f, 8, 3*math.Pi/4, math.Pi),
		ln(v(112, 172), v(112, 28)),
		ln(v(112, 28), v(120, 20)),
	})
}

// shallowArc is 10° of a circle of r = 700 as five 2° cubics (each
// within 0.11 px of its chord), closed by two short radial lines and a
// chord; every joint of the closing lines is a corner.
func shallowArc() Bezigon {
	c, r := v(128, 828), 700.0
	a0, a1 := -95*math.Pi/180, -85*math.Pi/180
	var segs []fit.Segment
	for i := 0; i < 5; i++ {
		segs = append(segs, arcSeg(c, r, a0+float64(i)*(a1-a0)/5, a0+float64(i+1)*(a1-a0)/5))
	}
	at := func(a, rr float64) geom.Vec2 { return v(c.X+rr*math.Cos(a), c.Y+rr*math.Sin(a)) }
	segs = append(segs, ln(at(a1, r), at(a1, r-40)), ln(at(a1, r-40), at(a0, r-40)), ln(at(a0, r-40), at(a0, r)))
	return FromSegments(segs)
}

func kindsOf(ps []piece) string {
	s := ""
	for i, p := range ps {
		if i > 0 {
			s += " "
		}
		s += [...]string{"free", "line", "arc"}[p.kind]
	}
	return s
}

func arcPiece(t *testing.T, ps []piece, r float64) piece {
	t.Helper()
	for _, p := range ps {
		if p.kind == pieceArc && math.Abs(p.r-r) <= 0.01*r {
			return p
		}
	}
	t.Fatalf("no arc piece of r≈%v in %s", r, kindsOf(ps))
	return piece{}
}

func TestPiecesSplitMixedRadii(t *testing.T) {
	_, _, ps := loopPieces(filletD(), 0.5, Options{})
	if got := kindsOf(ps); got != "line arc arc line line" {
		t.Fatalf("pieces %q", got)
	}
	if p := arcPiece(t, ps, 100); p.count != 1 {
		t.Fatalf("r=100 piece count %d", p.count)
	}
	if p := arcPiece(t, ps, 12); p.count != 1 {
		t.Fatalf("r=12 piece count %d", p.count)
	}
}

func TestPiecesMergeHealsSplit(t *testing.T) {
	_, _, ps := loopPieces(arcThenFillet(), 0.5, Options{})
	if p := arcPiece(t, ps, 80); p.count != 4 {
		t.Fatalf("r=80 piece count %d in %s", p.count, kindsOf(ps))
	}
	if p := arcPiece(t, ps, 8); p.count != 2 {
		t.Fatalf("r=8 piece count %d in %s", p.count, kindsOf(ps))
	}
}

func TestPiecesShortArcLeftoverIsNotLine(t *testing.T) {
	_, _, ps := loopPieces(shallowArc(), 0.5, Options{})
	if p := arcPiece(t, ps, 700); p.count != 5 {
		t.Fatalf("r=700 piece count %d in %s", p.count, kindsOf(ps))
	}
	if got := kindsOf(ps); got != "arc line line line" && got != "line line line arc" && got != "line arc line line" && got != "line line arc line" {
		t.Fatalf("pieces %q", got)
	}
}

func TestPiecesUncutLoopGetsFirstCut(t *testing.T) {
	b := roundedRectRadii([4]float64{24, 24, 24, 24})
	if hardCuts(b, Options{}) != nil {
		t.Fatal("a rounded rectangle has no hard cut")
	}
	_, _, ps := loopPieces(b, 0.5, Options{})
	lines, arcs := 0, 0
	for _, p := range ps {
		switch p.kind {
		case pieceLine:
			lines++
		case pieceArc:
			arcs++
		}
	}
	if len(ps) != 8 || lines != 4 || arcs != 4 {
		t.Fatalf("%d pieces (%s)", len(ps), kindsOf(ps))
	}
}

func TestPiecesRespectFixedAndFrozen(t *testing.T) {
	b := arcThenFillet()
	fixed := make([]bool, b.N())
	fixed[2] = true
	_, _, ps := loopPieces(b, 0.5, Options{Fixed: fixed})
	halves := 0
	for _, p := range ps {
		if p.kind == pieceArc && math.Abs(p.r-80) < 0.8 && p.count == 2 {
			halves++
		}
	}
	if halves != 2 {
		t.Fatalf("fixed joint: want two arc pieces of two segments, got %s", kindsOf(ps))
	}
	frozen := make([]bool, b.N())
	frozen[1] = true
	rb, _, ps := loopPieces(b, 0.5, Options{Frozen: frozen})
	// The loop was rotated; find the frozen segment by its geometry.
	want := b.Segment(1)
	found := false
	for _, p := range ps {
		for k := 0; k < p.count; k++ {
			if rb.Segment(p.first+k) == want {
				found = true
				if p.kind != pieceFree || p.count != 1 {
					t.Fatalf("frozen segment in piece %+v", p)
				}
			}
		}
	}
	if !found {
		t.Fatal("frozen segment lost")
	}
}

func TestPiecesTinyLoops(t *testing.T) {
	one := FromSegments([]fit.Segment{cubic(v(10, 10), v(30, 0), v(30, 20), v(10, 10))})
	two := FromSegments([]fit.Segment{cubic(v(10, 10), v(30, 0), v(50, 0), v(50, 10)), cubic(v(50, 10), v(50, 20), v(30, 20), v(10, 10))})
	for _, b := range []Bezigon{one, two} {
		_, _, ps := loopPieces(b, 0.5, Options{})
		if len(ps) > 1 || (len(ps) == 1 && ps[0].kind != pieceFree) {
			t.Fatalf("N=%d: %s", b.N(), kindsOf(ps))
		}
	}
}

func TestPiecesSplitDepthBounded(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 7))
	const n = 512
	var segs []fit.Segment
	pt := func(i int) geom.Vec2 {
		a := 2 * math.Pi * float64(i%n) / n
		r := 100 + 2*rng.Float64() - 1
		return v(128+r*math.Cos(a), 128+r*math.Sin(a))
	}
	pts := make([]geom.Vec2, n+1)
	for i := range pts {
		pts[i] = pt(i)
	}
	pts[n] = pts[0]
	for i := 0; i < n; i++ {
		segs = append(segs, ln(pts[i], pts[i+1]))
	}
	start := time.Now()
	loopPieces(FromSegments(segs), 0.5, Options{})
	if d := time.Since(start); d > 200*time.Millisecond {
		t.Fatalf("512 jittered segments took %v", d)
	}
}
