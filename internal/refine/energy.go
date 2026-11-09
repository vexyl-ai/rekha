package refine

import (
	"math"

	"github.com/vexyl-ai/rekha/internal/geom"
)

// Mask is the per-pixel target a bezigon is refined against: the coverage
// the curve should have at (x, y), in [0, 1], and a weight (0 = don't
// care). Pixels outside the image have coverage 0 and weight 1.
type Mask interface {
	At(x, y int) (cov, weight float32)
}

// Weights scale the four priors of Yang et al. 2016 (eq. 11).
type Weights struct {
	Spt float64 // self-intersection: shorter arc of every crossing
	Apt float64 // angle variation: absolute joint angles (L1 corner sparsity)
	Hpt float64 // short handles: inverse barrier on handle lengths
	Lpt float64 // curve length
}

// DefaultWeights are the paper's defaults (§5.1).
var DefaultWeights = Weights{Spt: 1.0, Apt: 0.08, Hpt: 0.1, Lpt: 0.1}

// minHandle floors handle lengths in the barrier so a collapsed handle
// gives a huge but finite energy.
const minHandle = 1e-6

// DataEnergy is Σ weight·(cov − mask)² over the window divided by l0, the
// initial arc length, so prior weights mean the same for every shape size.
func DataEnergy(cov []float32, m Mask, win Window, l0 float64) float64 {
	e := 0.0
	for y := 0; y < win.H; y++ {
		for x := 0; x < win.W; x++ {
			target, w := m.At(win.X0+x, win.Y0+y)
			d := float64(cov[y*win.W+x] - target)
			e += float64(w) * d * d
		}
	}
	return e / math.Max(l0, 1)
}

// AnglePrior sums the absolute angle between the incoming and outgoing
// handles at each joint (eq. 14); all joints when none are given. Joint j
// is the start anchor of segment j.
func AnglePrior(b Bezigon, joints ...int) float64 {
	n := b.N()
	if n == 0 {
		return 0
	}
	if len(joints) == 0 {
		joints = all(n)
	}
	e := 0.0
	for _, j := range joints {
		j = ((j % n) + n) % n
		m := len(b.Pts)
		anchor := b.Pts[3*j]
		in := anchor.Sub(b.Pts[((3*j-1)%m+m)%m])
		out := b.Pts[(3*j+1)%m].Sub(anchor)
		e += angleBetween(in, out)
	}
	return e
}

func angleBetween(a, b geom.Vec2) float64 {
	la, lb := a.Len(), b.Len()
	if la == 0 || lb == 0 {
		return 0
	}
	return math.Acos(min(max(a.Dot(b)/(la*lb), -1), 1))
}

// HandlePrior is the inverse barrier Σ 1/‖handle‖ over both handles of
// each segment (eq. 15); all segments when none are given.
func HandlePrior(b Bezigon, segs ...int) float64 {
	n := b.N()
	if n == 0 {
		return 0
	}
	if len(segs) == 0 {
		segs = all(n)
	}
	e := 0.0
	for _, j := range segs {
		c := b.Segment(j)
		e += 1/math.Max(c[1].Sub(c[0]).Len(), minHandle) + 1/math.Max(c[3].Sub(c[2]).Len(), minHandle)
	}
	return e
}

// LengthPrior is the flattened arc length of the given segments (eq. 16);
// the whole bezigon when none are given.
func LengthPrior(b Bezigon, segs ...int) float64 {
	n := b.N()
	if n == 0 {
		return 0
	}
	if len(segs) == 0 {
		return b.Length()
	}
	e := 0.0
	var buf [Flatten + 1]geom.Vec2
	for _, j := range segs {
		c := b.Segment(j)
		pts := flattenInto(buf[:0], c)
		pts = append(pts, c[3])
		e += polylineLength(pts, false)
	}
	return e
}

// SelfIntersectionPrior sums, for every crossing of the flattened outline
// with itself, the shorter of the two arcs the crossing cuts (eq. 12–13).
// Zero for a simple outline; grows with the size of a loop so the
// optimiser can shrink it away.
func SelfIntersectionPrior(b Bezigon) float64 {
	pts := b.Polyline()
	n := len(pts)
	if n < 4 {
		return 0
	}
	// Cumulative length at each vertex, closed.
	cum := make([]float64, n+1)
	for i := 1; i <= n; i++ {
		cum[i] = cum[i-1] + pts[i%n].Sub(pts[i-1]).Len()
	}
	total := cum[n]
	e := 0.0
	for i := 0; i < n; i++ {
		a0, a1 := pts[i], pts[(i+1)%n]
		for k := i + 2; k < n; k++ {
			if i == 0 && k == n-1 {
				continue // adjacent through the closing edge
			}
			b0, b1 := pts[k], pts[(k+1)%n]
			if !boxesOverlap(a0, a1, b0, b1) {
				continue
			}
			ta, tb, ok := segmentCross(a0, a1, b0, b1)
			if !ok {
				continue
			}
			sa := cum[i] + ta*(cum[i+1]-cum[i])
			sb := cum[k] + tb*(cum[k+1]-cum[k])
			arc := sb - sa
			e += math.Min(arc, total-arc)
		}
	}
	return e
}

func boxesOverlap(a0, a1, b0, b1 geom.Vec2) bool {
	return math.Min(a0.X, a1.X) <= math.Max(b0.X, b1.X) && math.Min(b0.X, b1.X) <= math.Max(a0.X, a1.X) &&
		math.Min(a0.Y, a1.Y) <= math.Max(b0.Y, b1.Y) && math.Min(b0.Y, b1.Y) <= math.Max(a0.Y, a1.Y)
}

// segmentCross reports whether segments a and b cross (half-open in each
// parameter, so a crossing at a vertex is counted once) and the
// parameters along each where they do.
func segmentCross(a0, a1, b0, b1 geom.Vec2) (ta, tb float64, ok bool) {
	da, db := a1.Sub(a0), b1.Sub(b0)
	den := geom.Cross(da, db)
	if den == 0 {
		return 0, 0, false
	}
	w := b0.Sub(a0)
	ta = geom.Cross(w, db) / den
	tb = geom.Cross(w, da) / den
	if ta < 0 || ta >= 1 || tb < 0 || tb >= 1 { // half-open: a crossing at a vertex counts once
		return 0, 0, false
	}
	return ta, tb, true
}

// Prior is the weighted sum of the four priors over the whole bezigon.
func (w Weights) Prior(b Bezigon) float64 {
	return w.Spt*SelfIntersectionPrior(b) + w.Apt*AnglePrior(b) + w.Hpt*HandlePrior(b) + w.Lpt*LengthPrior(b)
}

func all(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}
