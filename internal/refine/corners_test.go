package refine

import (
	"testing"

	"github.com/vexyl-ai/rekha/internal/corpus"
	"github.com/vexyl-ai/rekha/internal/fit"
	"github.com/vexyl-ai/rekha/internal/geom"
)

// filletedTriangle is the corpus triangle whose corner A = (38,196) is
// cut by a fillet of the given size: the edges stop d px short of the
// corner and a cubic bowing towards it joins them.
func filletedTriangle(d float64) Bezigon {
	tc := corpus.TriangleCorners()
	a, bb, c := tc[0], tc[1], tc[2]
	p1 := a.Add(c.Sub(a).Norm().Scale(d))
	p2 := a.Add(bb.Sub(a).Norm().Scale(d))
	return FromSegments([]fit.Segment{
		ln(bb, c), ln(c, p1),
		cubic(p1, p1.Add(a.Sub(p1).Scale(0.55)), p2.Add(a.Sub(p2).Scale(0.55)), p2),
		ln(p2, bb),
	})
}

func sharpTriangleMask() coverageMask {
	tc := corpus.TriangleCorners()
	return supersampled(FromSegments([]fit.Segment{ln(tc[1], tc[2]), ln(tc[2], tc[0]), ln(tc[0], tc[1])}), win256)
}

func cornerAt(t *testing.T, b Bezigon, want geom.Vec2, tol float64) {
	t.Helper()
	best := -1.0
	for j := 0; j < b.N(); j++ {
		if d := b.Pts[3*j].Sub(want).Len(); best < 0 || d < best {
			best = d
		}
	}
	if best > tol {
		t.Fatalf("nearest anchor is %.3f px from %v", best, want)
	}
}

func TestSharpenCornerDropsSmallFillet(t *testing.T) {
	out, _ := SnapPieces(filletedTriangle(2), sharpTriangleMask(), win256, 0.5, Options{})
	if out.N() != 3 {
		t.Fatalf("%d segments, want 3", out.N())
	}
	cornerAt(t, out, corpus.TriangleCorners()[0], 0.15)
}

// noCornerAt fails when an anchor of b lies within tol of want.
func noCornerAt(t *testing.T, b Bezigon, want geom.Vec2, tol float64) {
	t.Helper()
	for j := 0; j < b.N(); j++ {
		if d := b.Pts[3*j].Sub(want).Len(); d < tol {
			t.Fatalf("anchor %d is %.3f px from %v: the fillet was dropped", j, d, want)
		}
	}
}

// TestSharpenCornerKeepsLargeFillet: a 10 px fillet is an arc of 124°,
// snapped to two arc cubics, and stays.
func TestSharpenCornerKeepsLargeFillet(t *testing.T) {
	out, _ := SnapPieces(filletedTriangle(10), sharpTriangleMask(), win256, 0.5, Options{})
	if out.N() != 5 {
		t.Fatalf("%d segments, want 5", out.N())
	}
	noCornerAt(t, out, corpus.TriangleCorners()[0], 3)
}

func TestSharpenCornerKeepsFixedJoint(t *testing.T) {
	out, ro := SnapPieces(filletedTriangle(2), sharpTriangleMask(), win256, 0.5, Options{Fixed: []bool{false, false, true, false}})
	if out.N() < 4 {
		t.Fatalf("%d segments, want the fillet kept", out.N())
	}
	noCornerAt(t, out, corpus.TriangleCorners()[0], 1)
	fixed := 0
	for _, f := range ro.Fixed {
		if f {
			fixed++
		}
	}
	if fixed != 1 {
		t.Fatalf("fixed flags %v", ro.Fixed)
	}
}
