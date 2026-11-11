package gradient

import (
	"image/color"
	"math"
	"testing"
)

func rampMoments() moments {
	var m moments
	for y := 0; y < 8; y++ {
		for x := 0; x < 64; x++ {
			v := float64(x) / 63
			m.add(float64(x)+0.5, float64(y)+0.5, [3]float64{v, v, v})
		}
	}
	return m
}

func TestFitHorizontalRamp(t *testing.T) {
	m := rampMoments()
	f := m.fit()
	if !f.ok || f.rms > 1e-9 {
		t.Fatalf("rms %g ok %v", f.rms, f.ok)
	}
	if math.Abs(math.Abs(f.u[0])-1) > 1e-9 {
		t.Errorf("direction %v, want ±x", f.u)
	}
	if want := 1.0 / 63 * math.Sqrt(3); math.Abs(f.slope-want) > 1e-6 {
		t.Errorf("slope %g want %g", f.slope, want)
	}
}

func TestFitFlatIsZeroSlope(t *testing.T) {
	var m moments
	for p := 0; p < 100; p++ {
		m.add(float64(p%10)+0.5, float64(p/10)+0.5, [3]float64{0.3, 0.3, 0.3})
	}
	if f := m.fit(); !f.ok || f.slope > 1e-12 || f.rms > 1e-9 {
		t.Errorf("%+v", f)
	}
}

func TestFitCollinearNotOK(t *testing.T) {
	var m moments
	for x := 0; x < 10; x++ {
		m.add(float64(x)+0.5, 3.5, [3]float64{float64(x) / 9, 0, 0})
	}
	if m.fit().ok {
		t.Error("a single row of pixels must not fit a plane")
	}
}

func TestMergeMomentsEqualsAdding(t *testing.T) {
	var a, b, all moments
	for p := 0; p < 100; p++ {
		x, y := float64(p%10)+0.5, float64(p/10)+0.5
		c := [3]float64{x / 10, y / 10, (x + y) / 20}
		all.add(x, y, c)
		if p < 50 {
			a.add(x, y, c)
		} else {
			b.add(x, y, c)
		}
	}
	a.merge(&b)
	if a.n != all.n || math.Abs(a.sxx-all.sxx) > 1e-9 || math.Abs(a.scc[1]-all.scc[1]) > 1e-9 || math.Abs(a.scx[2]-all.scx[2]) > 1e-9 {
		t.Errorf("merged %+v\nall %+v", a, all)
	}
}

func TestRampStops(t *testing.T) {
	m := rampMoments()
	f := m.fit()
	b := newBins(0.5, 63.5)
	for y := 0; y < 8; y++ {
		for x := 0; x < 64; x++ {
			v := float64(x) / 63
			b.add(float64(x)+0.5, [3]float64{v, v, v})
		}
	}
	r := rampFrom(&m, field{u: f.u}, b, 0.03)
	if r.Stops[0].Colour != (color.NRGBA{0, 0, 0, 255}) || r.Stops[1].Colour != (color.NRGBA{255, 255, 255, 255}) {
		t.Errorf("stops %+v", r.Stops)
	}
	if math.Abs(r.X0-0.5) > 1e-6 || math.Abs(r.X1-63.5) > 1e-6 || math.Abs(r.Y0-4) > 1e-6 || math.Abs(r.Y1-4) > 1e-6 {
		t.Errorf("ends %+v", r)
	}
}

func TestAlongMatchesFit(t *testing.T) {
	m := rampMoments()
	f := m.fit()
	g := m.along(f.u)
	if !g.ok || g.u != f.u || math.Abs(g.rms-f.rms) > 1e-12 || math.Abs(g.slope-f.slope) > 1e-12 || math.Abs(g.extent-f.extent) > 1e-9 || g.a != f.a || g.b != f.b {
		t.Errorf("along %+v\nfit   %+v", g, f)
	}
}

func TestAlongNoSpreadNotOK(t *testing.T) {
	m := rampMoments()
	if g := m.along([2]float64{0, 1}); g.ok && g.extent > 1e-9 && g.slope > 1e-9 {
		t.Errorf("along y on a horizontal ramp: slope %g extent %g", g.slope, g.extent)
	}
	var one moments
	one.add(0.5, 0.5, [3]float64{1, 1, 1})
	if one.along([2]float64{1, 0}).ok {
		t.Error("one pixel must not fit")
	}
}

// linearBins fills bins over a w-pixel exact grey ramp 0..1 (v = x/(w-1)),
// skipping x in [x0,x1) when x1 > x0, and returns the moments and fit too.
func linearBins(w, x0, x1 int) (*moments, fitted, *bins) {
	var m moments
	b := newBins(0.5, float64(w)-0.5)
	for y := 0; y < 8; y++ {
		for x := 0; x < w; x++ {
			if x >= x0 && x < x1 {
				continue
			}
			v := float64(x) / float64(w-1)
			m.add(float64(x)+0.5, float64(y)+0.5, [3]float64{v, v, v})
			b.add(float64(x)+0.5, [3]float64{v, v, v})
		}
	}
	return &m, m.fit(), b
}

func TestRampStopsLinearIsTwo(t *testing.T) {
	m, f, b := linearBins(256, 0, 0)
	if b.nb != 32 {
		t.Fatalf("nb %d want 32", b.nb)
	}
	r := rampFrom(m, field{u: f.u}, b, 0.03)
	if r == nil {
		t.Fatal("nil ramp")
	}
	want := []Stop{{Offset: 0, Colour: color.NRGBA{0, 0, 0, 255}}, {Offset: 1, Colour: color.NRGBA{255, 255, 255, 255}}}
	if len(r.Stops) != 2 || r.Stops[0] != want[0] || r.Stops[1] != want[1] {
		t.Errorf("stops %+v", r.Stops)
	}
	if math.Abs(r.X0-0.5) > 1e-6 || math.Abs(r.X1-255.5) > 1e-6 || math.Abs(r.Y0-4) > 1e-6 || math.Abs(r.Y1-4) > 1e-6 {
		t.Errorf("ends %+v", r)
	}
}

func TestStopsCurvedKeepsBends(t *testing.T) {
	var m moments
	b := newBins(0.5, 255.5)
	curve := func(t float64) float64 { return float64((t / 255) * (t / 255)) }
	for y := 0; y < 8; y++ {
		for x := 0; x < 256; x++ {
			v := curve(float64(x))
			m.add(float64(x)+0.5, float64(y)+0.5, [3]float64{v, v, v})
			b.add(float64(x)+0.5, [3]float64{v, v, v})
		}
	}
	const tol = 0.03
	stops, _ := b.stops(tol)
	if len(stops) < 3 {
		t.Fatalf("%d stops, want a bent ramp: %+v", len(stops), stops)
	}
	for _, s := range stops {
		want := curve(0.5 + float64(s.Offset*255))
		if d := math.Abs(float64(s.Colour.G)/255 - want); d > tol {
			t.Errorf("stop %+v is %.3f from the curve (%.3f)", s, d, want)
		}
	}
	if stops[0].Offset != 0 || stops[len(stops)-1].Offset != 1 {
		t.Errorf("ends %+v %+v", stops[0], stops[len(stops)-1])
	}
}

func TestStopsSkipEmptyBins(t *testing.T) {
	// Bins are 8 px wide over 256 px; x in [24,40) empties bins 3 and 4.
	_, _, b := linearBins(256, 24, 40)
	raw := b.raw()
	if len(raw) != 30+2 {
		t.Fatalf("%d raw stops, want 30 bins + 2 ends", len(raw))
	}
	for i := 1; i < len(raw); i++ {
		if raw[i].offset <= raw[i-1].offset || raw[i].colour[0] < raw[i-1].colour[0] {
			t.Errorf("raw stops not monotone at %d: %+v %+v", i, raw[i-1], raw[i])
		}
	}
	if stops, _ := b.stops(0.03); len(stops) != 2 {
		t.Errorf("a linear ramp with a hole still simplifies to 2 stops, got %+v", stops)
	}
}

func TestStopsCapped(t *testing.T) {
	_, _, b := linearBins(4000, 0, 0)
	if b.nb != 64 {
		t.Errorf("nb %d want 64", b.nb)
	}
	if raw := b.raw(); len(raw) > 66 {
		t.Errorf("%d raw stops", len(raw))
	}
}

func TestStopsNeedTwoBins(t *testing.T) {
	b := newBins(0.5, 4.5)
	for x := 0; x < 5; x++ {
		b.add(float64(x)+0.5, [3]float64{0.5, 0.5, 0.5})
	}
	if stops, _ := b.stops(0.03); b.nb != 1 || stops != nil {
		t.Errorf("nb %d stops %+v", b.nb, stops)
	}
}

func TestRampFromPocketInEndBinIsNil(t *testing.T) {
	// A flat region whose last bin holds a pocket of a nearby colour:
	// the extrapolated end would amplify it past tol, the samples do not.
	var m moments
	b := newBins(0.5, 71.5)
	for y := 0; y < 48; y++ {
		for x := 0; x < 72; x++ {
			c := [3]float64{130.0 / 255, 40.0 / 255, 160.0 / 255}
			if x >= 64 && y < 24 {
				c = [3]float64{136.0 / 255, 50.0 / 255, 164.0 / 255}
			}
			m.add(float64(x)+0.5, float64(y)+0.5, c)
			b.add(float64(x)+0.5, c)
		}
	}
	f := m.fit()
	if !f.ok || f.rms > 0.03 {
		t.Fatalf("fixture must be a ramp candidate: %+v", f)
	}
	if r := rampFrom(&m, field{u: f.u}, b, 0.03); r != nil {
		t.Errorf("pocket gave %+v", r.Stops)
	}
}

func TestRampFromFlatIsNil(t *testing.T) {
	var m moments
	b := newBins(0.5, 63.5)
	for y := 0; y < 8; y++ {
		for x := 0; x < 64; x++ {
			m.add(float64(x)+0.5, float64(y)+0.5, [3]float64{0.5, 0.5, 0.5})
			b.add(float64(x)+0.5, [3]float64{0.5, 0.5, 0.5})
		}
	}
	if r := rampFrom(&m, field{u: m.fit().u}, b, 0.03); r != nil {
		t.Errorf("flat gave %+v", r)
	}
}

// A field whose colour depends on the distance from a point has colour
// gradients that all point through that point; the centre solved from
// the gradient moments is that point.
func TestCentreOfRadialField(t *testing.T) {
	const w, h = 120, 100
	cx, cy := 37.5, 61.5
	at := func(x, y float64) [3]float64 {
		r := math.Hypot(x-cx, y-cy)
		v := 0.2 + 0.006*r
		return [3]float64{v, v * 0.8, v * 0.5}
	}
	var m moments
	for y := 1; y < h-1; y++ {
		for x := 1; x < w-1; x++ {
			px, py := float64(x)+0.5, float64(y)+0.5
			m.add(px, py, at(px, py))
			m.addGrad(px, py, half(at(px+1, py), at(px-1, py)), half(at(px, py+1), at(px, py-1)))
		}
	}
	gx, gy, ok := m.centre()
	if !ok {
		t.Fatal("no centre")
	}
	if math.Abs(gx-cx) > 0.5 || math.Abs(gy-cy) > 0.5 {
		t.Fatalf("centre (%.2f,%.2f), want (%.1f,%.1f)", gx, gy, cx, cy)
	}
}

// A linear ramp's gradients are parallel: no centre.
func TestCentreOfLinearFieldIsNone(t *testing.T) {
	var m moments
	for y := 1; y < 50; y++ {
		for x := 1; x < 50; x++ {
			px, py := float64(x)+0.5, float64(y)+0.5
			m.add(px, py, [3]float64{px / 100, px / 100, px / 100})
			m.addGrad(px, py, [3]float64{0.01, 0.01, 0.01}, [3]float64{})
		}
	}
	if _, _, ok := m.centre(); ok {
		t.Fatal("linear field gave a centre")
	}
}
