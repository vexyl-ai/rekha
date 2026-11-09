// Package metrics scores traced output: pixel fidelity and SVG size.
package metrics

import (
	"errors"
	"image"
	"image/color"
	"image/draw"
	"math"
)

// ErrDimensionMismatch is returned when the two images have different bounds.
var ErrDimensionMismatch = errors.New("metrics: image dimensions differ")

// PSNR compares a and b over RGB after compositing both onto white, 8-bit
// per channel, and returns the peak signal-to-noise ratio in dB. Identical
// images give +Inf.
func PSNR(a, b image.Image) (float64, error) {
	if a.Bounds().Size() != b.Bounds().Size() {
		return 0, ErrDimensionMismatch
	}
	fa := flattenOnWhite(a)
	fb := flattenOnWhite(b)
	var sum float64
	n := 0
	for i := 0; i+3 < len(fa.Pix); i += 4 {
		for c := 0; c < 3; c++ {
			d := float64(fa.Pix[i+c]) - float64(fb.Pix[i+c])
			sum += d * d
		}
		n += 3
	}
	if n == 0 || sum == 0 {
		return math.Inf(1), nil
	}
	mse := sum / float64(n)
	return 20*math.Log10(255) - 10*math.Log10(mse), nil
}

func flattenOnWhite(src image.Image) *image.NRGBA {
	r := image.Rect(0, 0, src.Bounds().Dx(), src.Bounds().Dy())
	dst := image.NewNRGBA(r)
	draw.Draw(dst, r, image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(dst, r, src, src.Bounds().Min, draw.Over)
	return dst
}
