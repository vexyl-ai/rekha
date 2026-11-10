package bitmap

import (
	"image"
	"image/color"
	"testing"
)

func TestTransparentNilForOpaque(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 2))
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 255
	}
	if got := Transparent(img); got != nil {
		t.Fatalf("opaque NRGBA: got %v, want nil", got)
	}
	if got := Transparent(image.NewGray(image.Rect(0, 0, 4, 2))); got != nil {
		t.Fatalf("Gray: got %v, want nil", got)
	}
}

func TestTransparentMarksClearPixels(t *testing.T) {
	img := image.NewNRGBA(image.Rect(2, 1, 6, 3)) // bounds not at the origin
	img.SetNRGBA(2, 1, color.NRGBA{255, 0, 0, 255})
	img.SetNRGBA(5, 2, color.NRGBA{0, 0, 255, 1}) // a trace of coverage is not transparent
	got := Transparent(img)
	want := []bool{false, true, true, true, true, true, true, false}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("pixel %d: got %v, want %v (%v)", i, got[i], want[i], got)
		}
	}
	pal := image.NewPaletted(image.Rect(0, 0, 2, 1), color.Palette{color.NRGBA{0, 0, 0, 0}, color.NRGBA{9, 9, 9, 255}})
	pal.Pix[1] = 1
	if got := Transparent(pal); !got[0] || got[1] {
		t.Fatalf("paletted: got %v, want [true false]", got)
	}
}
