package bitmap

import (
	"image"
	"image/color"
	"image/draw"
	"math"
)

// Gray composites img onto white and returns 8-bit luminance
// (Y = 0.2126 R + 0.7152 G + 0.0722 B on sRGB values) with the width and height.
// Opaque NRGBA, RGBA, Gray and Paletted images are read directly; anything
// else is composited through the generic draw path first.
func Gray(img image.Image) ([]uint8, int, int) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	out := make([]uint8, w*h)
	switch src := img.(type) {
	case *image.Gray:
		for y := 0; y < h; y++ {
			off := src.PixOffset(b.Min.X, b.Min.Y+y)
			copy(out[y*w:(y+1)*w], src.Pix[off:off+w])
		}
		return out, w, h
	case *image.NRGBA:
		if opaque(src.Pix, src.Stride, b) {
			lumaRows(out, src.Pix, src.Stride, b)
			return out, w, h
		}
	case *image.RGBA:
		// Premultiplied, but with alpha 255 the channels are straight.
		if opaque(src.Pix, src.Stride, b) {
			lumaRows(out, src.Pix, src.Stride, b)
			return out, w, h
		}
	case *image.Paletted:
		lut := make([]uint8, len(src.Palette))
		for i, c := range src.Palette {
			r, g, bl, a := c.RGBA()
			// Composite the premultiplied colour onto white.
			r, g, bl = r+0xffff-a, g+0xffff-a, bl+0xffff-a
			lut[i] = luma(uint8(r>>8), uint8(g>>8), uint8(bl>>8))
		}
		for y := 0; y < h; y++ {
			off := src.PixOffset(b.Min.X, b.Min.Y+y)
			for x, idx := range src.Pix[off : off+w] {
				out[y*w+x] = lut[idx]
			}
		}
		return out, w, h
	}
	r := image.Rect(0, 0, w, h)
	flat := image.NewRGBA(r)
	draw.Draw(flat, r, image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(flat, r, img, b.Min, draw.Over)
	lumaRows(out, flat.Pix, flat.Stride, r)
	return out, w, h
}

// Per-channel luminance contributions, so the hot loop does no
// multiplication. Summed in the same order as the formula so results are
// bit-identical to computing it directly.
var lumaR, lumaG, lumaB [256]float64

func init() {
	for i := range lumaR {
		lumaR[i] = 0.2126 * float64(i)
		lumaG[i] = 0.7152 * float64(i)
		lumaB[i] = 0.0722 * float64(i)
	}
}

func luma(r, g, b uint8) uint8 {
	return uint8(math.Round(lumaR[r] + lumaG[g] + lumaB[b]))
}

// lumaRows fills out from the 4-byte pixels of an image whose Pix starts
// at the top-left of bounds b.
func lumaRows(out, pix []uint8, stride int, b image.Rectangle) {
	w, h := b.Dx(), b.Dy()
	for y := 0; y < h; y++ {
		row := pix[y*stride : y*stride+4*w]
		o := out[y*w : (y+1)*w]
		for x := range o {
			p := row[x*4:]
			o[x] = luma(p[0], p[1], p[2])
		}
	}
}

// opaque reports whether every alpha byte of an image whose Pix starts at
// the top-left of bounds b is 255.
func opaque(pix []uint8, stride int, b image.Rectangle) bool {
	w, h := b.Dx(), b.Dy()
	for y := 0; y < h; y++ {
		row := pix[y*stride : y*stride+4*w]
		for x := 3; x < len(row); x += 4 {
			if row[x] != 255 {
				return false
			}
		}
	}
	return true
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
