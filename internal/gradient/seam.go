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
	seamSweeps = 16        // Jacobi passes per radius, at most
	seamStrand = 2         // fewer own neighbours than this is a strand
)

// seamRadii are the half-widths of the voting window, coarse to fine:
// a window only straightens steps shorter than its width, so the wide
// one takes a staircase whose steps run tens of pixels and the narrow
// one cleans the corners it leaves.
var seamRadii = [...]int{32, 5}

// smoothSeams relabels soft boundary pixels in place. model[l] paints
// label l, or is nil for a flat label.
func smoothSeams(labels []uint8, rgb []uint8, w, h int, model []func(x, y float64) [3]float64, tol float64) {
	s := &seams{labels: labels, rgb: rgb, w: w, h: h, model: model, tol: tol}
	s.queued = make([]bool, w*h)
	s.votes = make([]int, len(model))
	for _, radius := range seamRadii {
		for p := 0; p < w*h; p++ {
			if soft, strand := s.look(p); soft || strand {
				s.enqueue(p)
			}
		}
		for sweep := 0; sweep < seamSweeps && len(s.cand) > 0; sweep++ {
			s.sweep(radius)
		}
		s.drop()
	}
	// A capped last sweep may have cut a strand loose: peel what is
	// left, strands only, until nothing moves.
	for p := 0; p < w*h; p++ {
		if _, strand := s.look(p); strand {
			s.enqueue(p)
		}
	}
	for len(s.cand) > 0 {
		s.sweep(0)
	}
}

type seams struct {
	labels, rgb []uint8
	w, h        int
	model       []func(x, y float64) [3]float64
	tol         float64
	cand        []int // pixels to look at in the next sweep
	queued      []bool
	votes       []int
}

func (s *seams) hasModel(l int) bool { return l < len(s.model) && s.model[l] != nil }

func (s *seams) fit(l, p int) float64 {
	return dist(s.model[l](float64(p%s.w)+0.5, float64(p/s.w)+0.5), colourOf(s.rgb, p))
}

// drop empties the queue, so a pixel left over at the sweep cap can be
// queued again later.
func (s *seams) drop() {
	for _, p := range s.cand {
		s.queued[p] = false
	}
	s.cand = s.cand[:0]
}

func (s *seams) enqueue(p int) {
	if !s.queued[p] {
		s.queued[p] = true
		s.cand = append(s.cand, p)
	}
}

func (s *seams) neighbours(p int, f func(q int)) {
	x, y := p%s.w, p/s.w
	if x > 0 {
		f(p - 1)
	}
	if x < s.w-1 {
		f(p + 1)
	}
	if y > 0 {
		f(p - s.w)
	}
	if y < s.h-1 {
		f(p + s.w)
	}
}

// look reports whether p is worth a vote: on a soft boundary with
// another modelled label, or stranded (fewer than seamStrand neighbours
// of its own label).
func (s *seams) look(p int) (soft, strand bool) {
	a := int(s.labels[p])
	if !s.hasModel(a) {
		return false, false
	}
	c := colourOf(s.rgb, p)
	own := 0
	s.neighbours(p, func(q int) {
		b := int(s.labels[q])
		switch {
		case b == a:
			own++
		case s.hasModel(b):
			if dist(c, colourOf(s.rgb, q)) <= seamStep {
				soft = true
			}
		}
	})
	return soft, own < seamStrand
}

// sweep votes on every queued pixel in queue order, moving each at
// once (two neighbours voted together would swap for ever), and queues
// the neighbourhood of each move for the next sweep. The window is
// clipped symmetrically at the image border: a window cut on one side
// would make the larger region eat the smaller one along a straight
// seam, one pixel a sweep. Radius 0 votes on strands only.
func (s *seams) sweep(radius int) {
	cand := s.cand
	s.cand = nil
	for _, p := range cand {
		s.queued[p] = false
		soft, strand := s.look(p)
		if !strand && (!soft || radius == 0) {
			continue
		}
		a := int(s.labels[p])
		for l := range s.votes {
			s.votes[l] = 0
		}
		if strand {
			s.neighbours(p, func(q int) { s.votes[s.labels[q]]++ })
		} else {
			x, y := p%s.w, p/s.w
			rx := min(radius, x, s.w-1-x)
			ry := min(radius, y, s.h-1-y)
			for yy := y - ry; yy <= y+ry; yy++ {
				for _, l := range s.labels[yy*s.w+x-rx : yy*s.w+x+rx+1] {
					s.votes[l]++
				}
			}
		}
		best := a
		for l, n := range s.votes {
			if n > s.votes[best] && (strand || s.hasModel(l)) {
				best = l
			}
		}
		if best != a && (strand || s.fit(best, p) <= s.tol) {
			s.labels[p] = uint8(best)
			s.enqueue(p)
			s.neighbours(p, s.enqueue)
		}
	}
}
