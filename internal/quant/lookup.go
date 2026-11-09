package quant

import (
	"math"

	"github.com/vexyl-ai/rekha/internal/colour"
)

// Lookup maps every cell at Bits per channel to a palette entry, to the
// runner-up and to Blend, the share of the entry's colour in the cell
// when the colour is projected onto the sRGB segment between the two:
// 255 means entirely the entry's colour, 128 means halfway. The two
// candidates are the nearest entries in OKLab; between them the cell goes
// to whichever side of the sRGB midpoint it falls, because anti-aliasing
// mixes colours in sRGB and the OKLab midpoint of black and white is grey
// 100, which would thin every dark shape on a light ground. Blend is the
// soft coverage mask later phases refine boundaries with.
type Lookup struct {
	Bits                   int
	Nearest, Second, Blend []uint8
}

// NewLookup builds the table for palette at bits per channel.
func NewLookup(palette []colour.OKLab, bits int) *Lookup {
	n := 1 << (3 * bits)
	l := &Lookup{Bits: bits, Nearest: make([]uint8, n), Second: make([]uint8, n), Blend: make([]uint8, n)}
	srgb := make([][3]float64, len(palette))
	for i, p := range palette {
		r, g, b := p.ToSRGB8()
		srgb[i] = [3]float64{float64(r), float64(g), float64(b)}
	}
	for idx := 0; idx < n; idx++ {
		r, g, b := CellCentre(idx, bits)
		c := colour.FromSRGB8(r, g, b)
		n1, n2 := nearest(palette, c)
		t := project(srgb[n1], srgb[n2], [3]float64{float64(r), float64(g), float64(b)})
		if t > 0.5 {
			n1, n2, t = n2, n1, 1-t
		}
		l.Nearest[idx], l.Second[idx] = uint8(n1), uint8(n2)
		l.Blend[idx] = uint8(math.Round((1 - t) * 255))
	}
	return l
}

// project returns where c falls along the segment p1→p2, 0 at p1 and 1 at
// p2, clamped to [0, 1]; 0 when the segment is degenerate.
func project(p1, p2, c [3]float64) float64 {
	var num, den float64
	for i := range p1 {
		d := p2[i] - p1[i]
		num += float64((c[i] - p1[i]) * d)
		den += float64(d * d)
	}
	if den == 0 {
		return 0
	}
	return math.Min(math.Max(num/den, 0), 1)
}

// Map fills labels, second and blend (one byte per pixel) from rgb (3
// bytes per pixel).
func (l *Lookup) Map(rgb []uint8, labels, second, blend []uint8) {
	for i, p := 0, 0; i+2 < len(rgb); i, p = i+3, p+1 {
		idx := CellIndex(rgb[i], rgb[i+1], rgb[i+2], l.Bits)
		labels[p], second[p], blend[p] = l.Nearest[idx], l.Second[idx], l.Blend[idx]
	}
}
