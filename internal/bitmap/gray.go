package bitmap

import (
	"image"
	"image/color"
	"image/draw"
	"math"
)

// Gray composites img onto white and returns 8-bit luminance
// (Y = 0.2126 R + 0.7152 G + 0.0722 B on sRGB values) with the width and height.
func Gray(img image.Image) ([]uint8, int, int) {
	r := image.Rect(0, 0, img.Bounds().Dx(), img.Bounds().Dy())
	flat := image.NewNRGBA(r)
	draw.Draw(flat, r, image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(flat, r, img, img.Bounds().Min, draw.Over)
	out := make([]uint8, r.Dx()*r.Dy())
	for i := range out {
		p := flat.Pix[i*4:]
		y := 0.2126*float64(p[0]) + 0.7152*float64(p[1]) + 0.0722*float64(p[2])
		out[i] = uint8(math.Round(y))
	}
	return out, r.Dx(), r.Dy()
}

// FromGray thresholds: a pixel is foreground when gray < threshold.
func FromGray(gray []uint8, w, h int, threshold uint8) *Bitmap {
	b := New(w, h)
	for y := 0; y < h; y++ {
		row := gray[y*w : (y+1)*w]
		for x, g := range row {
			if g < threshold {
				b.Set(x, y, true)
			}
		}
	}
	return b
}

// Otsu returns the threshold that maximises between-class variance of the
// grey histogram. Pixels with value < threshold are foreground. A flat
// image returns its single value.
func Otsu(gray []uint8) uint8 {
	var hist [256]int
	for _, g := range gray {
		hist[g]++
	}
	total := len(gray)
	if total == 0 {
		return 128
	}
	sum := 0.0
	for i, c := range hist {
		sum += float64(i) * float64(c)
	}
	bestT, bestVar := -1, -1.0
	wB, sumB := 0, 0.0
	for t := 0; t < 256; t++ {
		// Classes: [0,t) background-of-threshold = foreground pixels, [t,255].
		wB += hist[t]
		sumB += float64(t) * float64(hist[t])
		wF := total - wB
		if wB == 0 || wF == 0 {
			continue
		}
		mB := sumB / float64(wB)
		mF := (sum - sumB) / float64(wF)
		v := float64(wB) * float64(wF) * (mB - mF) * (mB - mF)
		if v > bestVar {
			bestVar, bestT = v, t+1
		}
	}
	if bestT < 0 {
		// Flat image: every pixel has the same value.
		for i, c := range hist {
			if c > 0 {
				return uint8(i)
			}
		}
		return 128
	}
	return uint8(bestT)
}
