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

// capImage is a white page with three two-band grey ramps A, B (12
// counts above A at the same x) and C, and one speck of every other
// label on the last row, so n = 254 leaves room for two merged groups.
func capImage(w, h int) (rgb, labels []uint8) {
	const bg = 253
	rgb = make([]uint8, 3*w*h)
	labels = make([]uint8, w*h)
	set := func(x, y int, v uint8, l uint8) {
		p := y*w + x
		rgb[3*p], rgb[3*p+1], rgb[3*p+2] = v, v, v
		labels[p] = l
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			set(x, y, 255, bg)
		}
	}
	for y := 0; y < 20; y++ { // A: labels 0,1
		for x := 0; x < 30; x++ {
			set(x, y, uint8(20+7*x), uint8(x/15))
		}
	}
	for y := 20; y < 40; y++ { // B: labels 2,3
		for x := 0; x < 30; x++ {
			set(x, y, uint8(40+7*x), uint8(2+x/15))
		}
	}
	for y := 48; y < 66; y++ { // C: labels 4,5
		for x := 0; x < 30; x++ {
			set(x, y, uint8(20+8*x), uint8(4+x/15))
		}
	}
	for k := 0; k < bg; k++ {
		set(2*k+1, h-1, 128, uint8(k))
	}
	return rgb, labels
}

func TestMergeDissolveKeepsLabelBudget(t *testing.T) {
	// Pass 1 makes A and B and refuses C at the cap; pass 2 joins A+B
	// (which would free a label) and then C; A+B's model misses B and
	// dissolves. The labels must still fit in 256 and distinct regions
	// must keep distinct labels.
	const w, h, n = 520, 72, 254
	rgb, labels := capImage(w, h)
	r := Merge(rgb, labels, w, h, n, 0.03)
	if r.N > 256 {
		t.Errorf("N %d > 256", r.N)
	}
	for p, l := range r.Labels {
		if int(l) >= r.N {
			t.Fatalf("label %d past N %d at %d", l, r.N, p)
		}
	}
	a := r.Labels[5*w+5]
	for p, l := range r.Labels {
		if x, y := p%w, p/w; l == a && !(y < 20 && x < 30) {
			t.Fatalf("pixel (%d,%d) outside A carries A's label %d (N %d)", x, y, a, r.N)
		}
	}
}

func TestMergeSteepGrainyRampJoins(t *testing.T) {
	// A steep ramp (6 counts per pixel) with ±4 grain: the per-pixel
	// gradient gate lets through only pixels whose grain cancels the
	// slope, which would tilt a tensor-only direction and stop the bands
	// joining. v1 joined every such case.
	const w, h = 40, 128
	rng := rand.New(rand.NewPCG(7, 4004))
	rgb := make([]uint8, 3*w*h)
	labels := make([]uint8, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			p := y*w + x
			v := float64(x) * 255 / float64(w-1)
			for k := 0; k < 3; k++ {
				rgb[3*p+k] = clamp8(int(math.Round(v)) + rng.IntN(9) - 4)
			}
			labels[p] = uint8(x * 4 / w)
		}
	}
	r := Merge(rgb, labels, w, h, 4, 0.03)
	if r.N != 1 || r.Ramps[0] == nil {
		t.Errorf("N %d ramps %v", r.N, r.Ramps)
	}
}

// canvas is a grey image with labels, painted region by region.
type canvas struct {
	w, h   int
	rgb    []uint8
	labels []uint8
}

func newCanvas(w, h int) *canvas {
	return &canvas{w: w, h: h, rgb: make([]uint8, 3*w*h), labels: make([]uint8, w*h)}
}

func (c *canvas) set(x, y int, v uint8, l uint8) {
	p := y*c.w + x
	c.rgb[3*p], c.rgb[3*p+1], c.rgb[3*p+2] = v, v, v
	c.labels[p] = l
}

// quad paints a quadratic grey ramp 0.2..0.8 over columns x0..x0+256,
// quantised into 8 bands labelled l0..l0+7 by colour range.
func (c *canvas) quad(x0 int, l0 uint8) {
	for y := 0; y < c.h; y++ {
		for x := 0; x < 256; x++ {
			s := float64(x) / 255
			v := 0.2 + float64(0.6*float64(s*s))
			c.set(x0+x, y, uint8(math.Round(v*255)), l0+uint8(min(int((v-0.2)/0.6*8), 7)))
		}
	}
}

// wander paints a band over columns x0..x0+128 that meets 204 grey at
// its left edge and then varies along y more and more with x, so it
// joins a ramp ending at 204 across a soft boundary but fits no model
// along the ramp's direction.
func (c *canvas) wander(x0 int, l uint8) {
	for y := 0; y < c.h; y++ {
		for x := 0; x < 128; x++ {
			v := 204 + x*(y-c.h/2)/(c.h/2)
			c.set(x0+x, y, clamp8(v), l)
		}
	}
}

func rampLabels(r Result) (n int, stops int) {
	for _, rp := range r.Ramps {
		if rp != nil {
			n++
			stops = max(stops, len(rp.Stops))
		}
	}
	return n, stops
}

func TestMergePartialDissolveKeepsGoodMembers(t *testing.T) {
	c := newCanvas(384, 32)
	c.quad(0, 0)
	c.wander(256, 8)
	r := Merge(c.rgb, c.labels, c.w, c.h, 9, 0.03)
	ramps, stops := rampLabels(r)
	if r.N != 2 || ramps != 1 || stops < 3 {
		t.Errorf("N %d ramps %d stops %d: the bent ramp must stay one group while the wandering band reverts", r.N, ramps, stops)
	}
}

func TestMergePartialDissolveSplits(t *testing.T) {
	c := newCanvas(640, 32)
	c.quad(0, 0)
	c.wander(256, 8)
	// Mirror the wandering band's right edge into a second ramp: paint
	// the ramp reversed so it starts at 204 where the band ends.
	for y := 0; y < c.h; y++ {
		for x := 0; x < 256; x++ {
			s := float64(255-x) / 255
			v := 0.2 + float64(0.6*float64(s*s))
			c.set(384+x, y, uint8(math.Round(v*255)), 9+uint8(min(int((v-0.2)/0.6*8), 7)))
		}
	}
	// The band's right edge must meet 204: repaint its last column flat.
	for y := 0; y < c.h; y++ {
		c.set(383, y, 204, 8)
	}
	r := Merge(c.rgb, c.labels, c.w, c.h, 17, 0.03)
	ramps, stops := rampLabels(r)
	if r.N != 3 || ramps != 2 || stops < 3 {
		t.Errorf("N %d ramps %d stops %d: both bent ramps must survive as groups of their own", r.N, ramps, stops)
	}
}

// bentCanvas is a 256×64 hub: a merged chunk M (labels 0 and 1, a linear
// ramp 128..204 along x over columns 0..128 of the lower half) meets a
// wandering band W (label 2, columns 128..256 of the lower half, 204 at
// its left edge, bending more and more along y with x) which in turn
// meets two lone bands A (label 3) and B (label 4) above it, a quadratic
// fall 204..24 along x that no line fits but one multi-stop ramp does.
// The join pass takes all four across soft edges, the group fits no
// model, and in the replay only A with B fits. F (label 5, flat 128)
// fills the upper-left quarter and joins nothing.
func bentCanvas() *canvas {
	c := newCanvas(256, 64)
	for y := 0; y < 64; y++ {
		for x := 0; x < 256; x++ {
			s := float64(x-128) / 128
			switch {
			case x < 128 && y < 32:
				c.set(x, y, 128, 5)
			case x < 128:
				c.set(x, y, uint8(128+76*x/127), uint8(x/64))
			case y < 32:
				c.set(x, y, clamp8(int(math.Round(204-float64(180*float64(s*s))))), 3+uint8((x-128)/64))
			default:
				g := 1 - float64(2*float64(y-32))/31
				c.set(x, y, clamp8(int(math.Round(204-float64(180*float64(s*s))*g))), 2)
			}
		}
	}
	return c
}

func TestMergePartialDissolveLabelBudget(t *testing.T) {
	// Budget: with M merged, A+B need the 257th label; with n = 255 the
	// replay must refuse the lone-only sub-group and keep A and B apart.
	c := bentCanvas()
	const n = 255
	for k := 6; k < n; k++ { // a speck of every other label on the last row
		c.set(k, 63, 128, uint8(k))
	}
	r := Merge(c.rgb, c.labels, c.w, c.h, n, 0.03)
	if r.N > 256 {
		t.Errorf("N %d > 256", r.N)
	}
	for p, l := range r.Labels {
		if int(l) >= r.N {
			t.Fatalf("label %d past N %d at %d", l, r.N, p)
		}
	}
	if r.Labels[48*256+30] != r.Labels[48*256+100] || r.Ramps[r.Labels[48*256+30]] == nil {
		t.Errorf("M must stay one ramp")
	}
	if r.Labels[16*256+150] == r.Labels[16*256+220] {
		t.Errorf("A and B joined: the 257th label was spent")
	}
	// With a label to spare, A and B re-join as a ramp of their own.
	c = bentCanvas()
	r = Merge(c.rgb, c.labels, c.w, c.h, 6, 0.03)
	if r.Labels[16*256+150] != r.Labels[16*256+220] || r.Ramps[r.Labels[16*256+150]] == nil {
		t.Errorf("A and B must re-join after the dissolve: labels %d %d ramps %v", r.Labels[16*256+150], r.Labels[16*256+220], r.Ramps)
	}
	if r.Labels[48*256+30] == r.Labels[16*256+150] {
		t.Errorf("M must not join A: the hub must dissolve")
	}
}

// modelFits walks only the pixels it is given, so a dissolved group's
// pixels are gathered once and each replay check costs the union, not
// the image.
func TestModelFitsWalksGivenPixels(t *testing.T) {
	rgb, labels := rampImage(128, 32, 2)
	w, h := 128, 32
	comp := make([]int32, w*h)
	for p := range comp {
		comp[p] = int32(labels[p])
	}
	r1 := []int32{0, 1}
	in := []bool{true, true}
	var m moments
	var all, left []int
	for p := 0; p < w*h; p++ {
		all = append(all, p)
		if p%w < 64 {
			left = append(left, p)
		}
	}
	for p := 0; p < w*h; p++ {
		if p%w >= 64 && (p/w+p%w)%2 == 0 { // checkerboard noise on the right half
			rgb[3*p], rgb[3*p+1], rgb[3*p+2] = 0, 0, 0
		}
	}
	for _, p := range all {
		m.add(float64(p%w)+0.5, float64(p/w)+0.5, colourOf(rgb, p))
	}
	if modelFits(rgb, comp, w, h, r1, in, all, &m, 0.03) {
		t.Errorf("the noisy right half must fail")
	}
	m = moments{}
	for _, p := range left {
		m.add(float64(p%w)+0.5, float64(p/w)+0.5, colourOf(rgb, p))
	}
	if !modelFits(rgb, comp, w, h, r1, in, left, &m, 0.03) {
		t.Errorf("the clean left half alone must fit")
	}
}

// islandImage is the 128×32 four-band grey ramp with an island of label
// 4 over x0..x1 × y0..y1 whose grey is the ramp's value plus delta.
func islandImage(x0, x1, y0, y1, delta int) (rgb, labels []uint8) {
	rgb, labels = rampImage(128, 32, 4)
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			p := y*128 + x
			v := clamp8(x*255/127 + delta)
			rgb[3*p], rgb[3*p+1], rgb[3*p+2] = v, v, v
			labels[p] = 4
		}
	}
	return rgb, labels
}

func TestMergeIslandAbsorbed(t *testing.T) {
	rgb, labels := islandImage(60, 63, 14, 17, 4)
	r := Merge(rgb, labels, 128, 32, 5, 0.03)
	if r.N != 1 || r.Ramps[0] == nil {
		t.Errorf("N %d ramps %v: a speck of the ramp's own colour must join it", r.N, r.Ramps)
	}
	if r.OldToNew[4] != -1 {
		t.Errorf("OldToNew[4] %d, want -1 (every pixel joined the ramp)", r.OldToNew[4])
	}
}

func TestMergeIslandOfOtherColourKept(t *testing.T) {
	rgb, labels := islandImage(60, 63, 14, 17, 40)
	r := Merge(rgb, labels, 128, 32, 5, 0.03)
	if r.N != 2 || r.Labels[15*128+61] == r.Labels[15*128+20] {
		t.Errorf("N %d: a speck 40 counts off must keep its label", r.N)
	}
}

func TestMergeIslandSliverAbsorbed(t *testing.T) {
	rgb, labels := islandImage(60, 61, 10, 16, 3)
	r := Merge(rgb, labels, 128, 32, 5, 0.03)
	if r.N != 1 || r.Ramps[0] == nil {
		t.Errorf("N %d ramps %v: a 1-px sliver has no interior pixel but still joins", r.N, r.Ramps)
	}
}

func TestMergeIslandPicksLargestBorder(t *testing.T) {
	// A: horizontal ramp 40+x over x < 128 (labels 0..3); B: vertical
	// ramp 167+2(y−15) over x ≥ 128 (labels 4..7); a hard step between
	// them except near y = 15. The island (label 8) over x 125..128,
	// y 14..16 fits both models at its centroid; it borders A along 9
	// pixel pairs and B along 5, so it takes A's label.
	const w, h = 256, 32
	rgb := make([]uint8, 3*w*h)
	labels := make([]uint8, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			p := y*w + x
			var v int
			if x < 128 {
				v, labels[p] = 40+x, uint8(x/32)
			} else {
				v, labels[p] = 167+2*(y-15), uint8(4+y/8)
			}
			if x >= 125 && x < 129 && y >= 14 && y < 17 {
				v, labels[p] = 167, 8
			}
			c := clamp8(v)
			rgb[3*p], rgb[3*p+1], rgb[3*p+2] = c, c, c
		}
	}
	r := Merge(rgb, labels, w, h, 9, 0.03)
	a, b, isl := r.Labels[15*w+60], r.Labels[15*w+200], r.Labels[15*w+126]
	if a == b || r.Ramps[a] == nil || r.Ramps[b] == nil {
		t.Fatalf("fixture: a %d b %d ramps %v", a, b, r.Ramps)
	}
	if isl != a {
		t.Errorf("island label %d, want A's %d (B is %d)", isl, a, b)
	}
}

// ringImage is a disc of radius rad about (cx, cy) on a white page,
// shaded grey from v0 at the centre to v1 at the rim, quantised into
// bands equal-width label bands; the page is label 0.
func ringImage(w, h int, cx, cy, rad, v0, v1 float64, bands int) (rgb, labels []uint8) {
	rgb = make([]uint8, 3*w*h)
	labels = make([]uint8, w*h)
	for p := range labels {
		x, y := float64(p%w)+0.5, float64(p/w)+0.5
		r := math.Hypot(x-cx, y-cy)
		v := 1.0
		if r <= rad {
			s := r / rad
			v = v0 + s*(v1-v0)
			labels[p] = uint8(1 + min(int(s*float64(bands)), bands-1))
		}
		g := uint8(math.Round(v * 255))
		rgb[3*p], rgb[3*p+1], rgb[3*p+2] = g, g, g
	}
	return rgb, labels
}

// Concentric bands of a disc shaded from its centre out join into one
// region with a radial gradient about that centre: no direction fits
// them, but the distance from the centre does.
func TestMergeRadialRings(t *testing.T) {
	const w, h = 160, 160
	cx, cy, rad := 80.0, 80.0, 70.0
	rgb, labels := ringImage(w, h, cx, cy, rad, 0.25, 0.75, 6)
	r := Merge(rgb, labels, w, h, 7, 0.03)
	if r.N != 2 {
		t.Fatalf("N %d, want the page and one disc: ramps %v", r.N, r.Ramps)
	}
	var rp *Ramp
	for _, x := range r.Ramps {
		if x != nil {
			rp = x
		}
	}
	if rp == nil || !rp.Radial {
		t.Fatalf("no radial ramp: %+v", rp)
	}
	if math.Abs(rp.CX-cx) > 1 || math.Abs(rp.CY-cy) > 1 || math.Abs(rp.R-rad) > 2 {
		t.Fatalf("centre (%.1f,%.1f) r %.1f, want (%.0f,%.0f) r %.0f", rp.CX, rp.CY, rp.R, cx, cy, rad)
	}
	if len(rp.Stops) < 2 {
		t.Fatalf("stops %+v", rp.Stops)
	}
	for _, st := range rp.Stops {
		want := 0.25 + 0.5*float64(st.Offset*rp.R)/rad
		if d := math.Abs(float64(st.Colour.R)/255 - want); d > 0.03 {
			t.Errorf("stop %+v is %.3f from the shading", st, d)
		}
	}
}
