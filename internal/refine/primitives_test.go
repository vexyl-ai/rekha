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

func TestSnapPiecesQuarterCircle(t *testing.T) {
	b := quarterTwice()
	m := supersampled(b, win80)
	out, _ := SnapPieces(b, m, win80, 0.5, Options{})
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

func TestSnapPiecesKeepsFixedEnds(t *testing.T) {
	b := quarterTwice()
	m := supersampled(b, win80)
	// The run's end anchors are fixed: the arc keeps them exactly (a
	// free end would slide to the tangent point the mask prefers).
	o := Options{Fixed: []bool{false, true, false, true, false, false}}
	out, ro := SnapPieces(b, m, win80, 0.5, o)
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

func TestSnapPiecesRevertsOnEnergyRise(t *testing.T) {
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
	out, _ := SnapPieces(b, m, win80, 0.5, Options{Fixed: []bool{false, true, true, false, false}})
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

// curvedSegments are the indices of b's segments that are not straight.
// The piece pass rotates a loop to start at a piece boundary, so tests
// find arcs by shape, not by position.
func curvedSegments(b Bezigon) []int {
	var out []int
	for j := 0; j < b.N(); j++ {
		if !straightCubic(b.Segment(j), 0.25) {
			out = append(out, j)
		}
	}
	return out
}

// cornerNear is the curved segment of b whose chord midpoint is nearest p.
func cornerNear(t *testing.T, b Bezigon, p geom.Vec2) fit.Cubic {
	t.Helper()
	best, d := -1, math.Inf(1)
	for _, j := range curvedSegments(b) {
		c := b.Segment(j)
		if dd := c[0].Add(c[3]).Scale(0.5).Sub(p).Len(); dd < d {
			best, d = j, dd
		}
	}
	if best < 0 {
		t.Fatal("no curved segment")
	}
	return b.Segment(best)
}

func TestCommonRadiusFourCorners(t *testing.T) {
	win := Window{0, 0, 120, 120}
	m := supersampled(roundedRectRadii([4]float64{24, 24, 24, 24}), win)
	b := roundedRectRadii([4]float64{23.5, 24, 24.4, 24.1})
	out, _ := CommonRadius(b, m, win, 0.5, Options{})
	if out.N() != 8 {
		t.Fatalf("%d segments", out.N())
	}
	curved := curvedSegments(out)
	if len(curved) != 4 {
		t.Fatalf("curved segments %v, want 4", curved)
	}
	for _, j := range curved {
		if r := arcRadius(out.Segment(j)); math.Abs(r-24) > 0.05 {
			t.Errorf("segment %d radius %v, want 24", j, r)
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
		out, _ := SnapPieces(b, m, win80, 0.5, Options{})
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
	for _, mid := range []geom.Vec2{v(22, 22), v(98, 22), v(98, 98)} {
		if r := arcRadius(cornerNear(t, out, mid)); math.Abs(r-24) > 0.01 {
			t.Errorf("corner at %v radius %v, want 24 kept", mid, r)
		}
	}
	if r := arcRadius(cornerNear(t, out, v(22, 98))); math.Abs(r-24) >= 0.3 {
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

// TestSnapPiecesBesideBowedLine: a refined "line" bows 0.1 px within its
// band with its ends 0.08 px apart across the true edge, as refinement
// leaves them; once straightened to what is emitted, the corner beside
// it snaps to the arc the mask shows.
func TestSnapPiecesBesideBowedLine(t *testing.T) {
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
	out, _ := SnapPieces(b.Straightened(0.25), m, win, 0.5, Options{})
	if out.N() != 8 {
		t.Fatalf("%d segments", out.N())
	}
	// The arc is tangent to the lines the refinement left, which tilt by
	// 0.09 px, so it lands a little inside the true corner.
	c := cornerNear(t, out, v(22, 98))
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

// annularSector is the band between r = 60 and r = 100 about (128,128)
// within ±60° of +x: outer arc as three 40° cubics, radial line, inner
// arc back as three cubics, radial line; anchors nudged by d.
func annularSector(d float64) Bezigon {
	c := v(128, 128)
	a := math.Pi / 3
	segs := nudgedArc(c, 100, -a, a, 3, d)
	at := func(r, ang float64) geom.Vec2 { return v(c.X+r*math.Cos(ang), c.Y+r*math.Sin(ang)) }
	segs = append(segs, ln(segs[len(segs)-1].P[3], at(60, a)))
	inner := nudgedArc(c, 60, a, -a, 3, d)
	inner[0].P[0] = segs[len(segs)-1].P[3]
	segs = append(segs, inner...)
	segs = append(segs, ln(segs[len(segs)-1].P[3], segs[0].P[0]))
	return FromSegments(segs)
}

var win256 = Window{0, 0, 256, 256}

func countKinds(b Bezigon, tol float64) (lines, curved int) {
	for j := 0; j < b.N(); j++ {
		if straightCubic(b.Segment(j), tol) {
			lines++
		} else {
			curved++
		}
	}
	return
}

func TestSnapPiecesHalfDisc(t *testing.T) {
	m := supersampled(halfDisc(0), win256)
	out, _ := SnapPieces(halfDisc(0.2), m, win256, 0.5, Options{})
	lines, curved := countKinds(out, 0.25)
	if out.N() != 3 || lines != 1 || curved != 2 {
		t.Fatalf("%d segments, %d lines, %d curved", out.N(), lines, curved)
	}
	for j := 0; j < out.N(); j++ {
		c := out.Segment(j)
		if straightCubic(c, 0.25) {
			continue
		}
		for _, p := range []geom.Vec2{c[0], c[3]} {
			if r := p.Sub(v(128, 128)).Len(); math.Abs(r-80) > 0.1 {
				t.Errorf("anchor %v is %.3f from the centre", p, r)
			}
		}
	}
	for _, p := range []geom.Vec2{out.Pts[0], out.Pts[3]} {
		if math.Abs(p.X-128) > 0.1 {
			t.Errorf("line end %v is off x = 128", p)
		}
	}
}

func TestSnapPiecesAnnularSector(t *testing.T) {
	m := supersampled(annularSector(0), win256)
	out, _ := SnapPieces(annularSector(0.2), m, win256, 0.5, Options{})
	lines, curved := countKinds(out, 0.25)
	if out.N() != 6 || lines != 2 || curved != 4 {
		t.Fatalf("%d segments, %d lines, %d curved", out.N(), lines, curved)
	}
	var centres []geom.Vec2
	for j := 0; j < out.N(); j++ {
		c := out.Segment(j)
		if straightCubic(c, 0.25) {
			continue
		}
		centre, r, ok := kasa([]geom.Vec2{c[0], c.At(0.25), c.At(0.5), c.At(0.75), c[3]})
		if !ok || (math.Abs(r-60) > 0.2 && math.Abs(r-100) > 0.2) {
			t.Errorf("segment %d radius %.3f", j, r)
		}
		centres = append(centres, centre)
	}
	for _, c := range centres[1:] {
		if !near(c, centres[0], 0.2) {
			t.Errorf("centres %v and %v differ", centres[0], c)
		}
	}
}

func TestSnapPiecesCollapsesCollinear(t *testing.T) {
	b := FromSegments([]fit.Segment{
		ln(v(10, 10), v(30, 10)), ln(v(30, 10), v(50, 10)), ln(v(50, 10), v(70, 10)),
		ln(v(70, 10), v(70, 70)), ln(v(70, 70), v(10, 70)), ln(v(10, 70), v(10, 10)),
	})
	m := supersampled(b, win80)
	out, _ := SnapPieces(b, m, win80, 0.5, Options{})
	if out.N() != 4 {
		t.Fatalf("%d segments, want 4", out.N())
	}
}

func TestSnapPiecesAllFixedUnchanged(t *testing.T) {
	b := quarterTwice()
	m := supersampled(b, win80)
	fixed, frozen := make([]bool, b.N()), make([]bool, b.N())
	for i := range fixed {
		fixed[i], frozen[i] = true, true
	}
	out, _ := SnapPieces(b, m, win80, 0.5, Options{Fixed: fixed, Frozen: frozen})
	out, _ = CommonRadius(out, m, win80, 0.5, Options{Fixed: fixed, Frozen: frozen})
	if len(out.Pts) != len(b.Pts) {
		t.Fatalf("N %d → %d", b.N(), out.N())
	}
	for i := range b.Pts {
		if out.Pts[i] != b.Pts[i] {
			t.Fatalf("point %d moved", i)
		}
	}
}

func TestSnapPiecesChordBeyondDiameter(t *testing.T) {
	// A semicircle whose fixed end anchors sit 0.4 px outside the circle:
	// its chord is longer than twice the Kåsa radius, so the circle
	// through the ends does not exist at that radius.
	segs := []fit.Segment{ln(v(128, 208.4), v(128, 47.6))}
	arc := nudgedArc(v(128, 128), 80, -math.Pi/2, math.Pi/2, 4, 0)
	arc[0].P[0], arc[3].P[3] = v(128, 47.6), v(128, 208.4)
	segs = append(segs, arc...)
	b := FromSegments(segs)
	m := supersampled(halfDisc(0), win256)
	out, _ := SnapPieces(b, m, win256, 0.5, Options{Fixed: []bool{true, true, false, false, false}})
	for i, p := range out.Pts {
		if math.IsNaN(p.X) || math.IsNaN(p.Y) {
			t.Fatalf("point %d is NaN", i)
		}
	}
	if out.Pts[3] != v(128, 47.6) || out.Pts[0] != v(128, 208.4) {
		t.Fatalf("fixed ends moved: %v %v", out.Pts[3], out.Pts[0])
	}
}

// TestSnapPiecesKeepsFrozenSegment: the half-disc's flat side is frozen
// and no joint is fixed; the snap of the arc must leave that segment's
// control points exactly as they were.
func TestSnapPiecesKeepsFrozenSegment(t *testing.T) {
	m := supersampled(halfDisc(0), win256)
	in := halfDisc(0.2)
	want := in.Segment(0)
	out, _ := SnapPieces(in, m, win256, 0.5, Options{Frozen: []bool{true, false, false, false, false}})
	for j := 0; j < out.N(); j++ {
		if out.Segment(j) == want {
			return
		}
	}
	t.Fatalf("the frozen line %v is not in the output %v", want, out.Pts)
}

// twoArcCornerRect is the rounded rectangle of roundedRectRadii with its
// top-right corner drawn as two arcs meeting at a 20° corner: r = 30 over
// the first 45°, then r24 from 65° before the horizontal to it, the right
// edge following. (Tangent arcs of these radii merge into one piece
// within the tolerance; the corner keeps them apart.)
func twoArcCornerRect(r24 float64) Bezigon {
	c1 := v(80, 40)
	p := c1.Add(v(math.Cos(-math.Pi/4), math.Sin(-math.Pi/4)).Scale(30))
	a2 := -65 * math.Pi / 180
	c2 := p.Sub(v(math.Cos(a2), math.Sin(a2)).Scale(r24))
	right := c2.X + r24
	return FromSegments([]fit.Segment{
		ln(v(10+24, 10), v(80, 10)),
		arcSeg(c1, 30, -math.Pi/2, -math.Pi/4),
		arcSeg(c2, r24, a2, 0),
		ln(v(right, c2.Y), v(right, 110-24)),
		arcSeg(v(right-24, 110-24), 24, 0, math.Pi/2),
		ln(v(right-24, 110), v(10+24, 110)),
		arcSeg(v(10+24, 110-24), 24, math.Pi/2, math.Pi),
		ln(v(10, 110-24), v(10, 10+24)),
		arcSeg(v(10+24, 10+24), 24, math.Pi, 3*math.Pi/2),
	})
}

// TestCommonRadiusKeepsExactNeighbourArc: the four r ≈ 24 corners share a
// radius; the r = 30 arc beside one of them does not. Its exact cubic
// must come through the refit of its neighbour unchanged, while the
// neighbour itself is refit.
func TestCommonRadiusKeepsExactNeighbourArc(t *testing.T) {
	win := Window{0, 0, 120, 120}
	m := supersampled(twoArcCornerRect(24), win)
	in := twoArcCornerRect(24.3)
	big, member := in.Segment(1), in.Segment(2)
	out, _ := CommonRadius(in, m, win, 0.5, Options{})
	if out.N() != in.N() {
		t.Fatalf("%d segments, want %d", out.N(), in.N())
	}
	bigKept, memberKept := false, false
	for j := 0; j < out.N(); j++ {
		bigKept = bigKept || out.Segment(j) == big
		memberKept = memberKept || out.Segment(j) == member
	}
	if memberKept {
		t.Fatalf("the r = 24.3 member was not refit")
	}
	if !bigKept {
		t.Fatalf("the exact r = 30 arc %v was changed; output %v", big, out.Pts)
	}
}

// TestExactArcTellsLaidFromRefined: the cubics arcPieces lays are exact
// arcs; the same cubic with one handle moved a hundredth of a pixel, as
// a refinement leaves them, is not.
func TestExactArcTellsLaidFromRefined(t *testing.T) {
	b, _, ps := loopPieces(roundedRectRadii([4]float64{24, 24, 24, 24}), 0.5, Options{})
	for i, p := range ps {
		if p.kind != pieceArc {
			continue
		}
		c, r, ok := exactArc(b, p)
		if !ok || math.Abs(r-24) > 1e-6 {
			t.Fatalf("piece %d: exact arc not recognised (ok=%v r=%v centre=%v)", i, ok, r, c)
		}
		nudged := Bezigon{Pts: append([]geom.Vec2(nil), b.Pts...)}
		nudged.Pts[3*p.first+1] = nudged.Pts[3*p.first+1].Add(v(0.01, 0))
		if _, _, ok := exactArc(nudged, p); ok {
			t.Fatalf("piece %d: a cubic nudged 0.01 px passes as exact", i)
		}
	}
}
