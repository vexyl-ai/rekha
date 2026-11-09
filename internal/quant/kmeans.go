package quant

import "github.com/vexyl-ai/rekha/internal/colour"

// nearest returns the index of the palette entry closest to c and the
// second closest (equal to the first when the palette has one entry).
func nearest(pal []colour.OKLab, c colour.OKLab) (int, int) {
	n1, n2 := 0, 0
	d1, d2 := colour.Dist(pal[0], c), 0.0
	if len(pal) > 1 {
		n2, d2 = 1, colour.Dist(pal[1], c)
		if d2 < d1 {
			n1, n2, d1, d2 = n2, n1, d2, d1
		}
	}
	for i := 2; i < len(pal); i++ {
		d := colour.Dist(pal[i], c)
		switch {
		case d < d1:
			n2, d2 = n1, d1
			n1, d1 = i, d
		case d < d2:
			n2, d2 = i, d
		}
	}
	return n1, n2
}

// KMeans refines palette by weighted Lloyd iterations over cells: each
// cell joins its nearest entry and entries move to the weighted mean of
// their cells. It stops after maxIter iterations or when no cell changes
// entry. An entry that loses every cell keeps its position. The input
// slice is not modified.
func KMeans(cells []Cell, palette []colour.OKLab, maxIter int) []colour.OKLab {
	pal := append([]colour.OKLab(nil), palette...)
	if len(pal) == 0 || len(cells) == 0 {
		return pal
	}
	assign := make([]int, len(cells))
	for i := range assign {
		assign[i] = -1
	}
	for iter := 0; iter < maxIter; iter++ {
		changed := false
		for i, c := range cells {
			n, _ := nearest(pal, c.Lab)
			if n != assign[i] {
				assign[i], changed = n, true
			}
		}
		if !changed {
			break
		}
		sum := make([]colour.OKLab, len(pal))
		wt := make([]float64, len(pal))
		for i, c := range cells {
			k, w := assign[i], float64(c.Weight)
			sum[k].L += w * c.Lab.L
			sum[k].A += w * c.Lab.A
			sum[k].B += w * c.Lab.B
			wt[k] += w
		}
		for k := range pal {
			if wt[k] > 0 {
				pal[k] = colour.OKLab{L: sum[k].L / wt[k], A: sum[k].A / wt[k], B: sum[k].B / wt[k]}
			}
		}
	}
	return pal
}
