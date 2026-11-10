package gradient

import (
	"math"
	"math/rand/v2"
	"testing"
	"time"
)

// rampImage is a horizontal grey ramp 0..255 over w, quantised into
// bands equal-width label bands.
func rampImage(w, h, bands int) (rgb, labels []uint8) {
	rgb = make([]uint8, 3*w*h)
	labels = make([]uint8, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			p := y*w + x
			v := uint8(x * 255 / (w - 1))
			rgb[3*p], rgb[3*p+1], rgb[3*p+2] = v, v, v
			labels[p] = uint8(x * bands / w)
		}
	}
	return rgb, labels
}

// twoFlats is grey a on the left half (label 0) and grey b on the right
// (label 1).
func twoFlats(w, h int, a, b uint8) (rgb, labels []uint8) {
	rgb = make([]uint8, 3*w*h)
	labels = make([]uint8, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			p := y*w + x
			v := a
			if x >= w/2 {
				v, labels[p] = b, 1
			}
			rgb[3*p], rgb[3*p+1], rgb[3*p+2] = v, v, v
		}
	}
	return rgb, labels
}

func clamp8(v int) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

func TestMergeRampBandsJoin(t *testing.T) {
	rgb, labels := rampImage(128, 16, 8)
	r := Merge(rgb, labels, 128, 16, 8, 0.03)
	if r.N != 1 || r.Ramps[0] == nil {
		t.Fatalf("N %d ramps %v", r.N, r.Ramps)
	}
	if s := r.Ramps[0].Stops; s[0].Colour.R > 8 || s[1].Colour.R < 247 {
		t.Errorf("stops %+v", s)
	}
	for _, l := range r.Labels {
		if l != 0 {
			t.Fatal("every pixel should carry the merged label")
		}
	}
}

func TestMergeHardEdgeRefused(t *testing.T) {
	rgb, labels := twoFlats(64, 16, 100, 113) // step 13/255 ≈ 0.051
	r := Merge(rgb, labels, 64, 16, 2, 0.03)
	if r.N != 2 || r.Ramps[0] != nil || r.Ramps[1] != nil {
		t.Errorf("N %d ramps %v", r.N, r.Ramps)
	}
	if r.OldToNew[0] != 0 || r.OldToNew[1] != 1 {
		t.Errorf("OldToNew %v", r.OldToNew)
	}
}

func TestMergeNearFlatStaysFlat(t *testing.T) {
	rgb, labels := twoFlats(64, 16, 100, 104) // step ≈ 0.027: two flats, never a ramp
	r := Merge(rgb, labels, 64, 16, 2, 0.03)
	for _, rp := range r.Ramps {
		if rp != nil {
			t.Errorf("near-flat pair produced a ramp %+v", rp)
		}
	}
}

func TestMergeNoisyRampStillMerges(t *testing.T) {
	rgb, labels := rampImage(128, 16, 8)
	rnd := rand.New(rand.NewPCG(1, 2))
	for i := range rgb {
		rgb[i] = clamp8(int(rgb[i]) + rnd.IntN(9) - 4) // ±4/255 noise
	}
	r := Merge(rgb, labels, 128, 16, 8, 0.03)
	if r.N != 1 || r.Ramps[0] == nil {
		t.Errorf("N %d", r.N)
	}
}

func TestMergeTinyComponentIgnored(t *testing.T) {
	rgb, labels := twoFlats(32, 32, 120, 120)
	for p := range labels {
		labels[p] = 0
	}
	// A 3×3 blob of label 1, slightly lighter, in the middle.
	for y := 14; y < 17; y++ {
		for x := 14; x < 17; x++ {
			p := y*32 + x
			labels[p] = 1
			rgb[3*p], rgb[3*p+1], rgb[3*p+2] = 124, 124, 124
		}
	}
	r := Merge(rgb, labels, 32, 32, 2, 0.03)
	if r.N != 2 || r.Ramps[0] != nil || r.Ramps[1] != nil {
		t.Errorf("N %d ramps %v", r.N, r.Ramps)
	}
}

func TestMergeLabelCap(t *testing.T) {
	// 256 labels, one per 4-px column band of a 1024×20 image: a ramp
	// over bands 0..127 (grey = band) and, after a 60-grey jump, another
	// over 128..255. Each ramp wants a fresh label while all 256 are in
	// use, so the merges must stop at the cap rather than wrap a uint8.
	const w, h = 1024, 20
	rgb := make([]uint8, 3*w*h)
	labels := make([]uint8, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			p := y*w + x
			band := x / 4
			v := uint8(band)
			if band >= 128 {
				v = uint8(band - 60)
			}
			rgb[3*p], rgb[3*p+1], rgb[3*p+2] = v, v, v
			labels[p] = uint8(band)
		}
	}
	r := Merge(rgb, labels, w, h, 256, 0.03)
	if r.N > 256 {
		t.Fatalf("N %d", r.N)
	}
	for _, l := range r.Labels {
		if int(l) >= r.N {
			t.Fatalf("label %d past N %d", l, r.N)
		}
	}
}

func TestMergeOldToNew(t *testing.T) {
	const w, h = 128, 20
	rgb, labels := rampImage(w, 16, 4)
	// A flat label-4 strip of 4 rows below the ramp.
	rgb = append(rgb, make([]uint8, 3*w*4)...)
	labels = append(labels, make([]uint8, w*4)...)
	for p := w * 16; p < w*h; p++ {
		labels[p] = 4
		rgb[3*p], rgb[3*p+1], rgb[3*p+2] = 30, 200, 30
	}
	r := Merge(rgb, labels, w, h, 5, 0.03)
	if r.N != 2 {
		t.Fatalf("N %d", r.N)
	}
	if r.OldToNew[4] < 0 || r.Ramps[r.OldToNew[4]] != nil {
		t.Errorf("flat label must keep a flat new label: OldToNew %v ramps %v", r.OldToNew, r.Ramps)
	}
	for i := 0; i < 4; i++ {
		if r.OldToNew[i] != -1 {
			t.Errorf("band %d should be absorbed, OldToNew %v", i, r.OldToNew)
		}
	}
}

func TestMergeShortRampStaysFlat(t *testing.T) {
	// A 10×10 pocket of label 1 inside flat label 0, with a steep ramp
	// across its 10 px: too short to show as a gradient, so it stays flat.
	const w, h = 32, 32
	rgb := make([]uint8, 3*w*h)
	labels := make([]uint8, w*h)
	for p := range labels {
		rgb[3*p], rgb[3*p+1], rgb[3*p+2] = 120, 120, 120
	}
	for y := 11; y < 21; y++ {
		for x := 11; x < 21; x++ {
			p := y*w + x
			labels[p] = 1
			v := uint8(200 + (x-11)*5)
			rgb[3*p], rgb[3*p+1], rgb[3*p+2] = v, v, v
		}
	}
	r := Merge(rgb, labels, w, h, 2, 0.03)
	for _, rp := range r.Ramps {
		if rp != nil {
			t.Errorf("short pocket produced a ramp %+v", rp)
		}
	}
}

func TestMergeLoneRampBeforeMergedGroupsNoPanic(t *testing.T) {
	// 254 labels in use: a lone ramp first in scan order, then two
	// two-band ramps. Only two fresh labels are free; the merged groups
	// must get them and the lone ramp must stay flat, never a panic.
	const w, h = 192, 32
	rgb := make([]uint8, 3*w*h)
	labels := make([]uint8, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			p := y*w + x
			var v int
			var l uint8
			switch {
			case x < 64:
				v, l = x*40/63, 0
			case x < 128:
				v, l = 200+(x-64)*40/63, 1
				if x >= 96 {
					l = 2
				}
			default:
				v, l = 60+(x-128)*40/63, 3
				if x >= 160 {
					l = 4
				}
			}
			rgb[3*p], rgb[3*p+1], rgb[3*p+2] = uint8(v), uint8(v), uint8(v)
			labels[p] = l
		}
	}
	r := Merge(rgb, labels, w, h, 254, 0.03)
	if r.N > 256 {
		t.Fatalf("N %d", r.N)
	}
	ramps := 0
	for _, rp := range r.Ramps {
		if rp != nil {
			ramps++
		}
	}
	if ramps != 2 {
		t.Errorf("%d ramps, want the two merged groups", ramps)
	}
}

func TestMergeAAEdgeRefused(t *testing.T) {
	// Two flats with a slanted anti-aliased edge: the edge pixels mix by
	// coverage and carry the nearer label, halving the measured step.
	const w, h = 128, 64
	for _, d := range []int{6, 10, 16} {
		rgb := make([]uint8, 3*w*h)
		labels := make([]uint8, w*h)
		for y := 0; y < h; y++ {
			ex := float64(w)/2 + 0.3*float64(y)
			for x := 0; x < w; x++ {
				p := y*w + x
				cov := math.Max(0, math.Min(1, float64(x)+1-ex))
				rgb[3*p] = uint8(math.Round(100 + float64(d)*cov))
				rgb[3*p+1], rgb[3*p+2] = 100, 100
				if cov >= 0.5 {
					labels[p] = 1
				}
			}
		}
		r := Merge(rgb, labels, w, h, 2, 0.03)
		if r.N != 2 || r.Ramps[0] != nil || r.Ramps[1] != nil {
			t.Errorf("step %d/255: N %d ramps %v", d, r.N, r.Ramps)
		}
	}
}

func TestMergeToleranceZeroKeepsFlatFlat(t *testing.T) {
	rgb, labels := twoFlats(64, 32, 120, 120)
	r := Merge(rgb, labels, 64, 32, 2, 0)
	for _, rp := range r.Ramps {
		if rp != nil {
			t.Errorf("flat region became a ramp at tolerance 0: %+v", rp)
		}
	}
}

func TestMergeHubAbsorbsIslandsInOnePass(t *testing.T) {
	// A ramp dotted with 10 404 islands that all belong to it (the side is
	// a multiple of the period so no island is clipped): a hub must
	// absorb many neighbours per pass, not one per round.
	const w, h = 1020, 1020
	rgb := make([]uint8, 3*w*h)
	labels := make([]uint8, w*h)
	for p := range labels {
		x, y := p%w, p/w
		v := uint8(x * 255 / (w - 1))
		rgb[3*p], rgb[3*p+1], rgb[3*p+2] = v, v, v
		if x%10 >= 2 && x%10 < 8 && y%10 >= 2 && y%10 < 8 {
			labels[p] = 1
		}
	}
	start := time.Now()
	r := Merge(rgb, labels, w, h, 2, 0.03)
	if d := time.Since(start); d > 3*time.Second {
		t.Errorf("took %v", d)
	}
	if r.N != 1 || r.Ramps[0] == nil {
		t.Errorf("N %d", r.N)
	}
}

// curveImage is a horizontal grey image v = 255·curve(x/(w-1)) with
// optional ±noise, labelled into bands by equal colour ranges of the
// curve (so bands are narrow where the curve is steep, as a quantiser
// would make them).
func curveImage(w, h, bands int, curve func(float64) float64, noise int, seed uint64) (rgb, labels []uint8) {
	rgb = make([]uint8, 3*w*h)
	labels = make([]uint8, w*h)
	rng := rand.New(rand.NewPCG(seed, 1))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			p := y*w + x
			c := curve(float64(x) / float64(w-1))
			v := clamp8(int(math.Round(c*255)) + rng.IntN(2*noise+1) - noise)
			rgb[3*p], rgb[3*p+1], rgb[3*p+2] = v, v, v
			labels[p] = uint8(min(int(c*float64(bands)), bands-1))
		}
	}
	return rgb, labels
}

func TestMergeCurvedRampJoins(t *testing.T) {
	quad := func(s float64) float64 { return float64(s * s) }
	rgb, labels := curveImage(256, 32, 8, quad, 0, 1)
	r := Merge(rgb, labels, 256, 32, 8, 0.03)
	if r.N != 1 || r.Ramps[0] == nil {
		t.Fatalf("N %d ramps %v", r.N, r.Ramps)
	}
	s := r.Ramps[0].Stops
	if len(s) < 3 {
		t.Fatalf("a quadratic ramp needs more than two stops: %+v", s)
	}
	for _, st := range s {
		want := quad(st.Offset)
		if d := math.Abs(float64(st.Colour.R)/255 - want); d > 0.03 {
			t.Errorf("stop %+v is %.3f from the curve", st, d)
		}
	}
}

func TestMergeNoisyCurvedRampJoins(t *testing.T) {
	quad := func(s float64) float64 { return float64(s * s) }
	rgb, labels := curveImage(256, 32, 8, quad, 4, 2)
	r := Merge(rgb, labels, 256, 32, 8, 0.03)
	if r.N != 1 || r.Ramps[0] == nil || len(r.Ramps[0].Stops) < 3 {
		t.Fatalf("N %d ramps %v", r.N, r.Ramps)
	}
}

func TestMergeRiseAndFall(t *testing.T) {
	hump := func(s float64) float64 { return math.Sin(math.Pi * s) }
	rgb, labels := curveImage(256, 32, 8, hump, 0, 3)
	r := Merge(rgb, labels, 256, 32, 8, 0.03)
	if r.N != 1 || r.Ramps[0] == nil {
		t.Fatalf("N %d ramps %v", r.N, r.Ramps)
	}
	s := r.Ramps[0].Stops
	peak := 0
	for i, st := range s {
		if st.Colour.R > s[peak].Colour.R {
			peak = i
		}
	}
	if peak == 0 || peak == len(s)-1 || s[peak].Colour.R < 240 || s[0].Colour.R > 15 || s[len(s)-1].Colour.R > 15 {
		t.Errorf("stops should rise then fall: %+v", s)
	}
}

func TestMergeLonePocketStaysFlat(t *testing.T) {
	// One flat band with a pocket of a nearby colour in its middle: the
	// samples bump by tol but nothing ramps, so the band stays flat.
	const w, h = 192, 64
	rgb := make([]uint8, 3*w*h)
	labels := make([]uint8, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			p := y*w + x
			c := [3]uint8{30, 60, 220}
			if x >= 96 && x < 112 && y >= 8 && y < 56 {
				c = [3]uint8{42, 70, 222}
			}
			rgb[3*p], rgb[3*p+1], rgb[3*p+2] = c[0], c[1], c[2]
		}
	}
	r := Merge(rgb, labels, w, h, 1, 0.03)
	if r.N != 1 || r.Ramps[0] != nil {
		t.Errorf("N %d ramps %+v", r.N, r.Ramps)
	}
}

func TestMergeDirectionIgnoresOutline(t *testing.T) {
	// A narrow vertical ramp (two bands along y) on a white page: the
	// pixels beside its outline see a huge horizontal step, which must
	// not turn the ramp's direction sideways and stop the bands joining.
	const w, h = 64, 256
	rgb := make([]uint8, 3*w*h)
	labels := make([]uint8, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			p := y*w + x
			v := uint8(255)
			labels[p] = 2
			if x >= 24 && x < 40 {
				v = uint8(40 + y*60/(h-1))
				labels[p] = uint8(y * 2 / h)
				if x == 24 || x == 39 {
					v = uint8((int(v) + 255) / 2) // anti-aliased outline column
				}
			}
			rgb[3*p], rgb[3*p+1], rgb[3*p+2] = v, v, v
		}
	}
	r := Merge(rgb, labels, w, h, 3, 0.03)
	if r.N != 2 || r.Ramps[1] == nil {
		t.Errorf("N %d ramps %v", r.N, r.Ramps)
	}
}
