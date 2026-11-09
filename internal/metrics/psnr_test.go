package metrics

import (
	"errors"
	"image"
	"image/color"
	"math"
	"testing"
)

func solid(w, h int, c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

func TestPSNRIdentical(t *testing.T) {
	a := solid(8, 8, color.NRGBA{255, 0, 0, 255})
	b := solid(8, 8, color.NRGBA{255, 0, 0, 255})
	got, err := PSNR(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if !math.IsInf(got, 1) {
		t.Fatalf("got %v, want +Inf", got)
	}
}

func TestPSNRKnownValue(t *testing.T) {
	a := solid(2, 2, color.NRGBA{100, 100, 100, 255})
	b := solid(2, 2, color.NRGBA{116, 116, 116, 255})
	got, err := PSNR(a, b)
	if err != nil {
		t.Fatal(err)
	}
	want := 20*math.Log10(255) - 10*math.Log10(256)
	if math.Abs(got-want) > 0.01 {
		t.Fatalf("got %.4f, want %.4f", got, want)
	}
}

func TestPSNRDimensionMismatch(t *testing.T) {
	a := solid(8, 8, color.NRGBA{0, 0, 0, 255})
	b := solid(8, 9, color.NRGBA{0, 0, 0, 255})
	_, err := PSNR(a, b)
	if !errors.Is(err, ErrDimensionMismatch) {
		t.Fatalf("got %v, want ErrDimensionMismatch", err)
	}
}

func TestPSNRAlphaOnWhite(t *testing.T) {
	a := solid(4, 4, color.NRGBA{0, 0, 0, 0})
	b := solid(4, 4, color.NRGBA{255, 255, 255, 255})
	got, err := PSNR(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if !math.IsInf(got, 1) {
		t.Fatalf("got %v, want +Inf (transparent composites to white)", got)
	}
}
