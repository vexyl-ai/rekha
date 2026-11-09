package refine

import (
	"math"
	"testing"

	"github.com/vexyl-ai/rekha/internal/fit"
	"github.com/vexyl-ai/rekha/internal/geom"
)

// coverageMask is a Mask backed by a coverage array over a window.
type coverageMask struct {
	win Window
	cov []float32
}

func (m coverageMask) At(x, y int) (float32, float32) {
	x, y = x-m.win.X0, y-m.win.Y0
	if x < 0 || y < 0 || x >= m.win.W || y >= m.win.H {
		return 0, 1
	}
	return m.cov[y*m.win.W+x], 1
}

func maskOf(b Bezigon, win Window) coverageMask {
	cov := make([]float32, win.W*win.H)
	Coverage(b, win, cov)
	return coverageMask{win, cov}
}

// figureEight is a self-crossing bow tie: (0,0) → (40,40) → (40,0) → (0,40).
func figureEight() Bezigon {
	return FromSegments([]fit.Segment{
		ln(v(0, 0), v(40, 40)), ln(v(40, 40), v(40, 0)), ln(v(40, 0), v(0, 40)), ln(v(0, 40), v(0, 0)),
	})
}

func TestAnglePriorSquare(t *testing.T) {
	if got := AnglePrior(square(0, 0, 10, 10)); math.Abs(got-2*math.Pi) > 1e-6 {
		t.Fatalf("square angle prior %v, want 2π", got)
	}
	if got := AnglePrior(circle(32, 32, 20)); got > 1e-3 {
		t.Fatalf("circle angle prior %v, want ~0", got)
	}
	// Joint subset: the square's joints 0 and 1 give π.
	if got := AnglePrior(square(0, 0, 10, 10), 0, 1); math.Abs(got-math.Pi) > 1e-6 {
		t.Fatalf("two joints %v, want π", got)
	}
}

func TestHandlePriorNearZero(t *testing.T) {
	sq := square(0, 0, 10, 10)
	if got := HandlePrior(sq); got >= 10 {
		t.Fatalf("square handle prior %v", got)
	}
	sq.Pts[1] = sq.Pts[0].Add(v(1e-9, 0))
	if got := HandlePrior(sq); got <= 1e5 {
		t.Fatalf("near-zero handle prior %v", got)
	}
	if got := HandlePrior(sq, 2); got >= 10 {
		t.Fatalf("segment 2 alone should be unaffected: %v", got)
	}
}

func TestLengthPriorEqualsPolyline(t *testing.T) {
	c := circle(32, 32, 20)
	if got, want := LengthPrior(c), c.Length(); math.Abs(got-want) > 1e-9 {
		t.Fatalf("%v vs %v", got, want)
	}
	sq := square(0, 0, 10, 10)
	if got := LengthPrior(sq, 0, 1); math.Abs(got-20) > 1e-9 {
		t.Fatalf("two sides %v, want 20", got)
	}
}

func TestSelfIntersectionFigureEight(t *testing.T) {
	if got := SelfIntersectionPrior(figureEight()); got <= 0 {
		t.Fatalf("figure eight prior %v, want > 0", got)
	}
	if got := SelfIntersectionPrior(circle(32, 32, 20)); got != 0 {
		t.Fatalf("circle prior %v, want 0", got)
	}
}

func TestDataEnergyZeroOnOwnCoverage(t *testing.T) {
	win := Window{X0: 0, Y0: 0, W: 64, H: 64}
	c := circle(32, 32, 20)
	cov := make([]float32, 64*64)
	Coverage(c, win, cov)
	if got := DataEnergy(cov, maskOf(c, win), win, c.Length()); got != 0 {
		t.Fatalf("own coverage energy %v", got)
	}
	shifted := circle(33, 32, 20)
	if got := DataEnergy(cov, maskOf(shifted, win), win, c.Length()); got <= 0 {
		t.Fatalf("shifted energy %v", got)
	}
}

func TestWeightsPrior(t *testing.T) {
	sq := square(0, 0, 10, 10)
	w := DefaultWeights
	want := w.Spt*SelfIntersectionPrior(sq) + w.Apt*AnglePrior(sq) + w.Hpt*HandlePrior(sq) + w.Opt*HandleOrderPrior(sq) + w.Lpt*LengthPrior(sq)/40
	if got := w.Prior(sq, 40); math.Abs(got-want) > 1e-12 {
		t.Fatalf("%v vs %v", got, want)
	}
}

// TestSelfIntersectionCandidatesMatchFull: pruning the other edges to the
// ones whose box overlaps the range's reach changes nothing.
func TestSelfIntersectionCandidatesMatchFull(t *testing.T) {
	for _, b := range []Bezigon{figureEight(), smoothBezigon(3, 12), circle(32, 32, 20)} {
		pts := b.Polyline()
		for j := 0; j < b.N(); j++ {
			lo, hi := Flatten*j, Flatten*(j+2)
			r := reach(b, 3*j, 3*j+7, Window{X0: 0, Y0: 0, W: 64, H: 64})
			cand := edgeCandidates(pts, r.lo, r.hi, nil)
			if len(cand) > len(pts) {
				t.Fatalf("piece %d: %d candidates for %d edges", j, len(cand), len(pts))
			}
			full, pruned := selfIntersection(pts, lo, hi, nil, nil), selfIntersection(pts, lo, hi, cand, nil)
			if full != pruned {
				t.Fatalf("piece %d: full %v pruned %v", j, full, pruned)
			}
		}
	}
	// Edges far from the box are never candidates.
	pts := circle(32, 32, 20).Polyline()
	if c := edgeCandidates(pts, v(10, 30), v(14, 34), nil); len(c) == len(pts) || len(c) == 0 {
		t.Fatalf("box on the left rim: %d candidates of %d", len(c), len(pts))
	}
}

// TestSelfIntersectionPrecomputedArcs: a caller-supplied arc-length table
// gives the same prior as the one built on demand.
func TestSelfIntersectionPrecomputedArcs(t *testing.T) {
	pts := figureEight().Polyline()
	cum := arcLengths(pts, nil)
	if len(cum) != len(pts)+1 || cum[0] != 0 || cum[len(pts)] <= 0 {
		t.Fatalf("arc table %d entries, first %v last %v", len(cum), cum[0], cum[len(pts)])
	}
	want := selfIntersection(pts, 0, len(pts), nil, nil)
	if got := selfIntersection(pts, 0, len(pts), nil, cum); got != want || want <= 0 {
		t.Fatalf("precomputed %v on-demand %v", got, want)
	}
}

func TestHandleOrderPrior(t *testing.T) {
	// A quarter circle and a straight line are well-formed: zero.
	if got := HandleOrderPrior(circle(32, 32, 20)); got != 0 {
		t.Fatalf("circle %v, want 0", got)
	}
	if got := HandleOrderPrior(square(0, 0, 10, 10)); got != 0 {
		t.Fatalf("square %v, want 0", got)
	}
	// Crossed handles: h1 near the far end, h2 near the start.
	crossed := Bezigon{Pts: []geom.Vec2{v(0, 0), v(9, 1), v(1, -1), v(10, 0), v(10, 5), v(0, 5)}}
	if got := HandleOrderPrior(crossed); got < 1 {
		t.Fatalf("crossed handles %v, want ≥ 1", got)
	}
	// A handle running backwards past its anchor.
	back := Bezigon{Pts: []geom.Vec2{v(0, 0), v(-3, 0), v(7, 0), v(10, 0), v(10, 5), v(0, 5)}}
	if got := HandleOrderPrior(back); got < 1 {
		t.Fatalf("backward handle %v, want ≥ 1", got)
	}
	if got := HandleOrderPrior(back, 1); got != 0 {
		t.Fatalf("segment 1 alone %v, want 0", got)
	}
}
