package gradient

import (
	"math/rand/v2"
	"testing"
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
	rgb, labels := twoFlats(64, 16, 100, 102) // step ≈ 0.014: may merge; a ramp spanning under tol stays flat
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
	// 256 labels, one per column of a 256×20 image: a ramp over columns
	// 0..127 (grey = x) and, after a 60-grey jump, another over 128..255.
	// Each ramp wants a fresh label while all 256 are in use, so the
	// merges must stop at the cap rather than wrap a uint8.
	const w, h = 256, 20
	rgb := make([]uint8, 3*w*h)
	labels := make([]uint8, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			p := y*w + x
			v := uint8(x)
			if x >= 128 {
				v = uint8(x - 60)
			}
			rgb[3*p], rgb[3*p+1], rgb[3*p+2] = v, v, v
			labels[p] = uint8(x)
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
