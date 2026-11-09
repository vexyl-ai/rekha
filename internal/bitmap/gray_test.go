package bitmap

import (
	"image"
	"image/color"
	"testing"
)

func TestGrayCompositesAlphaOnWhite(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 3, 1))
	img.SetNRGBA(0, 0, color.NRGBA{0, 0, 0, 0})
	img.SetNRGBA(1, 0, color.NRGBA{0, 0, 0, 255})
	img.SetNRGBA(2, 0, color.NRGBA{255, 0, 0, 255})
	g, w, h := Gray(img)
	if w != 3 || h != 1 {
		t.Fatalf("size %dx%d", w, h)
	}
	if g[0] != 255 || g[1] != 0 || g[2] != 54 {
		t.Fatalf("gray = %v, want [255 0 54]", g)
	}
}

func TestFromGrayThreshold(t *testing.T) {
	b := FromGray([]uint8{0, 127, 128, 255}, 4, 1, 128)
	want := []bool{true, true, false, false}
	for x, w := range want {
		if b.Get(x, 0) != w {
			t.Fatalf("x=%d got %v want %v", x, b.Get(x, 0), w)
		}
	}
}

func TestOtsuBimodal(t *testing.T) {
	g := make([]uint8, 200)
	for i := range g {
		if i < 100 {
			g[i] = 20
		} else {
			g[i] = 200
		}
	}
	th := Otsu(g)
	if th <= 20 || th > 200 {
		t.Fatalf("Otsu = %d, want in (20,200]", th)
	}
}

func TestOtsuFlat(t *testing.T) {
	g := []uint8{77, 77, 77}
	if th := Otsu(g); th != 77 {
		t.Fatalf("Otsu flat = %d, want 77", th)
	}
}
