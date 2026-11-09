package fit

import (
	"math"
	"testing"

	"github.com/vexyl-ai/rekha/internal/geom"
)

func sample(c Cubic, n int) []geom.Vec2 {
	out := make([]geom.Vec2, n)
	for i := range out {
		out[i] = c.At(float64(i) / float64(n-1))
	}
	return out
}

// distToSegs returns the distance from p to the nearest segment: exact for
// lines, coarse sampling plus golden-section refinement for cubics.
func distToSegs(p geom.Vec2, segs []Segment) float64 {
	best := math.Inf(1)
	for _, s := range segs {
		var d float64
		if s.Line {
			d = geom.PerpDist(p, s.P[0], s.P[3])
			// Clamp to the segment extent.
			ab := s.P[3].Sub(s.P[0])
			t := p.Sub(s.P[0]).Dot(ab) / ab.Dot(ab)
			if t < 0 || t > 1 {
				d = math.Min(p.Sub(s.P[0]).Len(), p.Sub(s.P[3]).Len())
			}
		} else {
			f := func(t float64) float64 { return s.P.At(t).Sub(p).Len() }
			bt, bd := 0.0, math.Inf(1)
			for i := 0; i <= 200; i++ {
				t := float64(i) / 200
				if v := f(t); v < bd {
					bt, bd = t, v
				}
			}
			lo, hi := math.Max(0, bt-0.01), math.Min(1, bt+0.01)
			for i := 0; i < 60; i++ {
				m1 := lo + (hi-lo)*0.382
				m2 := lo + (hi-lo)*0.618
				if f(m1) < f(m2) {
					hi = m2
				} else {
					lo = m1
				}
			}
			d = f((lo + hi) / 2)
		}
		if d < best {
			best = d
		}
	}
	return best
}

func TestFitRecoversCubic(t *testing.T) {
	c := Cubic{{X: 0, Y: 0}, {X: 30, Y: 60}, {X: 70, Y: 60}, {X: 100, Y: 0}}
	pts := sample(c, 50)
	t0 := c[1].Sub(c[0]).Norm()
	t1 := c[2].Sub(c[3]).Norm()
	segs := FitOpen(pts, t0, t1, 0.1)
	if len(segs) != 1 || segs[0].Line {
		t.Fatalf("got %d segments (line=%v)", len(segs), len(segs) > 0 && segs[0].Line)
	}
	for i := range c {
		if d := segs[0].P[i].Sub(c[i]).Len(); d > 0.5 {
			t.Fatalf("control point %d off by %.3f: %v vs %v", i, d, segs[0].P[i], c[i])
		}
	}
}

func TestFitLineIsSingleSegment(t *testing.T) {
	var pts []geom.Vec2
	for i := 0; i < 20; i++ {
		pts = append(pts, geom.Vec2{X: float64(i) * 2, Y: float64(i) * 3})
	}
	segs := FitRun(pts, 0.5)
	if len(segs) != 1 {
		t.Fatalf("got %d segments", len(segs))
	}
	for _, p := range pts {
		if d := distToSegs(p, segs); d > 1e-6 {
			t.Fatalf("point %v deviates %.3g", p, d)
		}
	}
}

func TestFitFewPointsIsLine(t *testing.T) {
	segs := FitRun([]geom.Vec2{{X: 0, Y: 0}, {X: 1, Y: 1}, {X: 2, Y: 0}}, 0.1)
	if len(segs) != 1 || !segs[0].Line {
		t.Fatalf("got %+v", segs)
	}
	if segs[0].P[0] != (geom.Vec2{X: 0, Y: 0}) || segs[0].P[3] != (geom.Vec2{X: 2, Y: 0}) {
		t.Fatalf("line endpoints %v %v", segs[0].P[0], segs[0].P[3])
	}
}

func TestFitCircleWithinTolerance(t *testing.T) {
	var pts []geom.Vec2
	for i := 0; i < 200; i++ {
		a := 2 * math.Pi * float64(i) / 200
		pts = append(pts, geom.Vec2{X: 50 * math.Cos(a), Y: 50 * math.Sin(a)})
	}
	segs := FitClosed(pts, 0.5)
	if len(segs) > 8 {
		t.Fatalf("got %d segments, want ≤ 8", len(segs))
	}
	for _, p := range pts {
		if d := distToSegs(p, segs); d > 0.5 {
			t.Fatalf("point %v deviates %.3f", p, d)
		}
	}
	// Closed: last segment ends where the first begins.
	last := segs[len(segs)-1]
	if d := last.P[3].Sub(segs[0].P[0]).Len(); d > 1e-9 {
		t.Fatalf("not closed: gap %.3g", d)
	}
}

func TestFitSplitsOnSharpError(t *testing.T) {
	var pts []geom.Vec2
	for i := 0; i <= 20; i++ {
		pts = append(pts, geom.Vec2{X: float64(i), Y: float64(20 - i)})
	}
	for i := 1; i <= 20; i++ {
		pts = append(pts, geom.Vec2{X: float64(20 + i), Y: float64(i)})
	}
	segs := FitRun(pts, 0.1)
	if len(segs) < 2 {
		t.Fatalf("got %d segments, want ≥ 2", len(segs))
	}
}

// digitalCircle returns the integer-grid boundary of a disc of radius r:
// the loop of pixel corners visited by walking the quantised circle, which
// zigzags like a real traced contour.
func digitalCircle(r float64) []geom.Vec2 {
	var pts []geom.Vec2
	steps := int(2 * math.Pi * r * 4)
	for i := 0; i < steps; i++ {
		a := 2 * math.Pi * float64(i) / float64(steps)
		p := geom.Vec2{X: math.Round(r * math.Cos(a)), Y: math.Round(r * math.Sin(a))}
		if len(pts) == 0 || pts[len(pts)-1] != p {
			pts = append(pts, p)
		}
	}
	if pts[0] == pts[len(pts)-1] {
		pts = pts[:len(pts)-1]
	}
	return pts
}

// A quantised circle fitted at tolerance 1.5 (1 px plus the half-pixel
// the integer corners sit off the true edge) must come out round: radial
// deviation stays near the quantisation error and a handful of cubics
// suffice. Zigzag tangents at split points, or lopsided splits, produce
// visible lumps and many extra segments.
func TestFitClosedDigitalCircleIsRound(t *testing.T) {
	for _, r := range []float64{16, 48, 96, 300, 1500} {
		segs := FitClosed(digitalCircle(r), 1.5)
		if len(segs) > 8 {
			t.Errorf("r=%v: segments = %d, want <= 8", r, len(segs))
		}
		worst := 0.0
		for _, s := range segs {
			for k := 0; k <= 20; k++ {
				d := math.Abs(s.P.At(float64(k)/20).Len() - r)
				worst = math.Max(worst, d)
			}
		}
		if worst > 1.25 {
			t.Errorf("r=%v: max radial deviation = %.2f, want <= 1.25", r, worst)
		}
	}
}

// A run whose points all lie within tolerance of its chord is a line, even
// when the supplied end tangents disagree with it: straight edges must
// not come out as cubics with collinear control points.
func TestFitOpenStraightRunIsLine(t *testing.T) {
	var pts []geom.Vec2
	for i := 0; i <= 40; i++ {
		pts = append(pts, geom.Vec2{X: float64(i), Y: float64(i % 2)}) // 1 px zigzag
	}
	segs := FitOpen(pts, geom.Vec2{X: 1, Y: 0.3}.Norm(), geom.Vec2{X: -1, Y: 0.3}.Norm(), 1.0)
	if len(segs) != 1 || !segs[0].Line {
		t.Fatalf("got %+v, want one Line segment", segs)
	}
}

// A long straight run whose first point is displaced (a corner placed off
// the edge) must not be shattered: the fitter isolates the bad end rather
// than halving the straight part again and again.
func TestFitIsolatesBadEnd(t *testing.T) {
	var pts []geom.Vec2
	pts = append(pts, geom.Vec2{X: 0, Y: 4}) // displaced start
	for i := 1; i <= 400; i++ {
		pts = append(pts, geom.Vec2{X: float64(i), Y: float64(i % 2)})
	}
	segs := FitOpen(pts, geom.Vec2{X: 1, Y: 0}, geom.Vec2{X: -1, Y: 0}, 1.5)
	if len(segs) > 3 {
		t.Fatalf("got %d segments, want at most 3", len(segs))
	}
}

// A run that bends (down three, then left three) must not be fitted by a
// near-chord that lies to one side of every point even though each is
// within tolerance: on a feature thinner than twice the tolerance both
// sides would fit the same curve and the feature would vanish. The fit
// has to follow the bend.
func TestFitRejectsOneSidedCubic(t *testing.T) {
	pts := []geom.Vec2{{X: 3, Y: 0}, {X: 3, Y: 1}, {X: 2, Y: 1}, {X: 2, Y: 2}, {X: 1, Y: 2}, {X: 1, Y: 3}, {X: 0, Y: 3}, {X: 0, Y: 4}, {X: -3, Y: 4}}
	segs := FitOpen(pts, geom.Vec2{X: -1, Y: 1}.Norm(), geom.Vec2{X: 1, Y: 0}, 1.5)
	if d := distToSegs(geom.Vec2{X: 0, Y: 4}, segs); d > 0.75 {
		t.Fatalf("bend point %.2f px from the fit %+v", d, segs)
	}
	// A straight 3-px-wide band's side, where the chord is the fit, must
	// not be accepted as a line when every point lies on one side of it.
	side := []geom.Vec2{{X: 0, Y: 0}, {X: 1, Y: 1}, {X: 2, Y: 1}, {X: 3, Y: 1}, {X: 4, Y: 1}, {X: 5, Y: 1}, {X: 6, Y: 1}, {X: 7, Y: 1}, {X: 8, Y: 0}}
	if segs := FitOpen(side, geom.Vec2{X: 1, Y: 0}, geom.Vec2{X: -1, Y: 0}, 1.5); len(segs) == 1 && segs[0].Line {
		t.Fatalf("one-sided run accepted as its chord")
	}
}
