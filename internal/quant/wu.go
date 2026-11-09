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

// Wu builds a palette by variance-minimising box splitting over a 3-D
// histogram of OKLab, in the manner of Wu (1991). Starting from one box
// around all cells it repeatedly splits the box with the largest sum of
// squared error, at the cut that reduces total error most, and stops when
// there are maxK boxes (MaxColors when maxK is 0), when the largest box's
// RMS radius sqrt(SSE/weight) is below tol, or when no box can be split.
// The palette is the box centroids, in split order.
func Wu(cells []Cell, maxK int, tol float64) []colour.OKLab {
	if len(cells) == 0 {
		return nil
	}
	if maxK <= 0 || maxK > MaxColors {
		maxK = MaxColors
	}
	m := newMoments(cells)
	boxes := []box{{0, wuBins, 0, wuBins, 0, wuBins}}
	dead := []bool{false}
	for len(boxes) < maxK {
		best, bestSSE := -1, -1.0
		for i, bx := range boxes {
			if dead[i] {
				continue
			}
			if _, sse, _ := m.stats(bx); sse > bestSSE {
				best, bestSSE = i, sse
			}
		}
		if best < 0 {
			break
		}
		if w, _, _ := m.stats(boxes[best]); math.Sqrt(bestSSE/w) < tol {
			break
		}
		lo, hi, ok := m.split(boxes[best])
		if !ok {
			dead[best] = true
			continue
		}
		boxes[best] = lo
		boxes = append(boxes, hi)
		dead = append(dead, false)
	}
	pal := make([]colour.OKLab, 0, len(boxes))
	for _, bx := range boxes {
		if w, _, c := m.stats(bx); w > 0 {
			pal = append(pal, c)
		}
	}
	return pal
}
