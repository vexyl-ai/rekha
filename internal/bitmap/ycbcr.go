package bitmap

import (
	"image"
	"image/color"
)

// flattenYCbCr converts a decoded JPEG to RGB rows. Go's image/jpeg
// leaves chroma at its subsampled resolution and the generic conversion
// replicates each chroma sample over its block, which puts a 2 px
// staircase on every colour edge. This interpolates the chroma planes the
// way libjpeg's fancy upsampling does (a triangle filter: the pixel next
// to a sample takes 3/4 of it and 1/4 of the neighbour), which is 2-3 dB
// closer to the pixels that were encoded.
func flattenYCbCr(out []uint8, src *image.YCbCr, b image.Rectangle) {
	w, h := b.Dx(), b.Dy()
	sx, sy := chromaStep(src.SubsampleRatio)
	cb := src.Bounds()
	xs := taps(b.Min.X, w, sx, cb.Min.X, (cb.Dx()+sx-1)/sx)
	ys := taps(b.Min.Y, h, sy, cb.Min.Y, (cb.Dy()+sy-1)/sy)
	for y := 0; y < h; y++ {
		yr := src.Y[src.YOffset(b.Min.X, b.Min.Y+y):]
		ty := ys[y]
		row0 := src.Cb[ty.i0*src.CStride:]
		row1 := src.Cb[ty.i1*src.CStride:]
		crow0 := src.Cr[ty.i0*src.CStride:]
		crow1 := src.Cr[ty.i1*src.CStride:]
		row := out[y*w*3 : (y+1)*w*3]
		for x := 0; x < w; x++ {
			tx := xs[x]
			w00, w01 := ty.w0*tx.w0, ty.w0*tx.w1
			w10, w11 := ty.w1*tx.w0, ty.w1*tx.w1
			cbv := (w00*int(row0[tx.i0]) + w01*int(row0[tx.i1]) + w10*int(row1[tx.i0]) + w11*int(row1[tx.i1]) + 128) >> 8
			crv := (w00*int(crow0[tx.i0]) + w01*int(crow0[tx.i1]) + w10*int(crow1[tx.i0]) + w11*int(crow1[tx.i1]) + 128) >> 8
			r, g, bl := color.YCbCrToRGB(yr[x], uint8(cbv), uint8(crv))
			row[x*3], row[x*3+1], row[x*3+2] = r, g, bl
		}
	}
}

// chromaStep is the luma pixels per chroma sample along each axis.
func chromaStep(r image.YCbCrSubsampleRatio) (sx, sy int) {
	switch r {
	case image.YCbCrSubsampleRatio422:
		return 2, 1
	case image.YCbCrSubsampleRatio420:
		return 2, 2
	case image.YCbCrSubsampleRatio440:
		return 1, 2
	case image.YCbCrSubsampleRatio411:
		return 4, 1
	case image.YCbCrSubsampleRatio410:
		return 4, 2
	default:
		return 1, 1
	}
}

// tap is one axis of a bilinear chroma lookup: the two sample indices
// (into the chroma plane, relative to its origin) and their weights in
// sixteenths, summing to 16.
type tap struct {
	i0, i1 int
	w0, w1 int
}

// taps gives, for n luma positions starting at min, the chroma samples
// to blend. Chroma sample j is centred on luma positions j*step +
// (step-1)/2 + 1/2, in the plane's coordinates (origin cmin, cn samples);
// a position before the first centre or past the last takes that sample
// alone. With step 1 every tap is the sample itself.
func taps(min, n, step, cmin, cn int) []tap {
	out := make([]tap, n)
	for k := range out {
		// Position of this luma pixel in chroma sample units, in
		// sixteenths and offset so that sample j's centre lands on 16*j.
		u := ((min+k-cmin)*2+1)*16/(2*step) - 8
		j := u >> 4 // floor division on the fixed point
		f := u - j*16
		i0, i1 := j, j+1
		switch {
		case i0 < 0:
			i0, i1, f = 0, 0, 0
		case i1 >= cn:
			i0, i1, f = cn-1, cn-1, 0
		}
		out[k] = tap{i0: i0, i1: i1, w0: 16 - f, w1: f}
	}
	return out
}
