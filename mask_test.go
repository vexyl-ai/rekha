package rekha

import (
	"math"
	"testing"

	"github.com/vexyl-ai/rekha/internal/contour"
	"github.com/vexyl-ai/rekha/internal/corpus"
)

// paletteIndex returns the palette entry nearest the given sRGB colour.
func paletteIndex(t *testing.T, s *Segmentation, r, g, b uint8) uint8 {
	t.Helper()
	best, bestD := -1, math.Inf(1)
	for i, c := range s.Palette {
		d := math.Hypot(math.Hypot(float64(c.R)-float64(r), float64(c.G)-float64(g)), float64(c.B)-float64(b))
		if d < bestD {
			best, bestD = i, d
		}
	}
	if best < 0 {
		t.Fatal("empty palette")
	}
	return uint8(best)
}

func TestMaskCircleAABoundary(t *testing.T) {
	img := synth(t, "circle-aa")
	o := DefaultOptions()
	s, err := Segment(img, o)
	if err != nil {
		t.Fatal(err)
	}
	layers := s.buildLayers(o)
	if len(layers) != 1 {
		t.Fatalf("%d layers, want the disc alone", len(layers))
	}
	m := s.mask(1, layers[0].shape.Outer.Pts, false, nil)
	if cov, w := m.At(128, 128); cov != 1 || w != 1 {
		t.Fatalf("interior (%v,%v), want (1,1)", cov, w)
	}
	if cov, w := m.At(5, 5); cov != 0 || w != 1 {
		t.Fatalf("exterior (%v,%v), want (0,1)", cov, w)
	}
	if cov, w := m.At(-3, 128); cov != 0 || w != 1 {
		t.Fatalf("outside the image (%v,%v), want (0,1)", cov, w)
	}
	// At y = 100 the disc's right edge crosses pixel 219 obliquely, so a
	// few pixels there hold greys between black and white.
	checked := 0
	for x := 214; x <= 226; x++ {
		g := img.NRGBAAt(x, 100).R
		if g == 0 || g == 255 {
			continue
		}
		want := float32(255-int(g)) / 255
		if cov, _ := m.At(x, 100); math.Abs(float64(cov-want)) > 1.0/255 {
			t.Fatalf("boundary pixel (%d,100) grey %d: cov %v, want %v", x, g, cov, want)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("no anti-aliased pixel found on the edge")
	}
}

func TestMaskHoleIsComplement(t *testing.T) {
	img := synth(t, "logo-holes")
	o := PresetOptions(PresetPoster)
	o.Hierarchical = HierarchicalCutout
	s, err := Segment(img, o)
	if err != nil {
		t.Fatal(err)
	}
	red := paletteIndex(t, s, 220, 30, 30)
	var hole *contour.Contour
	for _, l := range s.buildLayers(o) {
		if l.colour == int(red) && len(l.shape.Holes) == 1 {
			hole = &l.shape.Holes[0]
		}
	}
	if hole == nil {
		t.Fatal("no red shape with one hole")
	}
	m := s.mask(red, hole.Pts, true, nil)
	if cov, w := m.At(80, 128); cov != 1 || w != 1 {
		t.Fatalf("inside the hole (%v,%v), want (1,1)", cov, w)
	}
	if cov, _ := m.At(50, 128); cov != 0 {
		t.Fatalf("in the ring body: cov %v, want 0", cov)
	}
	// The hole's rounded top-left corner (radius 12 about (76,104)) passes
	// obliquely through the pixels around (67,95); those show the
	// complement of red's share.
	checked := 0
	for x := 64; x <= 72; x++ {
		p := 95*s.Width + x
		var share float32
		switch {
		case s.Labels[p] == red:
			share = float32(s.Blend[p]) / 255
		case s.Second[p] == red:
			share = 1 - float32(s.Blend[p])/255
		default:
			continue
		}
		if share == 0 || share == 1 {
			continue
		}
		if cov, _ := m.At(x, 95); math.Abs(float64(cov-(1-share))) > 1e-6 {
			t.Fatalf("hole boundary pixel (%d,95): cov %v, want %v", x, cov, 1-share)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("no anti-aliased pixel found on the hole's edge")
	}
}

func TestMaskExtensionBandIsHard(t *testing.T) {
	img := synth(t, "aa-abut")
	o := PresetOptions(PresetPoster)
	s, err := Segment(img, o)
	if err != nil {
		t.Fatal(err)
	}
	red, blue := paletteIndex(t, s, 220, 30, 30), paletteIndex(t, s, 30, 60, 220)
	var extended *layer
	for i := range s.buildLayers(o) {
		l := &s.buildLayers(o)[i]
		if l.colour == int(red) && len(l.band) > 0 {
			extended = l
		}
	}
	if extended == nil {
		t.Fatal("red was not extended into blue")
	}
	m := s.mask(red, extended.contours[0].Pts, false, extended.inBand)
	checked := 0
	for _, p := range extended.band {
		if s.Labels[p] != blue {
			continue
		}
		x, y := int(p)%s.Width, int(p)/s.Width
		if cov, w := m.At(x, y); cov != 1 || w != 1 {
			t.Fatalf("band pixel (%d,%d): (%v,%v), want (1,1)", x, y, cov, w)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("no blue-labelled band pixel")
	}
	_ = corpus.Size
}

// TestMaskHoleEqualsEnclosedOuter: in cutout, the navy page's hole around
// the red shape and the red shape's outer are the same curve seen from
// either side, so their masks agree pixel for pixel: fully red pixels on
// the hole's edge are 1 whatever their (meaningless) second colour is.
func TestMaskHoleEqualsEnclosedOuter(t *testing.T) {
	img := synth(t, "logo-holes")
	o := PresetOptions(PresetPoster)
	o.Hierarchical = HierarchicalCutout
	s, err := Segment(img, o)
	if err != nil {
		t.Fatal(err)
	}
	red, navy := paletteIndex(t, s, 220, 30, 30), paletteIndex(t, s, 20, 30, 60)
	var outer, hole *contour.Contour
	layers := s.buildLayers(o)
	for i, l := range layers {
		if l.colour == int(red) && len(l.shape.Holes) == 1 {
			outer = &layers[i].shape.Outer
		}
	}
	if outer == nil {
		t.Fatal("red shape with one hole not found")
	}
	for i, l := range layers {
		if l.colour == int(navy) && l.depth == 0 {
			for k := range l.shape.Holes {
				if l.shape.Holes[k].Area2 == -outer.Area2 {
					hole = &layers[i].shape.Holes[k]
				}
			}
		}
	}
	if hole == nil {
		t.Fatal("navy hole around the red shape not found")
	}
	om, hm := s.mask(red, outer.Pts, false, nil), s.mask(navy, hole.Pts, true, nil)
	if om.x0 != hm.x0 || om.y0 != hm.y0 || om.w != hm.w || om.h != hm.h {
		t.Fatalf("boxes differ: %+v vs %+v", *om, *hm)
	}
	for i := range om.cov {
		if om.cov[i] != hm.cov[i] {
			t.Fatalf("pixel (%d,%d): outer %v, hole %v", om.x0+i%om.w, om.y0+i/om.w, om.cov[i], hm.cov[i])
		}
	}
}
