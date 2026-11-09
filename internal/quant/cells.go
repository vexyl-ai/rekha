// Package quant builds a colour palette for an image and maps its pixels
// onto it. It works on cells (pixels binned to a few bits per channel), so
// its cost is bounded by the number of distinct cell colours, not by the
// pixel count.
package quant

import "github.com/vexyl-ai/rekha/internal/colour"

// Cell is one occupied colour cell: its index (see CellIndex), the OKLab
// colour of its centre and the number of pixels in it.
type Cell struct {
	Index  int
	Lab    colour.OKLab
	Weight int
}

// CellIndex returns the index of the cell holding an sRGB colour at bits
// per channel: r, g and b each truncated to bits, packed r-major.
func CellIndex(r, g, b uint8, bits int) int {
	s := 8 - bits
	return int(r>>s)<<(2*bits) | int(g>>s)<<bits | int(b>>s)
}

// CellCentre returns the sRGB colour at the centre of cell idx.
func CellCentre(idx, bits int) (r, g, b uint8) {
	s := 8 - bits
	mask := 1<<bits - 1
	half := uint8(1<<s) / 2
	return uint8(idx>>(2*bits)&mask)<<s + half, uint8(idx>>bits&mask)<<s + half, uint8(idx&mask)<<s + half
}

// Cells counts the pixels of rgb (3 bytes per pixel) per cell at bits per
// channel and returns the occupied cells in ascending index order.
func Cells(rgb []uint8, bits int) []Cell {
	counts := make([]int32, 1<<(3*bits))
	for i := 0; i+2 < len(rgb); i += 3 {
		counts[CellIndex(rgb[i], rgb[i+1], rgb[i+2], bits)]++
	}
	var cells []Cell
	for idx, n := range counts {
		if n == 0 {
			continue
		}
		r, g, b := CellCentre(idx, bits)
		cells = append(cells, Cell{Index: idx, Lab: colour.FromSRGB8(r, g, b), Weight: int(n)})
	}
	return cells
}
