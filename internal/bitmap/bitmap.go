// Package bitmap holds a packed 1-bit image and the grey/threshold steps
// that produce it from a decoded picture.
package bitmap

import "math/bits"

// Bitmap is a packed row-major 1-bit image. True means foreground.
type Bitmap struct {
	W, H  int
	words int // words per row
	bits  []uint64
}

// New returns an all-background bitmap.
func New(w, h int) *Bitmap {
	words := (w + 63) / 64
	return &Bitmap{W: w, H: h, words: words, bits: make([]uint64, words*h)}
}

// Get reports the pixel at (x, y); outside the bounds it is false.
func (b *Bitmap) Get(x, y int) bool {
	if x < 0 || y < 0 || x >= b.W || y >= b.H {
		return false
	}
	return b.bits[y*b.words+x>>6]&(1<<(uint(x)&63)) != 0
}

// Set writes the pixel at (x, y); outside the bounds it does nothing.
func (b *Bitmap) Set(x, y int, v bool) {
	if x < 0 || y < 0 || x >= b.W || y >= b.H {
		return
	}
	i := y*b.words + x>>6
	m := uint64(1) << (uint(x) & 63)
	if v {
		b.bits[i] |= m
	} else {
		b.bits[i] &^= m
	}
}

// Count returns the number of foreground pixels.
func (b *Bitmap) Count() int {
	n := 0
	for _, w := range b.bits {
		n += bits.OnesCount64(w)
	}
	return n
}

// Parse builds a bitmap from rows of '#' (foreground) and '.' (background).
// It exists for tests in this and other packages.
func Parse(rows []string) *Bitmap {
	h := len(rows)
	w := 0
	for _, r := range rows {
		if len(r) > w {
			w = len(r)
		}
	}
	b := New(w, h)
	for y, r := range rows {
		for x := 0; x < len(r); x++ {
			if r[x] == '#' {
				b.Set(x, y, true)
			}
		}
	}
	return b
}

// Clear sets every pixel to background, keeping the size.
func (b *Bitmap) Clear() {
	for i := range b.bits {
		b.bits[i] = 0
	}
}
