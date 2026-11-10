package corpus

import (
	"bytes"
	"math"
	"testing"

	"github.com/vexyl-ai/rekha/internal/geom"
)

func find(t *testing.T, name string) Synthetic {
	t.Helper()
	for _, s := range All() {
		if s.Name == name {
			return s
		}
	}
	t.Fatalf("no synthetic named %q", name)
	return Synthetic{}
}

func TestAllNamesUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range All() {
		if seen[s.Name] {
			t.Fatalf("duplicate name %q", s.Name)
		}
		seen[s.Name] = true
	}
	if len(seen) != 16 {
		t.Fatalf("got %d synthetics, want 16", len(seen))
	}
}

func TestRenderDeterministic(t *testing.T) {
	for _, s := range All() {
		a := Render(s)
		b := Render(s)
		if !bytes.Equal(a.Pix, b.Pix) {
			t.Fatalf("%s: two renders differ", s.Name)
		}
	}
}

func TestRenderSize(t *testing.T) {
	for _, s := range All() {
		img := Render(s)
		want := s.Size
		if want == 0 {
			want = 256
		}
		if img.Bounds().Dx() != want || img.Bounds().Dy() != want {
			t.Fatalf("%s: bounds %v, want %d square", s.Name, img.Bounds(), want)
		}
	}
}

func TestCircleAAHasFractionalEdge(t *testing.T) {
	aa := Render(find(t, "circle-aa"))
	hard := Render(find(t, "circle-hard"))
	frac := func(img interface{ PixOffset(x, y int) int }, pix []uint8) bool {
		for y := 0; y < 256; y++ {
			for x := 0; x < 256; x++ {
				r := pix[img.PixOffset(x, y)]
				if r > 0 && r < 255 {
					return true
				}
			}
		}
		return false
	}
	if !frac(aa, aa.Pix) {
		t.Fatal("circle-aa has no fractional edge pixel")
	}
	if frac(hard, hard.Pix) {
		t.Fatal("circle-hard has a fractional pixel")
	}
}

func TestSyntheticSizes(t *testing.T) {
	want := map[string]int{"small-glyphs": 256, "rect-rot": 256, "big-poster": 2048, "circle-aa": 256}
	seen := 0
	for _, s := range All() {
		w, ok := want[s.Name]
		if !ok {
			continue
		}
		seen++
		if b := Render(s).Bounds(); b.Dx() != w || b.Dy() != w {
			t.Errorf("%s: bounds %v, want %d square", s.Name, b, w)
		}
	}
	if seen != len(want) {
		t.Fatalf("found %d of %d named synthetics", seen, len(want))
	}
}

// TestSmallGlyphsLayout: three rows of small shapes; each row's ring has a
// white 5-px counter at its centre and black ink 8 px to the left of it.
func TestSmallGlyphsLayout(t *testing.T) {
	img := Render(find(t, "small-glyphs"))
	for _, c := range SmallGlyphRingCentres() {
		if p := img.NRGBAAt(int(c.X), int(c.Y)); p.R != 255 {
			t.Errorf("ring centre %v not white: %v", c, p)
		}
		if p := img.NRGBAAt(int(c.X)-8, int(c.Y)); p.R != 0 {
			t.Errorf("ring ink at %v-8 not black: %v", c, p)
		}
	}
	if n := len(SmallGlyphRingCentres()); n != 3 {
		t.Fatalf("%d rings, want 3", n)
	}
}

// TestRectRotCorners: the exported corners are the rectangle's corners and
// the image is black just inside them and white just outside.
func TestRectRotCorners(t *testing.T) {
	img := Render(find(t, "rect-rot"))
	cs := RectRotCorners()
	cx, cy := 0.0, 0.0
	for _, c := range cs {
		cx, cy = cx+c.X/4, cy+c.Y/4
	}
	for _, c := range cs {
		in := c.Add(geom.Vec2{X: cx, Y: cy}.Sub(c).Norm().Scale(3))
		out := c.Sub(geom.Vec2{X: cx, Y: cy}.Sub(c).Norm().Scale(3))
		if p := img.NRGBAAt(int(in.X), int(in.Y)); p.R != 0 {
			t.Errorf("inside corner %v not black: %v", c, p)
		}
		if p := img.NRGBAAt(int(out.X), int(out.Y)); p.R != 255 {
			t.Errorf("outside corner %v not white: %v", c, p)
		}
	}
	if d := cs[0].Sub(cs[1]).Len(); math.Abs(d-120) > 1e-9 && math.Abs(d-80) > 1e-9 {
		t.Errorf("side %v, want 120 or 80", d)
	}
}
