package refine

import (
	"math"
	"testing"

	"github.com/vexyl-ai/rekha/internal/fit"
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
