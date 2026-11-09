package rekha

import (
	"image"
	"image/color"
	"reflect"
	"testing"

	"github.com/vexyl-ai/rekha/internal/bitmap"
	"github.com/vexyl-ai/rekha/internal/colour"
)

func labOf(c color.NRGBA) colour.OKLab { return colour.FromSRGB8(c.R, c.G, c.B) }

// paletteNear reports whether some palette entry is within tol (OKLab) of c.
func paletteNear(pal []color.NRGBA, c color.NRGBA, tol float64) bool {
	for _, p := range pal {
		if colour.Dist(labOf(p), labOf(c)) <= tol {
			return true
		}
	}
	return false
}

func TestSegmentBWMatchesThreshold(t *testing.T) {
	img := synth(t, "star-5")
	s, err := Segment(img, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	want := []color.NRGBA{{255, 255, 255, 255}, {0, 0, 0, 255}}
	if !reflect.DeepEqual(s.Palette, want) {
		t.Fatalf("palette %v", s.Palette)
	}
	gray, w, h := bitmap.Gray(img)
	bm := bitmap.FromGray(gray, w, h, 128)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if (s.Labels[y*w+x] == 1) != bm.Get(x, y) {
				t.Fatalf("pixel (%d,%d) label %d, bitmap %v", x, y, s.Labels[y*w+x], bm.Get(x, y))
			}
		}
	}
}

func TestSegmentTwoColourAbut(t *testing.T) {
	s, err := Segment(synth(t, "two-colour-abut"), PresetOptions(PresetPoster))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Palette) != 2 || !paletteNear(s.Palette, color.NRGBA{220, 30, 30, 255}, 0.02) || !paletteNear(s.Palette, color.NRGBA{30, 60, 220, 255}, 0.02) {
		t.Fatalf("palette %v", s.Palette)
	}
	for y := 0; y < s.Height; y++ {
		row := s.Labels[y*s.Width : (y+1)*s.Width]
		for x, l := range row {
			if l != row[0] && x < 128 || l == row[0] && x >= 128 {
				t.Fatalf("row %d: label %d at x=%d", y, l, x)
			}
		}
	}
}

func TestSegmentThreeLayerStack(t *testing.T) {
	s, err := Segment(synth(t, "three-layer-stack"), PresetOptions(PresetPoster))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Palette) != 3 {
		t.Fatalf("palette %v, want blue, red, white", s.Palette)
	}
	for _, c := range []color.NRGBA{{30, 60, 220, 255}, {220, 30, 30, 255}, {255, 255, 255, 255}} {
		if !paletteNear(s.Palette, c, 0.02) {
			t.Errorf("%v missing from %v", c, s.Palette)
		}
	}
}

func TestSegmentManyColours(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.SetNRGBA(x, y, color.NRGBA{uint8(x * 4), uint8(y * 4), uint8((x ^ y) * 4), 255})
		}
	}
	s, err := Segment(img, PresetOptions(PresetPhoto))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Palette) > 256 || len(s.Palette) < 2 {
		t.Fatalf("palette size %d", len(s.Palette))
	}
	for _, l := range s.Labels {
		if int(l) >= len(s.Palette) {
			t.Fatalf("label %d out of range", l)
		}
	}
}

func TestSegmentFixedPaletteDropsUnused(t *testing.T) {
	o := PresetOptions(PresetPoster)
	red, blue, green := color.NRGBA{255, 0, 0, 255}, color.NRGBA{0, 0, 255, 255}, color.NRGBA{0, 255, 0, 255}
	o.Palette = []color.NRGBA{green, blue, red}
	s, err := Segment(synth(t, "two-colour-abut"), o)
	if err != nil {
		t.Fatal(err)
	}
	// Scan order: red half first, then blue.
	if !reflect.DeepEqual(s.Palette, []color.NRGBA{red, blue}) {
		t.Fatalf("palette %v", s.Palette)
	}
}

func TestSegmentDeterministic(t *testing.T) {
	img := synth(t, "noisy-disc")
	a, err := Segment(img, PresetOptions(PresetPhoto))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Segment(img, PresetOptions(PresetPhoto))
	if !reflect.DeepEqual(a, b) {
		t.Fatal("two segmentations differ")
	}
}

// A small logo on a large canvas must survive quantisation under the
// colour presets: the reviewer found a 20×20 square on 256² vanishing.
func TestSegmentKeepsSmallSubject(t *testing.T) {
	for _, tc := range []struct {
		canvas, side int
		preset       Preset
	}{{256, 20, PresetPoster}, {1024, 40, PresetPoster}, {1024, 40, PresetPhoto}} {
		img := image.NewNRGBA(image.Rect(0, 0, tc.canvas, tc.canvas))
		for i := 0; i < len(img.Pix); i += 4 {
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = 255, 255, 255, 255
		}
		o := tc.canvas / 2
		for y := o; y < o+tc.side; y++ {
			for x := o; x < o+tc.side; x++ {
				img.SetNRGBA(x, y, color.NRGBA{0, 0, 0, 255})
			}
		}
		s, err := Segment(img, PresetOptions(tc.preset))
		if err != nil {
			t.Fatal(err)
		}
		if len(s.Palette) != 2 {
			t.Errorf("%d² canvas, %d² square, %s: palette %v, want black and white", tc.canvas, tc.side, tc.preset, s.Palette)
		}
	}
}
