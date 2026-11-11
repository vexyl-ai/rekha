package gradient

// Seam smoothing. Where two gradient regions meet across a soft
// boundary (neighbouring pixels within seamStep of each other) the exact
// line between them is quantiser noise: both models paint the seam
// pixels within tolerance, and the label boundary wanders in a staircase
// that shows once each side is filled with a smooth gradient. A majority
// vote over a window moves such pixels to the label that surrounds them,
// which straightens the staircase; a pixel changes hands only when its
// new model paints it within tolerance, so crisp edges, where the other
// side's model misfits, stay where they are. A pixel with fewer than two
// neighbours of its own label is a strand or a lone island, which would
// become a path of its own: it goes to the label most of its neighbours
// carry, flat or not, whatever the fit. Otherwise flat labels (no model)
// take no part.

const (
	seamStep   = 4.0 / 255 // largest neighbour step that counts as soft
	seamRadius = 5         // half-width of the voting window
	seamSweeps = 16        // Jacobi passes, at most
	seamStrand = 2         // fewer own neighbours than this is a strand
)

// smoothSeams relabels soft boundary pixels in place. model[l] paints
// label l, or is nil for a flat label.
func smoothSeams(labels []uint8, rgb []uint8, w, h int, model []func(x, y float64) [3]float64, tol float64) {
	np := w * h
	hasModel := func(l int) bool { return l < len(model) && model[l] != nil }
	fit := func(l, p int) float64 {
		return dist(model[l](float64(p%w)+0.5, float64(p/w)+0.5), colourOf(rgb, p))
	}
	// Candidates are pixels on a soft boundary between two modelled
	// labels; after a sweep the neighbourhood of every moved pixel is
	// looked at again.
	cand := make([]int, 0, np/16)
	queued := make([]bool, np)
	enqueue := func(p int) {
		if !queued[p] {
			queued[p] = true
			cand = append(cand, p)
		}
	}
	neighbours := func(p int, f func(q int)) {
		x, y := p%w, p/w
		if x > 0 {
			f(p - 1)
		}
		if x < w-1 {
			f(p + 1)
		}
		if y > 0 {
			f(p - w)
		}
		if y < h-1 {
			f(p + w)
		}
	}
	// look reports whether p is worth a vote: on a soft boundary with
	// another modelled label, or stranded (fewer than seamStrand
	// neighbours of its own label).
	look := func(p int) (soft, strand bool) {
		a := int(labels[p])
		if !hasModel(a) {
			return false, false
		}
		c := colourOf(rgb, p)
		own := 0
		neighbours(p, func(q int) {
			b := int(labels[q])
			switch {
			case b == a:
				own++
			case hasModel(b):
				if dist(c, colourOf(rgb, q)) <= seamStep {
					soft = true
				}
			}
		})
		return soft, own < seamStrand
	}
	for p := 0; p < np; p++ {
		if soft, strand := look(p); soft || strand {
			enqueue(p)
		}
	}
	type move struct {
		p int
		l uint8
	}
	votes := make([]int, len(model))
	for sweep := 0; sweep < seamSweeps && len(cand) > 0; sweep++ {
		var moves []move
		for _, p := range cand {
			queued[p] = false
			soft, strand := look(p)
			if !soft && !strand {
				continue
			}
			a := int(labels[p])
			x, y := p%w, p/w
			for l := range votes {
				votes[l] = 0
			}
			if strand {
				neighbours(p, func(q int) { votes[labels[q]]++ })
			} else {
				for yy := max(y-seamRadius, 0); yy <= min(y+seamRadius, h-1); yy++ {
					for xx := max(x-seamRadius, 0); xx <= min(x+seamRadius, w-1); xx++ {
						votes[labels[yy*w+xx]]++
					}
				}
			}
			best := a
			for l, n := range votes {
				if n > votes[best] && (strand || hasModel(l)) {
					best = l
				}
			}
			if best != a && (strand || fit(best, p) <= tol) {
				moves = append(moves, move{p: p, l: uint8(best)})
			}
		}
		cand = cand[:0]
		for _, m := range moves {
			labels[m.p] = m.l
			enqueue(m.p)
			neighbours(m.p, enqueue)
		}
	}
}
