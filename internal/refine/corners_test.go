package refine

import (
	"math"
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

// halfDiscSliver is the half-disc whose arc stops short of the bottom
// corner, the last 2.5 px of it traced as a kinked straight sliver to a
// corner anchor pushed 1.2 px out along the flat side, as the
// refinement leaves a corner between a curve and a straight edge.
func halfDiscSliver() Bezigon {
	c := v(128, 128)
	end := math.Pi/2 - 2.5/80
	segs := []fit.Segment{ln(v(128, 209.2), v(128, 48))}
	segs = append(segs, nudgedArc(c, 80, -math.Pi/2, end, 4, 0.2)...)
	segs = append(segs, ln(segs[len(segs)-1].P[3], v(128, 209.2)))
	return FromSegments(segs)
}

// TestSharpenCornerBetweenArcAndLine: the sliver goes, the corner sits
// where the flat side meets the circle, and the arc reaches it.
func TestSharpenCornerBetweenArcAndLine(t *testing.T) {
	m := supersampled(halfDisc(0), win256)
	out, _ := SnapPieces(halfDiscSliver(), m, win256, 0.5, Options{})
	lines, curved := countKinds(out, 0.25)
	if out.N() != 3 || lines != 1 || curved != 2 {
		t.Fatalf("%d segments, %d lines, %d curved", out.N(), lines, curved)
	}
	cornerAt(t, out, v(128, 208), 0.15)
	for j := 0; j < out.N(); j++ {
		if c := out.Segment(j); !straightCubic(c, 0.25) {
			for _, p := range []geom.Vec2{c[0], c[3]} {
				if r := p.Sub(v(128, 128)).Len(); math.Abs(r-80) > 0.1 {
					t.Errorf("anchor %v is %.3f from the centre", p, r)
				}
			}
		}
	}
}
