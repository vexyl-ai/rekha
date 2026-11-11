package gradient

import (
	"math"
	"testing"
)

// sphereImage is a disc of radius rad about (cx, cy) shaded by a linear
// base ramp along x under two radial overlays: a highlight inside the
// disc and a rim light centred outside it. Pixels outside the disc are
// white. It returns the image, the disc's pixels and the true colour
// function.
func sphereImage(w, h int) (rgb []uint8, pix []int, truth func(x, y float64) [3]float64) {
	cx, cy, rad := 100.0, 100.0, 90.0
	base := func(x float64) [3]float64 {
		f := x / float64(w)
		return [3]float64{0.3 + 0.2*f, 0.3 - 0.1*f, 0.8 - 0.2*f}
	}
	over := func(c [3]float64, a float64, hc [3]float64) [3]float64 {
		return [3]float64{c[0] + a*(hc[0]-c[0]), c[1] + a*(hc[1]-c[1]), c[2] + a*(hc[2]-c[2])}
	}
	truth = func(x, y float64) [3]float64 {
		c := base(x)
		r1 := math.Hypot(x-70, y-130)
		a1 := math.Max(0, 1-r1/90)
		c = over(c, a1*a1, [3]float64{0.6, 0.7, 1})
		r2 := math.Hypot(x-260, y-100)
		a2 := math.Max(0, math.Min(1, (220-r2)/80))
		return over(c, 0.6*a2, [3]float64{0.3, 0, 0.3})
	}
	rgb = make([]uint8, 3*w*h)
	for p := range w * h {
		x, y := float64(p%w)+0.5, float64(p/w)+0.5
		c := [3]float64{1, 1, 1}
		if math.Hypot(x-cx, y-cy) <= rad {
			c = truth(x, y)
			pix = append(pix, p)
		}
		for k := range c {
			rgb[3*p+k] = uint8(math.Round(math.Max(0, math.Min(1, c[k])) * 255))
		}
	}
	return rgb, pix, truth
}

// A sphere shaded by a ramp under a highlight and a rim light is
// reproduced by the layered model within a few counts, with the
// highlight found; the rim's share may be split between the base ramp
// and the second overlay, since the decomposition is not unique.
func TestFitLayeredSphere(t *testing.T) {
	const w, h = 200, 200
	rgb, pix, _ := sphereImage(w, h)
	in := make([]bool, w*h)
	for _, p := range pix {
		in[p] = true
	}
	member := func(p int) bool { return in[p] }
	l := fitLayered(rgb, pix, member, w, h, field{u: [2]float64{1, 0}}, 0.03)
	if l == nil {
		t.Fatal("no layered model")
	}
	if l.rms > 0.015 {
		t.Fatalf("rms %.4f", l.rms)
	}
	if len(l.over) != 2 {
		t.Fatalf("%d overlays, want 2", len(l.over))
	}
	found := false
	for _, o := range l.over {
		if math.Hypot(o.cx-70, o.cy-130) <= 10 {
			found = true
		}
	}
	if !found {
		t.Errorf("no overlay centred on the highlight: %+v", l.over)
	}
}

// A plain ramp needs no overlay: none is added when it does not pay.
func TestFitLayeredPlainRampAddsNothing(t *testing.T) {
	const w, h = 64, 32
	rgb := make([]uint8, 3*w*h)
	var pix []int
	for p := range w * h {
		v := uint8(p % w * 4)
		rgb[3*p], rgb[3*p+1], rgb[3*p+2] = v, v, v
		pix = append(pix, p)
	}
	l := fitLayered(rgb, pix, func(int) bool { return true }, w, h, field{u: [2]float64{1, 0}}, 0.03)
	if l != nil && len(l.over) != 0 {
		t.Fatalf("%d overlays on a plain ramp", len(l.over))
	}
}

// A layered model becomes a ramp: the base along its field, and one
// radial overlay per layer whose stops carry the opacity profile in
// their alpha, non-increasing from the centre.
func TestLayeredRamp(t *testing.T) {
	const w, h = 200, 200
	rgb, pix, _ := sphereImage(w, h)
	in := make([]bool, w*h)
	var m moments
	for _, p := range pix {
		in[p] = true
		m.add(float64(p%w)+0.5, float64(p/w)+0.5, colourOf(rgb, p))
	}
	member := func(p int) bool { return in[p] }
	l := fitLayered(rgb, pix, member, w, h, field{u: [2]float64{1, 0}}, 0.03)
	if l == nil || len(l.over) == 0 {
		t.Fatalf("layered %+v", l)
	}
	rp := l.ramp(&m, 0.03)
	if rp == nil || rp.Radial || len(rp.Stops) < 2 {
		t.Fatalf("ramp %+v", rp)
	}
	if math.Abs(rp.Y0-rp.Y1) > 1e-9 || rp.X1 <= rp.X0 {
		t.Errorf("base line (%.1f,%.1f)-(%.1f,%.1f) does not run along x", rp.X0, rp.Y0, rp.X1, rp.Y1)
	}
	for _, st := range rp.Stops {
		if st.Colour.A != 255 {
			t.Errorf("base stop %+v is not opaque", st)
		}
	}
	if len(rp.Overlays) != len(l.over) {
		t.Fatalf("%d overlays, want %d", len(rp.Overlays), len(l.over))
	}
	for i, o := range rp.Overlays {
		if o.CX != l.over[i].cx || o.CY != l.over[i].cy || o.R != l.over[i].tmax {
			t.Errorf("overlay %d is (%.1f,%.1f) r %.1f, want (%.1f,%.1f) r %.1f", i, o.CX, o.CY, o.R, l.over[i].cx, l.over[i].cy, l.over[i].tmax)
		}
		if len(o.Stops) < 2 || o.Stops[0].Offset != 0 || o.Stops[len(o.Stops)-1].Offset != 1 {
			t.Fatalf("overlay %d stops %+v", i, o.Stops)
		}
		want := quantise(l.over[i].colour)
		for j, st := range o.Stops {
			if st.Colour.R != want.R || st.Colour.G != want.G || st.Colour.B != want.B {
				t.Errorf("overlay %d stop %d colour %v, want %v", i, j, st.Colour, want)
			}
			if j > 0 && (st.Offset <= o.Stops[j-1].Offset || st.Colour.A > o.Stops[j-1].Colour.A) {
				t.Errorf("overlay %d stops not ordered and fading: %+v", i, o.Stops)
			}
		}
		if o.Stops[0].Colour.A == 0 {
			t.Errorf("overlay %d is clear at its centre", i)
		}
	}
}
