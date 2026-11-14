package quant

import (
	"math"
	"sort"

	"github.com/vexyl-ai/rekha/internal/colour"
)

// Histogram geometry for Wu's method: OKLab is binned to wuBins per axis
// over L in [0,1] and a, b in [-wuAB, wuAB].
const (
	wuBins = 32
	wuAB   = 0.4
	wuN    = wuBins + 1 // cumulative arrays are 1-based
)

// MaxColors is the largest palette the quantiser produces.
const MaxColors = 256

// moments holds cumulative (inclusive prefix) sums over the histogram so
// any box's weight, colour sum and squared-colour sum come from eight
// corner lookups.
type moments struct {
	w, l, a, b, sq []float64
}

func bin(v, lo, hi float64) int {
	k := int((v - lo) / (hi - lo) * wuBins)
	return min(max(k, 0), wuBins-1)
}

func at(x, y, z int) int { return (x*wuN+y)*wuN + z }

func newMoments(cells []Cell) *moments {
	n := wuN * wuN * wuN
	m := &moments{make([]float64, n), make([]float64, n), make([]float64, n), make([]float64, n), make([]float64, n)}
	for _, c := range cells {
		i := at(bin(c.Lab.L, 0, 1)+1, bin(c.Lab.A, -wuAB, wuAB)+1, bin(c.Lab.B, -wuAB, wuAB)+1)
		w := float64(c.Weight)
		m.w[i] += w
		m.l[i] += w * c.Lab.L
		m.a[i] += w * c.Lab.A
		m.b[i] += w * c.Lab.B
		m.sq[i] += w * (float64(c.Lab.L*c.Lab.L) + float64(c.Lab.A*c.Lab.A) + float64(c.Lab.B*c.Lab.B))
	}
	// Prefix sums along each axis in turn.
	for _, arr := range [][]float64{m.w, m.l, m.a, m.b, m.sq} {
		for x := 1; x < wuN; x++ {
			for y := 1; y < wuN; y++ {
				for z := 1; z < wuN; z++ {
					arr[at(x, y, z)] += arr[at(x-1, y, z)] + arr[at(x, y-1, z)] + arr[at(x, y, z-1)] -
						arr[at(x-1, y-1, z)] - arr[at(x-1, y, z-1)] - arr[at(x, y-1, z-1)] + arr[at(x-1, y-1, z-1)]
				}
			}
		}
	}
	return m
}

// box is a half-open range of bins on each axis: [l0, l1) etc.
type box struct{ l0, l1, a0, a1, b0, b1 int }

// vol sums arr over the box by inclusion–exclusion on the cumulative array.
func vol(arr []float64, bx box) float64 {
	return arr[at(bx.l1, bx.a1, bx.b1)] - arr[at(bx.l0, bx.a1, bx.b1)] - arr[at(bx.l1, bx.a0, bx.b1)] - arr[at(bx.l1, bx.a1, bx.b0)] +
		arr[at(bx.l0, bx.a0, bx.b1)] + arr[at(bx.l0, bx.a1, bx.b0)] + arr[at(bx.l1, bx.a0, bx.b0)] - arr[at(bx.l0, bx.a0, bx.b0)]
}

// stats returns the weight, sum of squared error about the centroid and
// the centroid of a box.
func (m *moments) stats(bx box) (w, sse float64, c colour.OKLab) {
	w = vol(m.w, bx)
	if w == 0 {
		return 0, 0, colour.OKLab{}
	}
	l, a, b := vol(m.l, bx), vol(m.a, bx), vol(m.b, bx)
	sse = vol(m.sq, bx) - (float64(l*l)+float64(a*a)+float64(b*b))/w
	return w, math.Max(sse, 0), colour.OKLab{L: l / w, A: a / w, B: b / w}
}

// split finds the cut of bx that minimises the summed SSE of the halves.
// It returns the two halves and false when no cut leaves both halves
// with weight.
func (m *moments) split(bx box) (box, box, bool) {
	bestGain, found := -1.0, false
	var best1, best2 box
	wAll := vol(m.w, bx)
	lAll, aAll, bAll := vol(m.l, bx), vol(m.a, bx), vol(m.b, bx)
	try := func(lo, hi box) {
		w1 := vol(m.w, lo)
		w2 := wAll - w1
		if w1 <= 0 || w2 <= 0 {
			return
		}
		l1, a1, b1 := vol(m.l, lo), vol(m.a, lo), vol(m.b, lo)
		l2, a2, b2 := lAll-l1, aAll-a1, bAll-b1
		// Splitting reduces SSE by the gain in |sum|²/w; maximise it.
		gain := (float64(l1*l1)+float64(a1*a1)+float64(b1*b1))/w1 + (float64(l2*l2)+float64(a2*a2)+float64(b2*b2))/w2
		if gain > bestGain {
			bestGain, found, best1, best2 = gain, true, lo, hi
		}
	}
	for c := bx.l0 + 1; c < bx.l1; c++ {
		lo, hi := bx, bx
		lo.l1, hi.l0 = c, c
		try(lo, hi)
	}
	for c := bx.a0 + 1; c < bx.a1; c++ {
		lo, hi := bx, bx
		lo.a1, hi.a0 = c, c
		try(lo, hi)
	}
	for c := bx.b0 + 1; c < bx.b1; c++ {
		lo, hi := bx, bx
		lo.b1, hi.b0 = c, c
		try(lo, hi)
	}
	return best1, best2, found
}

// A far cell (beyond tol from its box's centroid) is explained, and so
// not a hidden subject, when it lies within tol of an sRGB mixture of the
// centroid and another box's centroid (anti-aliasing between the two
// colours, see colour.MixDist) or within tailReach·tol of the centroid (the tail of noise
// around a flat colour). Of the unexplained far mass, one cell holding
// coherentShare of it with at least coherentMin weight marks a subject:
// noise spreads its far mass over many cells, a small flat shape puts it
// all in one.
//
// An explained cell can still be a subject: a soft grey shadow on a white
// page lies on the black–white mixture segment, and sRGB quantisation
// puts a flat colour's noise a long OKLab way from a dark centroid. The
// heaviest cell of each explanation is a subject when it holds at least
// 1/subjectShare of the whole image and stands alone: the box's other
// cells within adjacentCells cells of it in sRGB hold under 1/haloShare
// of its weight between them.
//
// A shaded object spreads its mass over many cells, and two small objects
// in one box dilute each other, so no single cell passes either test.
// Each pile is therefore also judged by its heaviest colour group: the
// cells within tol of a seed, seeds tried heaviest first. A far group is
// coherent on the same terms as a cell; a tail or mixture group is a
// subject on the same terms, with the halo taken outside the group. A
// group counts only when it holds 1/groupFloor of the image: the far tail
// of a gradient ramp on a large image is a few hundred pixels spread thin,
// and splitting on it cuts the ramp. A flat colour puts its mass in one cell
// and only its anti-aliased rim in the cells beside it; a noise tail
// spreads its mass over the neighbours, and a plateau at the end of a
// ramp trails a share of its mass up the ramp (cutting that plateau out
// would run a region boundary through the ramp, where the gradient fit
// wants the box whole). Such a subject becomes a palette entry of its
// own and its box is done: splitting the box by variance instead would
// cut the edge's mixture cells into boxes of their own on the way to it.
const (
	coherentShare = 0.25
	coherentMin   = 4
	tailReach     = 2.0
	subjectShare  = 64
	haloShare     = 16
	adjacentCells = 2
	groupFloor    = 512
)

// cellStep is the sRGB width of a cell, taken as the smallest positive
// Chebyshev distance between the centres of cells consecutive in index
// order (neighbours along blue, in an image with any two).
func cellStep(cells []Cell) int {
	step := 256
	for i := 1; i < len(cells); i++ {
		if d := chebyshev(cells[i-1].Lab, cells[i].Lab); d > 0 && d < step {
			step = d
		}
	}
	return step
}

// chebyshev is the largest per-channel sRGB difference between a and b.
func chebyshev(a, b colour.OKLab) int {
	ar, ag, ab := a.ToSRGB8()
	br, bg, bb := b.ToSRGB8()
	d := func(x, y uint8) int { return int(max(x, y) - min(x, y)) }
	return max(d(ar, br), d(ag, bg), d(ab, bb))
}

// pile accumulates the mass of one explanation and its heaviest cell.
type pile struct {
	mass, heaviest, cell int
	members              []int
}

func (p *pile) add(i, w int) {
	p.members = append(p.members, i)
	p.mass += w
	if w > p.heaviest {
		p.heaviest, p.cell = w, i
	}
}

// coherent reports whether one cell holds coherentShare of the pile with
// at least coherentMin weight.
func (p pile) coherent() bool {
	return p.heaviest >= coherentMin && float64(p.heaviest) >= coherentShare*float64(p.mass)
}

// group finds the heaviest colour group in p: cells within tol of a seed,
// seeds tried heaviest first (index order on ties), each cell in one
// group. It returns the group's mass, seed and members, or 0 and -1 for
// an empty pile.
func (p pile) group(cells []Cell, tol float64) (mass, seed int, group []int) {
	members := append([]int(nil), p.members...)
	sort.SliceStable(members, func(i, j int) bool {
		if cells[members[i]].Weight != cells[members[j]].Weight {
			return cells[members[i]].Weight > cells[members[j]].Weight
		}
		return members[i] < members[j]
	})
	taken := make([]bool, len(members))
	mass, seed = 0, -1
	for i, ci := range members {
		if taken[i] {
			continue
		}
		m := 0
		var in []int
		for j, cj := range members {
			if !taken[j] && colour.Dist(cells[ci].Lab, cells[cj].Lab) <= tol {
				taken[j] = true
				m += cells[cj].Weight
				in = append(in, cj)
			}
		}
		if m > mass {
			mass, seed, group = m, ci, in
		}
	}
	return mass, seed, group
}

// wuBox is a live histogram box with the cells it holds.
type wuBox struct {
	box
	cells []int
	done  bool // within tolerance
	dead  bool // no cut leaves both halves with weight
}

// Wu builds a palette by variance-minimising box splitting over a 3-D
// histogram of OKLab, in the manner of Wu (1991). Starting from one box
// around all cells it repeatedly splits, among the boxes not yet within
// tolerance, the one with the largest sum of squared error, at the cut
// that reduces total error most. It stops when there are maxK boxes
// (MaxColors when maxK is 0), when every box is within tolerance, or when
// no remaining box can be split. The palette is the box centroids, in
// split order.
//
// A box is within tolerance when its RMS radius sqrt(SSE/weight) is below
// tol and none of its unexplained far cells is coherent (see
// coherentShare): the RMS test alone lets a large background absorb a
// small subject, since weight normalisation hides anything under
// (tol/distance)² of the pixels. The test is run when a box is next in
// line to be split, so the other boxes it is judged against exist.
func Wu(cells []Cell, maxK int, tol float64) []colour.OKLab {
	if len(cells) == 0 {
		return nil
	}
	if maxK <= 0 || maxK > MaxColors {
		maxK = MaxColors
	}
	m := newMoments(cells)
	bins := make([][3]int, len(cells))
	all := make([]int, len(cells))
	for i, c := range cells {
		bins[i] = [3]int{bin(c.Lab.L, 0, 1), bin(c.Lab.A, -wuAB, wuAB), bin(c.Lab.B, -wuAB, wuAB)}
		all[i] = i
	}
	total := 0
	for _, c := range cells {
		total += c.Weight
	}
	near := adjacentCells*cellStep(cells) + 1
	// within reports whether the box is within tolerance, and, when it is
	// but for one explained subject cell, that cell's index (else -1).
	within := func(bx box, idx []int, others []colour.OKLab) (bool, int) {
		w, sse, c := m.stats(bx)
		if w == 0 {
			return true, -1
		}
		if math.Sqrt(sse/w) >= tol {
			return false, -1
		}
		var far, tail pile
		mix := make([]pile, len(others))
	cells:
		for _, i := range idx {
			if colour.Dist(cells[i].Lab, c) <= tol {
				continue
			}
			for k, o := range others {
				if colour.MixDist(cells[i].Lab, c, o) <= tol {
					mix[k].add(i, cells[i].Weight)
					continue cells
				}
			}
			if colour.Dist(cells[i].Lab, c) <= tailReach*tol {
				tail.add(i, cells[i].Weight)
				continue
			}
			far.add(i, cells[i].Weight)
		}
		if far.coherent() {
			return false, -1
		}
		if gm, _, _ := far.group(cells, tol); gm >= coherentMin && float64(gm) >= coherentShare*float64(far.mass) && gm*groupFloor >= total {
			return false, -1
		}
		// subject reports whether the pile's heaviest cell is a flat
		// colour of its own: heavy enough, with little of the box's
		// mass in the cells around it; failing that, whether its
		// heaviest colour group is. It returns the cell to promote.
		subject := func(p pile) (int, bool) {
			if p.heaviest > 0 && p.heaviest*subjectShare >= total {
				halo := 0
				for _, j := range idx {
					if j != p.cell && chebyshev(cells[j].Lab, cells[p.cell].Lab) <= near {
						halo += cells[j].Weight
					}
				}
				if halo*haloShare < p.heaviest {
					return p.cell, true
				}
			}
			gm, seed, group := p.group(cells, tol)
			if gm == 0 || gm*subjectShare < total || gm*groupFloor < total {
				return -1, false
			}
			in := make(map[int]bool, len(group))
			for _, j := range group {
				in[j] = true
			}
			// The halo is the box's mass next to any cell of the group,
			// not just the seed: a run of ramp cells shields its seed
			// from the ramp's neighbours, a hidden object has none.
			halo := 0
			for _, j := range idx {
				if in[j] {
					continue
				}
				for _, k := range group {
					if chebyshev(cells[j].Lab, cells[k].Lab) <= near {
						halo += cells[j].Weight
						break
					}
				}
			}
			return seed, halo*haloShare < gm
		}
		if cell, ok := subject(tail); ok {
			return true, cell
		}
		for _, p := range mix {
			if cell, ok := subject(p); ok {
				return true, cell
			}
		}
		return true, -1
	}
	inside := func(bx box, b [3]int) bool {
		return b[0] >= bx.l0 && b[0] < bx.l1 && b[1] >= bx.a0 && b[1] < bx.a1 && b[2] >= bx.b0 && b[2] < bx.b1
	}
	root := box{0, wuBins, 0, wuBins, 0, wuBins}
	boxes := []wuBox{{box: root, cells: all}}
	centroids := func(skip int) []colour.OKLab {
		var out []colour.OKLab
		for i, bx := range boxes {
			if i != skip {
				_, _, c := m.stats(bx.box)
				out = append(out, c)
			}
		}
		return out
	}
	var promoted []colour.OKLab
	for len(boxes)+len(promoted) < maxK {
		best, bestSSE := -1, -1.0
		for i, bx := range boxes {
			if bx.dead || bx.done {
				continue
			}
			if _, sse, _ := m.stats(bx.box); sse > bestSSE {
				best, bestSSE = i, sse
			}
		}
		if best < 0 {
			break
		}
		if ok, subject := within(boxes[best].box, boxes[best].cells, centroids(best)); ok {
			boxes[best].done = true
			if subject >= 0 {
				promoted = append(promoted, cells[subject].Lab)
			}
			continue
		}
		lo, hi, ok := m.split(boxes[best].box)
		if !ok {
			boxes[best].dead = true
			continue
		}
		var loCells, hiCells []int
		for _, i := range boxes[best].cells {
			if inside(lo, bins[i]) {
				loCells = append(loCells, i)
			} else {
				hiCells = append(hiCells, i)
			}
		}
		boxes[best] = wuBox{box: lo, cells: loCells}
		boxes = append(boxes, wuBox{box: hi, cells: hiCells})
	}
	pal := make([]colour.OKLab, 0, len(boxes)+len(promoted))
	for _, bx := range boxes {
		if w, _, c := m.stats(bx.box); w > 0 {
			pal = append(pal, c)
		}
	}
	return append(pal, promoted...)
}
