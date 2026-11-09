package quant

import (
	"math"

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
const (
	coherentShare = 0.25
	coherentMin   = 4
	tailReach     = 2.0
)

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
	within := func(bx box, idx []int, others []colour.OKLab) bool {
		w, sse, c := m.stats(bx)
		if w == 0 {
			return true
		}
		if math.Sqrt(sse/w) >= tol {
			return false
		}
		far, heaviest := 0, 0
	cells:
		for _, i := range idx {
			if colour.Dist(cells[i].Lab, c) <= tailReach*tol {
				continue
			}
			for _, o := range others {
				if colour.MixDist(cells[i].Lab, c, o) <= tol {
					continue cells
				}
			}
			far += cells[i].Weight
			heaviest = max(heaviest, cells[i].Weight)
		}
		return heaviest < coherentMin || float64(heaviest) < coherentShare*float64(far)
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
	for len(boxes) < maxK {
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
		if within(boxes[best].box, boxes[best].cells, centroids(best)) {
			boxes[best].done = true
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
	pal := make([]colour.OKLab, 0, len(boxes))
	for _, bx := range boxes {
		if w, _, c := m.stats(bx.box); w > 0 {
			pal = append(pal, c)
		}
	}
	return pal
}
