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
	c := Cubic{{0, 0}, {30, 60}, {70, 60}, {100, 0}}
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
		pts = append(pts, geom.Vec2{float64(i) * 2, float64(i) * 3})
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
	segs := FitRun([]geom.Vec2{{0, 0}, {1, 1}, {2, 0}}, 0.1)
	if len(segs) != 1 || !segs[0].Line {
		t.Fatalf("got %+v", segs)
	}
	if segs[0].P[0] != (geom.Vec2{0, 0}) || segs[0].P[3] != (geom.Vec2{2, 0}) {
		t.Fatalf("line endpoints %v %v", segs[0].P[0], segs[0].P[3])
	}
}

func TestFitCircleWithinTolerance(t *testing.T) {
	var pts []geom.Vec2
	for i := 0; i < 200; i++ {
		a := 2 * math.Pi * float64(i) / 200
		pts = append(pts, geom.Vec2{50 * math.Cos(a), 50 * math.Sin(a)})
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
		pts = append(pts, geom.Vec2{float64(i), float64(20 - i)})
	}
	for i := 1; i <= 20; i++ {
		pts = append(pts, geom.Vec2{float64(20 + i), float64(i)})
	}
	segs := FitRun(pts, 0.1)
	if len(segs) < 2 {
		t.Fatalf("got %d segments, want ≥ 2", len(segs))
	}
}
