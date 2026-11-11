package gradient

import "testing"

// seamCanvas is a 64×64 image whose colour runs smoothly across x, cut
// into two labels by a staircase boundary that wobbles ±3 px around
// x = 32. With crisp set, the right label's pixels are a different
// colour, so the boundary is a real edge.
func seamCanvas(crisp bool) (rgb, labels []uint8) {
	const w, h = 64, 64
	rgb = make([]uint8, 3*w*h)
	labels = make([]uint8, w*h)
	for y := 0; y < h; y++ {
		cut := 32
		if (y/4)%2 == 1 {
			cut = 35
		}
		for x := 0; x < w; x++ {
			p := y*w + x
			v := uint8(100 + x)
			if x >= cut {
				labels[p] = 1
				if crisp {
					v += 40
				}
			}
			rgb[3*p], rgb[3*p+1], rgb[3*p+2] = v, v, v
		}
	}
	return rgb, labels
}

func seamSpread(labels []uint8, w, h int) int {
	lo, hi := w, 0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if labels[y*w+x] == 1 {
				lo, hi = min(lo, x), max(hi, x)
				break
			}
		}
	}
	return hi - lo
}

func TestSmoothSeamsStraightensSoftBoundary(t *testing.T) {
	rgb, labels := seamCanvas(false)
	ramp := func(x, _ float64) [3]float64 {
		v := (100 + x - 0.5) / 255
		return [3]float64{v, v, v}
	}
	smoothSeams(labels, rgb, 64, 64, []func(x, y float64) [3]float64{ramp, ramp}, 0.03)
	if s := seamSpread(labels, 64, 64); s > 1 {
		t.Fatalf("boundary still wobbles by %d px", s)
	}
}

func TestSmoothSeamsLeavesCrispBoundary(t *testing.T) {
	rgb, labels := seamCanvas(true)
	left := func(x, _ float64) [3]float64 {
		v := (100 + x - 0.5) / 255
		return [3]float64{v, v, v}
	}
	right := func(x, _ float64) [3]float64 {
		v := (140 + x - 0.5) / 255
		return [3]float64{v, v, v}
	}
	before := append([]uint8(nil), labels...)
	smoothSeams(labels, rgb, 64, 64, []func(x, y float64) [3]float64{left, right}, 0.03)
	for p := range labels {
		if labels[p] != before[p] {
			t.Fatalf("pixel %d relabelled across a crisp edge", p)
		}
	}
}

func TestSmoothSeamsRefusesMisfit(t *testing.T) {
	rgb, labels := seamCanvas(false)
	ramp := func(x, _ float64) [3]float64 {
		v := (100 + x - 0.5) / 255
		return [3]float64{v, v, v}
	}
	wrong := func(_, _ float64) [3]float64 { return [3]float64{0, 0, 0} }
	before := append([]uint8(nil), labels...)
	smoothSeams(labels, rgb, 64, 64, []func(x, y float64) [3]float64{ramp, wrong}, 0.03)
	for p := range labels {
		if labels[p] == 1 && before[p] == 0 {
			t.Fatalf("pixel %d handed to a model that misfits it", p)
		}
	}
}

func TestSmoothSeamsAbsorbsStrandedPixel(t *testing.T) {
	rgb, labels := seamCanvas(false)
	// A lone crisp pixel of the left label inside the right region.
	p := 10*64 + 50
	labels[p] = 0
	rgb[3*p], rgb[3*p+1], rgb[3*p+2] = 190, 190, 190
	ramp := func(x, _ float64) [3]float64 {
		v := (100 + x - 0.5) / 255
		return [3]float64{v, v, v}
	}
	smoothSeams(labels, rgb, 64, 64, []func(x, y float64) [3]float64{ramp, ramp}, 0.03)
	if labels[p] != 1 {
		t.Fatal("stranded pixel kept its own label")
	}
}

func TestSmoothSeamsAbsorbsStrandIntoFlat(t *testing.T) {
	rgb, labels := seamCanvas(false)
	// A lone ramp pixel inside a flat label (no model) is a path of its
	// own; it goes to the flat label.
	p := 10*64 + 50
	labels[p] = 0
	ramp := func(x, _ float64) [3]float64 {
		v := (100 + x - 0.5) / 255
		return [3]float64{v, v, v}
	}
	smoothSeams(labels, rgb, 64, 64, []func(x, y float64) [3]float64{ramp, nil}, 0.03)
	if labels[p] != 1 {
		t.Fatal("stranded pixel kept its own label beside a flat one")
	}
}

func TestSmoothSeamsStrandFollowsItsNeighbours(t *testing.T) {
	rgb, labels := seamCanvas(false)
	// A lone pixel of the left label ringed by crisp pixels of the right
	// label, deep inside the left region: the window around it is mostly
	// its own label, but its neighbours are what it would join.
	cx, cy := 10, 30
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if dx != 0 || dy != 0 {
				p := (cy+dy)*64 + cx + dx
				labels[p] = 1
				rgb[3*p], rgb[3*p+1], rgb[3*p+2] = 190, 190, 190
			}
		}
	}
	ramp := func(x, _ float64) [3]float64 {
		v := (100 + x - 0.5) / 255
		return [3]float64{v, v, v}
	}
	smoothSeams(labels, rgb, 64, 64, []func(x, y float64) [3]float64{ramp, ramp}, 0.03)
	if labels[cy*64+cx] != 1 {
		t.Fatal("ringed pixel did not join its neighbours")
	}
}

func TestSmoothSeamsSkipsFlatLabels(t *testing.T) {
	rgb, labels := seamCanvas(false)
	ramp := func(x, _ float64) [3]float64 {
		v := (100 + x - 0.5) / 255
		return [3]float64{v, v, v}
	}
	before := append([]uint8(nil), labels...)
	smoothSeams(labels, rgb, 64, 64, []func(x, y float64) [3]float64{ramp, nil}, 0.03)
	for p := range labels {
		if labels[p] != before[p] {
			t.Fatalf("pixel %d moved although one side is flat", p)
		}
	}
}

// creaseCanvas folds two ramps along x = 32: the left half brightens
// down y, the right half brightens across x, and they meet at the same
// colour on the fold, so the seam is soft but the halves cannot share
// one linear model. The labels wobble ±3 px around the fold.
func creaseCanvas() (rgb, labels []uint8) {
	const w, h = 64, 64
	rgb = make([]uint8, 3*w*h)
	labels = make([]uint8, w*h)
	for y := 0; y < h; y++ {
		cut := 32
		if (y/4)%2 == 1 {
			cut = 35
		}
		for x := 0; x < w; x++ {
			p := y*w + x
			v := 30 + 2*y
			if x >= 32 {
				v += 3 * (x - 32)
			}
			if x >= cut {
				labels[p] = 1
			}
			rgb[3*p], rgb[3*p+1], rgb[3*p+2] = uint8(v), uint8(v), uint8(v)
		}
	}
	return rgb, labels
}

func TestMergeSmoothsSoftSeam(t *testing.T) {
	rgb, labels := creaseCanvas()
	r := Merge(rgb, labels, 64, 64, 2, 0.01)
	if r.N != 2 || r.Ramps[0] == nil || r.Ramps[1] == nil {
		t.Fatalf("want two ramp labels, got N=%d ramps=%v", r.N, r.Ramps)
	}
	if s := seamSpread(r.Labels, 64, 64); s > 1 {
		t.Fatalf("seam still wobbles by %d px", s)
	}
}
