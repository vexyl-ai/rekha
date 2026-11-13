package rekha

import (
	"image"
	"image/color"
	"strings"
	"testing"
)

// maskFixture is a 32×32 picture: red ground with a blue square from
// (8,8) to (24,24).
func maskFixture() *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			c := color.NRGBA{R: 220, G: 30, B: 30, A: 255}
			if x >= 8 && x < 24 && y >= 8 && y < 24 {
				c = color.NRGBA{R: 20, G: 40, B: 230, A: 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

// square is full coverage inside [x0,x1)×[y0,y1) of a 32×32 image.
func square(x0, y0, x1, y1 int) []uint8 {
	cov := make([]uint8, 32*32)
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			cov[y*32+x] = 255
		}
	}
	return cov
}

func fills(r *Result) map[string]int {
	out := map[string]int{}
	for _, p := range r.Paths {
		out[p.Fill]++
	}
	return out
}

func TestExternalMaskReplacesRegion(t *testing.T) {
	o := PresetOptions(PresetPoster)
	o.ExternalMasks = []Mask{{Coverage: square(4, 4, 20, 20), Fill: color.NRGBA{R: 0, G: 200, B: 0, A: 255}}}
	s, err := Segment(maskFixture(), o)
	if err != nil {
		t.Fatal(err)
	}
	green := -1
	for i, c := range s.Palette {
		if c == (color.NRGBA{R: 0, G: 200, B: 0, A: 255}) {
			green = i
		}
	}
	if green < 0 {
		t.Fatalf("palette %v lacks the mask's fill", s.Palette)
	}
	for _, p := range []struct{ x, y int }{{4, 4}, {10, 10}, {19, 19}} {
		if int(s.Labels[p.y*32+p.x]) != green {
			t.Errorf("pixel %v: label %d, want the mask's %d", p, s.Labels[p.y*32+p.x], green)
		}
	}
	for _, p := range []struct{ x, y int }{{3, 3}, {20, 20}, {22, 22}} {
		if int(s.Labels[p.y*32+p.x]) == green {
			t.Errorf("pixel %v outside the mask took its label", p)
		}
	}
	r, err := s.Finish(o)
	if err != nil {
		t.Fatal(err)
	}
	f := fills(r)
	if f["#00c800"] == 0 || f["#1428e6"] == 0 {
		t.Fatalf("paths %v: want the mask's green and the uncovered blue", f)
	}
}

func TestExternalMaskZeroFillTakesMean(t *testing.T) {
	o := PresetOptions(PresetPoster)
	o.ExternalMasks = []Mask{{Coverage: square(8, 8, 24, 24)}}
	s, err := Segment(maskFixture(), o)
	if err != nil {
		t.Fatal(err)
	}
	l := s.Labels[10*32+10]
	if s.Palette[l] != (color.NRGBA{R: 20, G: 40, B: 230, A: 255}) {
		t.Fatalf("mask colour %v, want the mean of its pixels", s.Palette[l])
	}
}

func TestExternalMaskSoftEdgeFeedsBlend(t *testing.T) {
	cov := square(8, 8, 24, 24)
	cov[8*32+8] = 200 // mostly the mask
	cov[8*32+9] = 60  // mostly the ground
	o := PresetOptions(PresetPoster)
	o.ExternalMasks = []Mask{{Coverage: cov, Fill: color.NRGBA{R: 0, G: 200, B: 0, A: 255}}}
	s, err := Segment(maskFixture(), o)
	if err != nil {
		t.Fatal(err)
	}
	mask := s.Labels[10*32+10]
	if p := 8*32 + 8; s.Labels[p] != mask || s.Second[p] == mask || s.Blend[p] != 200 {
		t.Errorf("coverage 200: label %d second %d blend %d, want label %d, second another, blend 200", s.Labels[p], s.Second[p], s.Blend[p], mask)
	}
	if p := 8*32 + 9; s.Labels[p] == mask || s.Second[p] != mask || s.Blend[p] != 195 {
		t.Errorf("coverage 60: label %d second %d blend %d, want another label, second %d, blend 195", s.Labels[p], s.Second[p], s.Blend[p], mask)
	}
}

func TestExternalMaskCutOutDropsRegion(t *testing.T) {
	ground := make([]uint8, 32*32)
	for i, c := range square(8, 8, 24, 24) {
		ground[i] = 255 - c
	}
	o := PresetOptions(PresetPoster)
	r, err := Trace(maskFixture(), o)
	if err != nil {
		t.Fatal(err)
	}
	if fills(r)["#dc1e1e"] == 0 {
		t.Fatalf("without the mask the red ground is drawn: %v", fills(r))
	}
	o.ExternalMasks = []Mask{{Coverage: ground, Cut: true}}
	r, err = Trace(maskFixture(), o)
	if err != nil {
		t.Fatal(err)
	}
	f := fills(r)
	if f["#dc1e1e"] != 0 || f["#1428e6"] == 0 {
		t.Fatalf("paths %v: the cut-out ground must go, the square must stay", f)
	}
}

func TestExternalMaskLaterWins(t *testing.T) {
	o := PresetOptions(PresetPoster)
	o.ExternalMasks = []Mask{
		{Coverage: square(0, 0, 32, 32), Fill: color.NRGBA{R: 0, G: 200, B: 0, A: 255}},
		{Coverage: square(8, 8, 24, 24), Fill: color.NRGBA{R: 200, G: 200, B: 0, A: 255}},
	}
	s, err := Segment(maskFixture(), o)
	if err != nil {
		t.Fatal(err)
	}
	if a, b := s.Palette[s.Labels[2*32+2]], s.Palette[s.Labels[10*32+10]]; a != (color.NRGBA{R: 0, G: 200, B: 0, A: 255}) || b != (color.NRGBA{R: 200, G: 200, B: 0, A: 255}) {
		t.Fatalf("outside %v inside %v: the later mask must win where both cover", a, b)
	}
}

func TestExternalMaskCoverageSizeIsChecked(t *testing.T) {
	o := PresetOptions(PresetPoster)
	o.ExternalMasks = []Mask{{Coverage: make([]uint8, 10)}}
	_, err := Segment(maskFixture(), o)
	if err == nil || !strings.Contains(err.Error(), "ExternalMasks[0]") {
		t.Fatalf("err %v, want a coverage size error naming the mask", err)
	}
}
