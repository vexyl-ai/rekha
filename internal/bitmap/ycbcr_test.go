package bitmap

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"math"
	"testing"
)

// TestFlattenYCbCr420InterpolatesChroma: with 4:2:0 subsampling a chroma
// sample covers a 2x2 block; the pixels between two samples take the
// 3/4, 1/4 blend libjpeg's fancy upsampling gives, not the nearest sample.
func TestFlattenYCbCr420InterpolatesChroma(t *testing.T) {
	src := image.NewYCbCr(image.Rect(0, 0, 4, 2), image.YCbCrSubsampleRatio420)
	for i := range src.Y {
		src.Y[i] = 128
	}
	src.Cb[0], src.Cb[1] = 64, 192
	src.Cr[0], src.Cr[1] = 128, 128
	out, w, h := Flatten(src)
	if w != 4 || h != 2 {
		t.Fatalf("size %dx%d", w, h)
	}
	wantCb := []uint8{64, 96, 160, 192}
	for x, cb := range wantCb {
		r, g, b := color.YCbCrToRGB(128, cb, 128)
		for y := 0; y < 2; y++ {
			p := (y*4 + x) * 3
			if out[p] != r || out[p+1] != g || out[p+2] != b {
				t.Errorf("pixel (%d,%d) = %v, want cb %d -> %v", x, y, out[p:p+3], cb, [3]uint8{r, g, b})
			}
		}
	}
}

// TestFlattenYCbCr444MatchesAt: full chroma has nothing to interpolate,
// so the fast path must agree with the generic conversion exactly.
func TestFlattenYCbCr444MatchesAt(t *testing.T) {
	src := image.NewYCbCr(image.Rect(0, 0, 5, 3), image.YCbCrSubsampleRatio444)
	for i := range src.Y {
		src.Y[i], src.Cb[i], src.Cr[i] = uint8(i*17), uint8(255-i*13), uint8(i*29)
	}
	out, _, _ := Flatten(src)
	for y := 0; y < 3; y++ {
		for x := 0; x < 5; x++ {
			c := color.RGBAModel.Convert(src.At(x, y)).(color.RGBA)
			p := (y*5 + x) * 3
			if out[p] != c.R || out[p+1] != c.G || out[p+2] != c.B {
				t.Fatalf("pixel (%d,%d) = %v, want %v", x, y, out[p:p+3], c)
			}
		}
	}
}

// TestFlattenYCbCrBeatsNearest: a smooth colour ramp through Go's JPEG
// encoder (4:2:0) comes back closer to the original with interpolated
// chroma than with the block replication the generic At path does.
func TestFlattenYCbCrBeatsNearest(t *testing.T) {
	const n = 64
	ref := image.NewNRGBA(image.Rect(0, 0, n, n))
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			ref.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 4), G: uint8(y * 4), B: uint8(255 - x*2 - y*2), A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, ref, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	dec, err := jpeg.Decode(&buf)
	if err != nil {
		t.Fatal(err)
	}
	yc, ok := dec.(*image.YCbCr)
	if !ok || yc.SubsampleRatio != image.YCbCrSubsampleRatio420 {
		t.Skipf("decoded as %T %v, test wants 4:2:0", dec, yc.SubsampleRatio)
	}
	got, _, _ := Flatten(yc)
	nearest := make([]uint8, 0, n*n*3)
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			c := color.RGBAModel.Convert(yc.At(x, y)).(color.RGBA)
			nearest = append(nearest, c.R, c.G, c.B)
		}
	}
	want, _, _ := Flatten(ref)
	fancy, block := psnr(got, want), psnr(nearest, want)
	if fancy < block+0.5 {
		t.Fatalf("fancy %.2f dB, nearest %.2f dB: interpolation should gain", fancy, block)
	}
	t.Logf("fancy %.2f dB, nearest %.2f dB", fancy, block)
}

func psnr(a, b []uint8) float64 {
	var se float64
	for i := range a {
		d := float64(a[i]) - float64(b[i])
		se += d * d
	}
	return 10 * math.Log10(255*255/(se/float64(len(a))))
}
