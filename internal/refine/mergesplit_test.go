package refine

import (
	"math"
	"testing"

	"github.com/vexyl-ai/rekha/internal/fit"
	"github.com/vexyl-ai/rekha/internal/geom"
)

// dataOf is the data energy of b over the image at l0.
func dataOf(b Bezigon, m Mask, img Window, l0 float64) float64 {
	cov := make([]float32, img.W*img.H)
	Coverage(b, img, cov)
	return DataEnergy(cov, m, img, l0)
}

// A circle fitted into eight cubics (each ideal quarter split in half)
// merges back to four or fewer without the fit getting worse.
func TestMergeReducesOverSegmentedCircle(t *testing.T) {
	ideal := circle(32, 32, 20)
	m := maskOf(ideal, img64)
	eight := ideal
	for j := 7; j >= 0; j -= 2 { // split each original quarter at t = 0.5
		eight = splitSegment(eight, j/2)
	}
	if eight.N() != 8 {
		t.Fatalf("fixture has %d segments, want 8", eight.N())
	}
	l0 := math.Max(eight.Length(), 1)
	before := dataOf(eight, m, img64, l0)
	got := MergeSplit(eight, m, img64, DefaultOptions)
	if got.N() > 4 {
		t.Fatalf("N = %d after merge, want ≤ 4", got.N())
	}
	if after := dataOf(got, m, img64, l0); after > before*1.01+1e-9 {
		t.Fatalf("data energy rose %v → %v", before, after)
	}
}

func TestMergeNeverBelowTwo(t *testing.T) {
	b := smoothBezigon(4, 2)
	m := maskOf(b, img64)
	if got := MergeSplit(b, m, img64, DefaultOptions); got.N() != 2 {
		t.Fatalf("N = %d, want 2", got.N())
	}
}

// One cubic spanning a right angle of the square (three segments in all)
// against the square's mask splits at the corner and the fit improves.
func TestSplitAtCorner(t *testing.T) {
	sq := square(10, 10, 40, 40)
	m := maskOf(sq, img64)
	b := FromSegments([]fit.Segment{
		cubic(v(10, 10), v(40, 10), v(40, 10), v(40, 40)),
		ln(v(40, 40), v(10, 40)), ln(v(10, 40), v(10, 10)),
	})
	l0 := math.Max(b.Length(), 1)
	before := dataOf(b, m, img64, l0)
	got := MergeSplit(b, m, img64, DefaultOptions)
	if got.N() != 4 {
		t.Fatalf("N = %d, want 4", got.N())
	}
	if after := dataOf(got, m, img64, l0); after > before/2 {
		t.Fatalf("data energy %v → %v, want at least halved", before, after)
	}
}

// Sharp joints never merge and a perfect fit never splits.
func TestMergeSplitKeepsSharpJoints(t *testing.T) {
	sq := square(10, 10, 40, 40)
	m := maskOf(sq, img64)
	got := MergeSplit(sq, m, img64, DefaultOptions)
	if got.N() != 4 {
		t.Fatalf("N = %d, want 4", got.N())
	}
	for j := 0; j < 4; j++ {
		if d := got.Pts[3*j].Sub(sq.Pts[3*j]).Len(); d > 0.1 {
			t.Fatalf("corner %d moved %v", j, d)
		}
	}
}

// splitSegment halves a segment at t = 0.5 and keeps the outline.
func TestSplitSegmentKeepsCurve(t *testing.T) {
	b := smoothBezigon(2, 4)
	got := splitSegment(b, 1)
	if got.N() != 5 {
		t.Fatalf("N = %d, want 5", got.N())
	}
	if d := curveDist(got, b); d > 0.01 { // the 64-point flattening's chord error
		t.Fatalf("outline moved %v", d)
	}
	if !near(got.Pts[3*2], b.Segment(1).At(0.5), 1e-9) {
		t.Fatalf("new anchor %v, want %v", got.Pts[6], b.Segment(1).At(0.5))
	}
}

// TestLocalDataEnergyDeltaMatchesGlobal: when only a few segments change,
// the data energy difference measured over the window around them equals
// the difference over the whole image.
func TestLocalDataEnergyDeltaMatchesGlobal(t *testing.T) {
	img := Window{X0: 0, Y0: 0, W: 64, H: 64}
	m := maskOf(circle(32, 32, 20), img)
	b := smoothBezigon(3, 8)
	cand := Bezigon{Pts: append([]geom.Vec2(nil), b.Pts...)}
	// Move segment 2's inner points and the joint into segment 3.
	for _, i := range []int{7, 8, 9, 10} {
		cand.Pts[i] = cand.Pts[i].Add(v(1.3, -0.8))
	}
	wl := windowAround(img, segmentsBox(b, 2, 3), segmentsBox(cand, 2, 3))
	if wl.W >= img.W || wl.H >= img.H {
		t.Fatalf("local window %+v is not local", wl)
	}
	const l0 = 100.0
	global := dataOf(cand, m, img, l0) - dataOf(b, m, img, l0)
	local := localDataEnergy(cand, m, wl, l0) - localDataEnergy(b, m, wl, l0)
	if math.Abs(global-local) > 1e-6*math.Max(math.Abs(global), 1) {
		t.Fatalf("global delta %v local delta %v", global, local)
	}
}

// TestMergePassWithinBudget: however many merges are accepted, the data
// energy ends no more than mergeRise above where the pass started.
func TestMergePassWithinBudget(t *testing.T) {
	img := Window{X0: 0, Y0: 0, W: 64, H: 64}
	for _, seed := range []uint64{1, 3, 5} {
		src := smoothBezigon(seed, 6)
		b := Bezigon{Pts: append([]geom.Vec2(nil), src.Pts...)}
		for b.N() < 24 {
			b = splitSegment(b, b.N()-1)
			b = splitSegment(b, 0)
		}
		m := maskOf(src, img)
		o := Options{Sweeps: 1, MaxIter: 30, Weights: DefaultWeights}
		l0 := math.Max(b.Length(), 1)
		before := dataOf(b, m, img, l0)
		out, _ := mergePass(b, m, img, l0, o, &scratch{})
		after := dataOf(out, m, img, l0)
		t.Logf("seed %d: %d → %d segments, energy %.4g → %.4g", seed, b.N(), out.N(), before, after)
		if after > before*(1+mergeRise)+1e-9 {
			t.Errorf("seed %d: energy rose %.2f%%", seed, 100*(after/before-1))
		}
	}
}

// TestMergeDeviation: a cubic split in half merges back within a tenth
// of a pixel (the refit and the flattening are not exact); an S-curve of
// two tangent-continuous cubics cannot be one cubic and deviates by
// pixels.
func TestMergeDeviation(t *testing.T) {
	c := fit.Cubic{{X: 0, Y: 0}, {X: 10, Y: 20}, {X: 30, Y: 20}, {X: 40, Y: 0}}
	b := Bezigon{Pts: []geom.Vec2{c[0], c[1], c[2], c[3], {X: 40, Y: -10}, {X: 0, Y: -10}}}
	halves := splitSegment(b, 0)
	if d := mergeDeviation(halves, 1, mergeJoint(halves, 1).Segment(0)); d > 0.1 {
		t.Fatalf("halves of one cubic deviate %v", d)
	}
	s := Bezigon{Pts: []geom.Vec2{
		{X: 0, Y: 0}, {X: 20, Y: 0}, {X: 20, Y: 30}, // bends up
		{X: 40, Y: 30}, {X: 60, Y: 30}, {X: 60, Y: 0}, // bends down, tangent continuous
		{X: 80, Y: 0}, {X: 80, Y: -20}, {X: 0, Y: -20},
	}}
	if d := mergeDeviation(s, 1, mergeJoint(s, 1).Segment(0)); d < 2 {
		t.Fatalf("S-curve merged cubic deviates only %v", d)
	}
}

// TestRangeCrossingsCountsOnlyTheRange: the merge pass judges a candidate
// by the crossings of the segments it changed, not the whole loop (which
// made the pass quadratic in the outline length). The figure eight's
// only crossing is between segments 0 and 2.
func TestRangeCrossingsCountsOnlyTheRange(t *testing.T) {
	b := figureEight()
	for _, c := range []struct{ from, to, want int }{
		{0, 0, 1}, {2, 2, 1}, {1, 1, 0}, {3, 3, 0}, {0, 3, 1}, {-1, 1, 1}, {1, 2, 1},
	} {
		if got := rangeCrossings(b, c.from, c.to); got != c.want {
			t.Errorf("rangeCrossings(%d, %d) = %d, want %d", c.from, c.to, got, c.want)
		}
	}
	if got := rangeCrossings(circle(32, 32, 20), 0, 3); got != 0 {
		t.Errorf("circle: %d crossings", got)
	}
}
