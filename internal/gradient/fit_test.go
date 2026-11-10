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
	r := ramp(&m, f, 0.5, 63.5)
	if r.Stops[0].Colour != (color.NRGBA{0, 0, 0, 255}) || r.Stops[1].Colour != (color.NRGBA{255, 255, 255, 255}) {
		t.Errorf("stops %+v", r.Stops)
	}
	if math.Abs(r.X0-0.5) > 1e-6 || math.Abs(r.X1-63.5) > 1e-6 || math.Abs(r.Y0-4) > 1e-6 || math.Abs(r.Y1-4) > 1e-6 {
		t.Errorf("ends %+v", r)
	}
}
