package bitmap

import "image"

// Transparent reports which pixels of img are fully transparent, row-major
// over the bounds, or nil when none is: an image without an alpha channel,
// or one whose every pixel has some coverage. Opaque NRGBA, RGBA and
// Paletted images are read directly; anything else goes through At.
func Transparent(img image.Image) []bool {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	var out []bool
	mark := func(x, y int) {
		if out == nil {
			out = make([]bool, w*h)
		}
		out[y*w+x] = true
	}
	switch src := img.(type) {
	case *image.Gray, *image.Gray16, *image.YCbCr, *image.CMYK:
		return nil
	case *image.NRGBA:
		alphaRows(src.Pix, src.Stride, b, mark)
	case *image.RGBA:
		alphaRows(src.Pix, src.Stride, b, mark)
	case *image.Paletted:
		clear := make([]bool, len(src.Palette))
		any := false
		for i, c := range src.Palette {
			_, _, _, a := c.RGBA()
			clear[i] = a == 0
			any = any || clear[i]
		}
		if !any {
			return nil
		}
		for y := 0; y < h; y++ {
			off := src.PixOffset(b.Min.X, b.Min.Y+y)
			for x, i := range src.Pix[off : off+w] {
				if clear[i] {
					mark(x, y)
				}
			}
		}
	default:
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				if _, _, _, a := img.At(b.Min.X+x, b.Min.Y+y).RGBA(); a == 0 {
					mark(x, y)
				}
			}
		}
	}
	return out
}

// alphaRows calls mark for every pixel of a 4-byte-per-pixel buffer whose
// alpha is zero.
func alphaRows(pix []uint8, stride int, b image.Rectangle, mark func(x, y int)) {
	w, h := b.Dx(), b.Dy()
	for y := 0; y < h; y++ {
		row := pix[y*stride : y*stride+4*w]
		for x := 3; x < len(row); x += 4 {
			if row[x] == 0 {
				mark(x/4, y)
			}
		}
	}
}
