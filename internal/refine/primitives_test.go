package refine

import (
	"math"
	"testing"

	"github.com/vexyl-ai/rekha/internal/corpus"
	"github.com/vexyl-ai/rekha/internal/fit"
	"github.com/vexyl-ai/rekha/internal/geom"
)

// arcSeg is the cubic approximating the arc of the circle (centre, r)
// from angle a0 to a1 (radians, y down), handle 4/3·tan(θ/4)·r.
func arcSeg(centre geom.Vec2, r, a0, a1 float64) fit.Segment {
	k := 4.0 / 3 * math.Tan((a1-a0)/4) * r
	p := func(a float64) geom.Vec2 { return v(centre.X+r*math.Cos(a), centre.Y+r*math.Sin(a)) }
	t := func(a float64) geom.Vec2 { return v(-math.Sin(a), math.Cos(a)) }
	return cubic(p(a0), p(a0).Add(t(a0).Scale(k)), p(a1).Sub(t(a1).Scale(k)), p(a1))
}

// cornerShape is a 60×60 square at (10,10) whose top-right corner is an
// arc of radius 20 drawn as the given segments between (50,10) and (70,30).
func cornerShape(corner ...fit.Segment) Bezigon {
	segs := []fit.Segment{ln(v(10, 10), v(50, 10))}
	segs = append(segs, corner...)
	segs = append(segs, ln(v(70, 30), v(70, 70)), ln(v(70, 70), v(10, 70)), ln(v(10, 70), v(10, 10)))
	return FromSegments(segs)
}

var win80 = Window{0, 0, 80, 80}

// quarterTwice is the quarter arc as two 45° cubics.
func quarterTwice() Bezigon {
	c := v(50, 30)
	return cornerShape(arcSeg(c, 20, -math.Pi/2, -math.Pi/4), arcSeg(c, 20, -math.Pi/4, 0))
}

func TestFitArcsQuarterCircle(t *testing.T) {
	b := quarterTwice()
	m := supersampled(b, win80)
	out, _ := FitArcs(b, m, win80, 0.5, Options{})
	if out.N() != 5 {
		t.Fatalf("%d segments, want 5", out.N())
	}
	// The radius is refitted against the (box-filtered) mask, so the
	// tangent points land within a few hundredths of a pixel.
	c := out.Segment(1)
	if !near(c[0], v(50, 10), 0.1) || !near(c[3], v(70, 30), 0.1) {
		t.Fatalf("arc ends %v %v", c[0], c[3])
	}
	r := c[0].Sub(v(50, 30)).Len()
	want := 4.0 / 3 * math.Tan(math.Pi/8)
	for _, h := range []float64{c[1].Sub(c[0]).Len(), c[2].Sub(c[3]).Len()} {
		if ratio := h / r; math.Abs(ratio-want) > 0.002 {
			t.Fatalf("handle ratio %v, want %v", ratio, want)
		}
	}
}

func TestFitArcsKeepsFixedEnds(t *testing.T) {
	b := quarterTwice()
	m := supersampled(b, win80)
	// The run's end anchors are fixed: the arc keeps them exactly (a
	// free end would slide to the tangent point the mask prefers).
	o := Options{Fixed: []bool{false, true, false, true, false, false}}
	out, ro := FitArcs(b, m, win80, 0.5, o)
	if out.N() != 5 {
		t.Fatalf("%d segments, want 5", out.N())
	}
	if out.Pts[3] != b.Pts[3] || out.Pts[6] != b.Pts[9] {
		t.Fatalf("fixed ends moved: %v %v", out.Pts[3], out.Pts[6])
	}
	if len(ro.Fixed) != 5 || !ro.Fixed[1] || !ro.Fixed[2] || ro.Fixed[0] || ro.Fixed[3] || ro.Fixed[4] {
		t.Fatalf("fixed flags %v", ro.Fixed)
	}
}

func TestFitArcsRevertsOnEnergyRise(t *testing.T) {
	// A quarter ellipse (semi-axes 19.2 and 20) between fixed ends is
	// within tolerance of a circle, but the mask is the ellipse exactly,
	// so no circle through those ends fits it as well.
	k := 4.0 / 3 * math.Tan(math.Pi/8)
	b := FromSegments([]fit.Segment{
		ln(v(10, 10), v(50, 10)),
		cubic(v(50, 10), v(50+k*19.2, 10), v(69.2, 30-k*20), v(69.2, 30)),
		ln(v(69.2, 30), v(69.2, 70)), ln(v(69.2, 70), v(10, 70)), ln(v(10, 70), v(10, 10)),
	})
	m := maskOf(b, win80)
	out, _ := FitArcs(b, m, win80, 0.5, Options{Fixed: []bool{false, true, true, false, false}})
	if out.N() != b.N() {
		t.Fatalf("%d segments, want %d (snap reverted)", out.N(), b.N())
	}
	for i := range b.Pts {
		if out.Pts[i] != b.Pts[i] {
			t.Fatalf("point %d moved", i)
		}
	}
}

// roundedRectRadii is a 100×100 rounded rectangle at (10,10) with the
// given corner radii, clockwise from the top-left corner.
func roundedRectRadii(r [4]float64) Bezigon {
	tl, tr, br, bl := r[0], r[1], r[2], r[3]
	return FromSegments([]fit.Segment{
		ln(v(10+tl, 10), v(110-tr, 10)),
		arcSeg(v(110-tr, 10+tr), tr, -math.Pi/2, 0),
		ln(v(110, 10+tr), v(110, 110-br)),
		arcSeg(v(110-br, 110-br), br, 0, math.Pi/2),
		ln(v(110-br, 110), v(10+bl, 110)),
		arcSeg(v(10+bl, 110-bl), bl, math.Pi/2, math.Pi),
		ln(v(10, 110-bl), v(10, 10+tl)),
		arcSeg(v(10+tl, 10+tl), tl, math.Pi, 3*math.Pi/2),
	})
}

func arcRadius(c fit.Cubic) float64 {
	return c[1].Sub(c[0]).Len() / (4.0 / 3 * math.Tan(math.Pi/8))
}

func TestCommonRadiusFourCorners(t *testing.T) {
	win := Window{0, 0, 120, 120}
	m := supersampled(roundedRectRadii([4]float64{24, 24, 24, 24}), win)
	b := roundedRectRadii([4]float64{23.5, 24, 24.4, 24.1})
	out, _ := CommonRadius(b, m, win, 0.5, Options{})
	if out.N() != 8 {
		t.Fatalf("%d segments", out.N())
	}
	for j := 1; j < 8; j += 2 {
		if r := arcRadius(out.Segment(j)); math.Abs(r-24) > 0.05 {
			t.Errorf("corner %d radius %v, want 24", j/2, r)
		}
	}
}

func rectBezigon(cs [4]geom.Vec2) Bezigon {
	return FromSegments([]fit.Segment{ln(cs[0], cs[1]), ln(cs[1], cs[2]), ln(cs[2], cs[3]), ln(cs[3], cs[0])})
}

func TestFitRectangleRotated(t *testing.T) {
	win := Window{0, 0, 256, 256}
	true4 := corpus.RectRotCorners()
	m := supersampled(rectBezigon(true4), win)
	noisy := true4
	for i, d := range [4]geom.Vec2{{X: 0.06, Y: -0.05}, {X: -0.07, Y: 0.04}, {X: 0.05, Y: 0.06}, {X: -0.04, Y: -0.07}} {
		noisy[i] = noisy[i].Add(d)
	}
	out, ok := FitRectangle(rectBezigon(noisy), m, win, 0.5)
	if !ok {
		t.Fatal("rectangle not recognised")
	}
	for i := range true4 {
		if d := out.Pts[3*i].Sub(true4[i]).Len(); d > 0.1 {
			t.Errorf("corner %d off by %v", i, d)
		}
	}
}

func TestRectangleRejectsTrapezoid(t *testing.T) {
	win := Window{0, 0, 128, 128}
	cs := [4]geom.Vec2{{X: 10, Y: 10}, {X: 110, Y: 10}, {X: 112, Y: 90}, {X: 8, Y: 90}}
	b := rectBezigon(cs)
	if _, ok := FitRectangle(b, maskOf(b, win), win, 0.5); ok {
		t.Fatal("trapezoid taken for a rectangle")
	}
}

func TestRectangleAxisSnap(t *testing.T) {
	win := Window{0, 0, 100, 100}
	axis := [4]geom.Vec2{{X: 30, Y: 35}, {X: 70, Y: 35}, {X: 70, Y: 65}, {X: 30, Y: 65}}
	m := supersampled(rectBezigon(axis), win)
	a := 0.6 * math.Pi / 180
	var rot [4]geom.Vec2
	for i, c := range axis {
		d := c.Sub(v(50, 50))
		rot[i] = v(50+d.X*math.Cos(a)-d.Y*math.Sin(a), 50+d.X*math.Sin(a)+d.Y*math.Cos(a))
	}
	out, ok := FitRectangle(rectBezigon(rot), m, win, 0.5)
	if !ok {
		t.Fatal("rectangle not recognised")
	}
	if out.Pts[0].Y != out.Pts[3].Y || out.Pts[0].X != out.Pts[9].X {
		t.Fatalf("not axis-aligned: %v %v %v", out.Pts[0], out.Pts[3], out.Pts[9])
	}
	for i := range axis {
		if d := out.Pts[3*i].Sub(axis[i]).Len(); d > 0.1 {
			t.Errorf("corner %d off by %v", i, d)
		}
	}
}

func TestPrimitivesSkipTinyLoops(t *testing.T) {
	for _, b := range []Bezigon{
		{Pts: []geom.Vec2{v(10, 10), v(20, 0), v(30, 20)}},
		{Pts: []geom.Vec2{v(10, 10), v(20, 0), v(30, 0), v(30, 10), v(20, 20), v(10, 20)}},
	} {
		m := maskOf(b, win80)
		out, _ := FitArcs(b, m, win80, 0.5, Options{})
		out, _ = CommonRadius(out, m, win80, 0.5, Options{})
		if _, ok := FitRectangle(out, m, win80, 0.5); ok {
			t.Fatal("tiny loop taken for a rectangle")
		}
		if len(out.Pts) != len(b.Pts) {
			t.Fatalf("N %d → %d", b.N(), out.N())
		}
		for i := range b.Pts {
			if out.Pts[i] != b.Pts[i] {
				t.Fatalf("point %d moved", i)
			}
		}
	}
}

// TestFineEnergyPrefersTrueRadius: judged by the Flatten-chord polygon a
// circle looks better 0.08 % too large (the polygon's area matches then);
// judged by fineDataEnergy the true radius wins.
func TestFineEnergyPrefersTrueRadius(t *testing.T) {
	win := Window{0, 0, 256, 256}
	truth := Circle(v(128, 128), 96, v(224, 128), true)
	m := supersampled(truth, win)
	big := Circle(v(128, 128), 96/chordFactor(2*math.Pi/(4*Flatten)), v(224.08, 128), true)
	l0 := truth.Length()
	if localDataEnergy(truth, m, win, l0) < localDataEnergy(big, m, win, l0) {
		t.Fatalf("the coarse energy should prefer the inflated circle")
	}
	if fineDataEnergy(truth, m, win, l0) >= fineDataEnergy(big, m, win, l0) {
		t.Fatalf("the fine energy prefers the inflated circle")
	}
}

// TestCommonRadiusKeepsTrueCorners: three corners already on the mask's
// radius are not moved to a common radius that one odd corner pulls up.
func TestCommonRadiusKeepsTrueCorners(t *testing.T) {
	win := Window{0, 0, 120, 120}
	m := supersampled(roundedRectRadii([4]float64{24, 24, 24, 24}), win)
	b := roundedRectRadii([4]float64{24, 24, 24, 24.3})
	out, _ := CommonRadius(b, m, win, 0.5, Options{})
	if out.N() != 8 {
		t.Fatalf("%d segments", out.N())
	}
	for _, j := range []int{1, 3, 7} {
		if r := arcRadius(out.Segment(j)); math.Abs(r-24) > 0.01 {
			t.Errorf("segment %d radius %v, want 24 kept", j, r)
		}
	}
	if r := arcRadius(out.Segment(5)); math.Abs(r-24) >= 0.3 {
		t.Errorf("the odd corner stays at %v", r)
	}
}

// TestStraightenedCollapsesNearLines: a segment whose handles sit within
// lineTol of its chord becomes that chord (handles at the thirds); a
// curved one is left alone.
func TestStraightenedCollapsesNearLines(t *testing.T) {
	bowed := cubic(v(0, 0), v(30, 0.1), v(60, -0.1), v(90, 0))
	b := FromSegments([]fit.Segment{bowed, cubic(v(90, 0), v(90, 50), v(0, 50), v(0, 0))})
	out := b.Straightened(0.25)
	if got := out.Segment(0); !near(got[1], v(30, 0), 1e-9) || !near(got[2], v(60, 0), 1e-9) {
		t.Fatalf("bowed line straightened to %v", got)
	}
	if got := out.Segment(1); got != b.Segment(1) {
		t.Fatalf("curved segment changed to %v", got)
	}
	if got := b.Straightened(0.05).Segment(0); got != bowed.P {
		t.Fatalf("a bow beyond lineTol was straightened: %v", got)
	}
}

// TestFitArcsBesideBowedLine: a refined "line" bows 0.1 px within its
// band with its ends 0.08 px apart across the true edge, as refinement
// leaves them; once straightened to what is emitted, the corner beside
// it snaps to the arc the mask shows.
func TestFitArcsBesideBowedLine(t *testing.T) {
	win := Window{0, 0, 120, 120}
	m := supersampled(roundedRectRadii([4]float64{24, 24, 24, 24}), win)
	segs := roundedRectRadii([4]float64{24, 24, 24, 24}).Segments(0)
	// Bottom line, drawn right to left: from (86,110) to (34,110). The
	// corner (centre (34,86)) is a refined one as amd64 left it on
	// rounded-rect: on its circle in the middle, its ends 3 px early
	// along the bottom and 1 px late down the left, a 7° kink.
	segs[4] = cubic(v(86.04, 110.02), v(70, 109.92), v(50, 110.1), v(31.02, 109.93))
	segs[5] = cubic(v(31.02, 109.93), v(20.62, 108.57), v(10.46, 99.72), v(10.01, 86.99))
	segs[6] = ln(v(10.01, 86.99), v(10, 34))
	b := FromSegments(segs)
	out, _ := FitArcs(b.Straightened(0.25), m, win, 0.5, Options{})
	if out.N() != 8 {
		t.Fatalf("%d segments", out.N())
	}
	// The arc is tangent to the lines the refinement left, which tilt by
	// 0.09 px, so it lands a little inside the true corner.
	c := out.Segment(5)
	if r := arcRadius(c); math.Abs(r-24) > 0.2 {
		t.Fatalf("corner radius %v, want 24", r)
	}
	if !near(c[0], v(34, 110), 0.25) {
		t.Fatalf("corner starts at %v, want the tangent point (34,110)", c[0])
	}
}

// TestFitRectangleDiamond: a rectangle at 45° is the one orientation the
// 4θ mean folds onto the axis test; it must snap like any other angle.
func TestFitRectangleDiamond(t *testing.T) {
	win := Window{0, 0, 256, 256}
	a := 45 * math.Pi / 180
	c, s := math.Cos(a), math.Sin(a)
	var true4 [4]geom.Vec2
	for i, d := range [4][2]float64{{-60, -40}, {60, -40}, {60, 40}, {-60, 40}} {
		true4[i] = geom.Vec2{X: 128 + d[0]*c - d[1]*s, Y: 128 + d[0]*s + d[1]*c}
	}
	m := supersampled(rectBezigon(true4), win)
	noisy := true4
	for i, d := range [4]geom.Vec2{{X: 0.06, Y: -0.05}, {X: -0.07, Y: 0.04}, {X: 0.05, Y: 0.06}, {X: -0.04, Y: -0.07}} {
		noisy[i] = noisy[i].Add(d)
	}
	out, ok := FitRectangle(rectBezigon(noisy), m, win, 0.5)
	if !ok {
		t.Fatal("diamond not recognised")
	}
	for i := range true4 {
		if d := out.Pts[3*i].Sub(true4[i]).Len(); d > 0.1 {
			t.Errorf("corner %d off by %v", i, d)
		}
	}
}
