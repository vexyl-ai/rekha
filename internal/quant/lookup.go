package quant

import (
	"math"

	"github.com/vexyl-ai/rekha/internal/colour"
)

// Lookup maps every cell at Bits per channel to a palette entry, to the
// runner-up and to Blend, the share of the entry's colour in the cell
// when the colour is projected onto the sRGB segment between the two:
// 255 means entirely the entry's colour, 128 means halfway. The entry is
// chosen between the two nearest in OKLab by the side of their sRGB
// midpoint the cell falls on, because anti-aliasing mixes colours in sRGB
// and the OKLab midpoint of black and white is grey 100, which would thin
// every dark shape on a light ground. The runner-up is then the entry
// whose sRGB segment from the chosen one passes nearest the cell: the
// colour the cell is mixed with, which for a navy pixel holding a little
// red is red, though blue lies nearer navy. Blend is the soft coverage
// mask later phases refine boundaries with.
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
		p := [3]float64{float64(r), float64(g), float64(b)}
		if project(srgb[n1], srgb[n2], p) > 0.5 {
			n1 = n2
		}
		n2, t := mixPartner(srgb, n1, p)
		l.Nearest[idx], l.Second[idx] = uint8(n1), uint8(n2)
		l.Blend[idx] = uint8(math.Round((1 - t) * 255))
	}
	return l
}

// mixPartner returns the entry other than n1 whose sRGB segment from n1
// passes nearest c, and where c falls along it (0 at n1); n1 itself with
// 0 when there is no other entry.
func mixPartner(srgb [][3]float64, n1 int, c [3]float64) (int, float64) {
	best, bestT, bestD := n1, 0.0, math.Inf(1)
	for n2 := range srgb {
		if n2 == n1 {
			continue
		}
		t := project(srgb[n1], srgb[n2], c)
		d := 0.0
		for i := range c {
			r := c[i] - (srgb[n1][i] + float64(t*(srgb[n2][i]-srgb[n1][i])))
			d += float64(r * r)
		}
		if d < bestD {
			best, bestT, bestD = n2, t, d
		}
	}
	return best, bestT
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
