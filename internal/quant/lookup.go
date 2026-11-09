package quant

import (
	"math"

	"github.com/vexyl-ai/rekha/internal/colour"
)

// Lookup maps every cell at Bits per channel to its nearest and second
// nearest palette entries and to Blend, the share of the cell's colour
// attributable to the nearest entry when the colour is projected onto the
// OKLab segment between the two: 255 means entirely the nearest colour,
// 128 means halfway. Blend is the soft coverage mask later phases refine
// boundaries with.
type Lookup struct {
	Bits                   int
	Nearest, Second, Blend []uint8
}

// NewLookup builds the table for palette at bits per channel.
func NewLookup(palette []colour.OKLab, bits int) *Lookup {
	n := 1 << (3 * bits)
	l := &Lookup{Bits: bits, Nearest: make([]uint8, n), Second: make([]uint8, n), Blend: make([]uint8, n)}
	for idx := 0; idx < n; idx++ {
		r, g, b := CellCentre(idx, bits)
		c := colour.FromSRGB8(r, g, b)
		n1, n2 := nearest(palette, c)
		l.Nearest[idx], l.Second[idx] = uint8(n1), uint8(n2)
		l.Blend[idx] = blend(palette[n1], palette[n2], c)
	}
	return l
}

// blend projects c onto the segment p1→p2 and returns the share of p1,
// in 128..255.
func blend(p1, p2, c colour.OKLab) uint8 {
	dl, da, db := p2.L-p1.L, p2.A-p1.A, p2.B-p1.B
	den := float64(dl*dl) + float64(da*da) + float64(db*db)
	if den == 0 {
		return 255
	}
	t := (float64((c.L-p1.L)*dl) + float64((c.A-p1.A)*da) + float64((c.B-p1.B)*db)) / den
	t = math.Min(math.Max(t, 0), 0.5)
	return uint8(math.Round((1 - t) * 255))
}

// Map fills labels, second and blend (one byte per pixel) from rgb (3
// bytes per pixel).
func (l *Lookup) Map(rgb []uint8, labels, second, blend []uint8) {
	for i, p := 0, 0; i+2 < len(rgb); i, p = i+3, p+1 {
		idx := CellIndex(rgb[i], rgb[i+1], rgb[i+2], l.Bits)
		labels[p], second[p], blend[p] = l.Nearest[idx], l.Second[idx], l.Blend[idx]
	}
}
