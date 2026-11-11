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
		if math.Hypot(o.cx-70, o.cy-130) <= 15 {
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

// vignetteImage is a disc shaded by a linear base ramp along x under a
// dark rim that deepens with the distance from the disc's centre (an
// overlay whose opacity rises outwards, as an edge darkening does).
func vignetteImage(w, h int) (rgb []uint8, pix []int) {
	cx, cy, rad := 100.0, 100.0, 90.0
	rgb = make([]uint8, 3*w*h)
	for p := range w * h {
		x, y := float64(p%w)+0.5, float64(p/w)+0.5
		c := [3]float64{1, 1, 1}
		if r := math.Hypot(x-cx, y-cy); r <= rad {
			f := x / float64(w)
			c = [3]float64{0.9 - 0.2*f, 0.5 + 0.1*f, 0.2}
			a := math.Max(0, (r-40)/50)
			c = mixColour(c, [3]float64{0.5, 0, 0.3}, 0.7*a*a)
			pix = append(pix, p)
		}
		for k := range c {
			rgb[3*p+k] = uint8(math.Round(math.Max(0, math.Min(1, c[k])) * 255))
		}
	}
	return rgb, pix
}

// A disc darkened towards its rim is reproduced by the layered model
// with an overlay whose opacity rises from the centre outwards.
func TestFitLayeredVignette(t *testing.T) {
	const w, h = 200, 200
	rgb, pix := vignetteImage(w, h)
	in := make([]bool, w*h)
	for _, p := range pix {
		in[p] = true
	}
	l := fitLayered(rgb, pix, func(p int) bool { return in[p] }, w, h, field{u: [2]float64{1, 0}}, 0.03)
	if l == nil {
		t.Fatal("no layered model")
	}
	if l.rms > 0.012 {
		t.Fatalf("rms %.4f", l.rms)
	}
	found := false
	for _, o := range l.over {
		if math.Hypot(o.cx-100, o.cy-100) <= 10 && o.alpha[len(o.alpha)-1] > o.alpha[0]+0.3 {
			found = true
		}
	}
	if !found {
		t.Errorf("no outward overlay about the centre: %+v", l.over)
	}
}

// fadeImage is a disc shaded by a linear base ramp along x under a
// dark colour that fades in towards the lower-right edge along the
// diagonal (an overlay whose opacity rises along one direction, as a
// shadow on one side does).
func fadeImage(w, h int) (rgb []uint8, pix []int) {
	cx, cy, rad := 100.0, 100.0, 90.0
	rgb = make([]uint8, 3*w*h)
	for p := range w * h {
		x, y := float64(p%w)+0.5, float64(p/w)+0.5
		c := [3]float64{1, 1, 1}
		if math.Hypot(x-cx, y-cy) <= rad {
			f := x / float64(w)
			c = [3]float64{0.9 - 0.2*f, 0.5 + 0.1*f, 0.2}
			t := (x + y) / math.Sqrt2 // along the diagonal
			a := math.Max(0, (t-120)/90)
			c = mixColour(c, [3]float64{0.5, 0, 0.3}, 0.7*a*a)
			pix = append(pix, p)
		}
		for k := range c {
			rgb[3*p+k] = uint8(math.Round(math.Max(0, math.Min(1, c[k])) * 255))
		}
	}
	return rgb, pix
}

// A disc darkened towards one edge is reproduced by the layered model
// with an overlay that fades along the diagonal.
func TestFitLayeredFade(t *testing.T) {
	const w, h = 200, 200
	rgb, pix := fadeImage(w, h)
	in := make([]bool, w*h)
	for _, p := range pix {
		in[p] = true
	}
	l := fitLayered(rgb, pix, func(p int) bool { return in[p] }, w, h, field{u: [2]float64{1, 0}}, 0.03)
	if l == nil {
		t.Fatal("no layered model")
	}
	if l.rms > 0.012 {
		t.Fatalf("rms %.4f", l.rms)
	}
	// The fade is reproduced by a linear overlay along the diagonal or
	// by a radial one centred far down the diagonal, which comes to
	// the same across the disc.
	found := false
	for _, o := range l.over {
		if o.linear && math.Abs(math.Abs(o.ux)-math.Sqrt2/2) < 0.1 && o.ux*o.uy > 0 {
			found = true
		}
		if !o.linear && o.cx > 190 && o.cy > 190 && math.Abs(o.cx-o.cy) < 0.3*(o.cx+o.cy-200) {
			found = true
		}
	}
	if !found {
		t.Errorf("no overlay fading along the diagonal: %+v", l.over)
	}
}

// A linear overlay is emitted as a gradient along its direction
// through the region's centroid, from the parameter's low end to its
// high end, with the profile in the stops' alpha.
func TestLayeredRampLinearOverlay(t *testing.T) {
	l := &layered{
		fld: field{u: [2]float64{1, 0}}, tmin: 0, tmax: 100,
		base: []rawStop{{offset: 0, colour: [3]float64{0.2, 0.2, 0.2}}, {offset: 1, colour: [3]float64{0.8, 0.8, 0.8}}},
		over: []overlay{{linear: true, ux: 0, uy: 1, cx: 50, cy: 50, tmin: 20, tmax: 120,
			colour: [3]float64{1, 0, 0}, alpha: []float64{0, 0.25, 0.5, 0.75, 1}, outward: true}},
	}
	m := &moments{n: 1, sx: 50, sy: 50}
	rp := l.ramp(m, 0.03)
	if rp == nil || len(rp.Overlays) != 1 {
		t.Fatalf("ramp %+v", rp)
	}
	o := rp.Overlays[0]
	if !o.Linear || o.X0 != 50 || o.Y0 != 20 || o.X1 != 50 || o.Y1 != 120 {
		t.Fatalf("overlay runs (%g,%g)→(%g,%g), want (50,20)→(50,120): %+v", o.X0, o.Y0, o.X1, o.Y1, o)
	}
	first, last := o.Stops[0], o.Stops[len(o.Stops)-1]
	if first.Offset != 0 || last.Offset != 1 || first.Colour.A != 0 || last.Colour.A != 255 {
		t.Errorf("stops %+v, want alpha 0 at offset 0 and 255 at offset 1", o.Stops)
	}
}
