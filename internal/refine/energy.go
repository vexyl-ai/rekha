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
	Apt float64 // angle variation: absolute joint angles (L1 corner sparsity)
	Hpt float64 // short handles: inverse barrier on handle lengths
	Lpt float64 // curve length
	Opt float64 // handle order: handles projected onto the chord must lie within it, in order
}

// DefaultWeights are the paper's defaults (§5.1) except Hpt, raised from
// 0.1 to 0.5: a piece cannot move its outer anchors, and with the paper's
// weight the data term pays the optimiser to hook the adjacent handle onto
// a displaced anchor (a sub-pixel reversed handle, a cusp the raster
// barely sees) rather than wait for the anchor's own piece; the cusp then
// sits behind the 1/length barrier for good. At 0.5 the hook costs more
// than it saves, and square corners survive refinement within 0.05 px.
// Opt (not in the paper) is 0.1: on the goal images it cuts the total
// handle-order violation from 107 px to 2 px at the same PSNR, where 1.0
// strands the gross-loop fixture.
var DefaultWeights = Weights{Apt: 0.08, Hpt: 0.5, Lpt: 0.1, Opt: 0.1}

// minHandle floors handle lengths in the barrier so a collapsed handle
// gives a huge but finite energy.
const minHandle = 1e-6

// DataScale multiplies coverage differences in the data term. It sets the
// balance against the paper's prior weights, whose intensity scale the
// paper does not state: with coverage in 0..1 and the length prior at 0.1
// per pixel, a closed curve settles about 0.3/DataScale² px inside its
// true edge (0.005 px at 8), while a larger scale lets the data term
// outbid the handle barrier and hook handles onto displaced anchors. 8 is
// the measured balance: perturbed smooth shapes recover within 0.05 px
// and square corners hold.
const DataScale = 8

// Boxed is a Mask that is zero outside a window: energies over the whole
// image can then be taken over the loop's box joined with that window.
type Boxed interface {
	Mask
	Box() Window
}

// loopWindow is the part of img where b's coverage or m can be nonzero:
// the control points' box padded by a pixel, joined with the mask's box
// when it has one; otherwise img. Energies over it equal those over img.
func loopWindow(b Bezigon, m Mask, img Window) Window {
	bm, ok := m.(Boxed)
	if !ok {
		return img
	}
	r := segmentsBox(b, 0, b.N()-1)
	x0, y0 := int(math.Floor(r.lo.X))-1, int(math.Floor(r.lo.Y))-1
	x1, y1 := int(math.Ceil(r.hi.X))+1, int(math.Ceil(r.hi.Y))+1
	mb := bm.Box()
	x0, y0 = min(x0, mb.X0), min(y0, mb.Y0)
	x1, y1 = max(x1, mb.X0+mb.W), max(y1, mb.Y0+mb.H)
	x0, y0 = max(x0, img.X0), max(y0, img.Y0)
	x1, y1 = min(x1, img.X0+img.W), min(y1, img.Y0+img.H)
	return Window{X0: x0, Y0: y0, W: max(x1-x0, 0), H: max(y1-y0, 0)}
}

// DataEnergy is Σ weight·(DataScale·(cov − mask))² over the window divided
// by l0, the initial arc length, so prior weights mean the same for every
// shape size.
func DataEnergy(cov []float32, m Mask, win Window, l0 float64) float64 {
	e := 0.0
	for y := 0; y < win.H; y++ {
		for x := 0; x < win.W; x++ {
			target, w := m.At(win.X0+x, win.Y0+y)
			d := DataScale * float64(cov[y*win.W+x]-target)
			e += float64(float64(w) * d * d)
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

// handleFloor is the fraction of a segment's chord beyond which the
// short-handle barrier stops rewarding a longer handle (the barrier is
// shifted so it reaches zero there): the barrier is there to keep
// handles from collapsing, not to grow them, and left unbounded it
// swings the handles of a short segment out sideways.
const handleFloor = 1.0 / 3

// HandlePrior is the inverse barrier Σ 1/‖handle‖ over both handles of
// each segment (eq. 15), zero from handleFloor of the chord outwards;
// all segments when none are given.
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
		floor := math.Max(handleFloor*c[3].Sub(c[0]).Len(), minHandle)
		e += math.Max(0, 1/math.Max(c[1].Sub(c[0]).Len(), minHandle)-1/floor)
		e += math.Max(0, 1/math.Max(c[3].Sub(c[2]).Len(), minHandle)-1/floor)
	}
	return e
}

// HandleOrderPrior is the sum, over the segments (all when none are
// given), of how far in pixels each handle's projection onto the chord
// falls outside it or past the other handle's: zero for a cubic whose
// handles run forward along the chord in order, positive for crossed,
// reversed or overshooting handles, which knot the curve. Linear, like
// the self-intersection prior: a squared penalty's gradient on a grossly
// knotted start overpowers the data and strands it bent. A segment with
// a chord shorter than minHandle is left alone.
func HandleOrderPrior(b Bezigon, segs ...int) float64 {
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
		chord := c[3].Sub(c[0])
		l := chord.Len()
		if l < minHandle {
			continue
		}
		u1, u2 := c[1].Sub(c[0]).Dot(chord)/l, c[2].Sub(c[0]).Dot(chord)/l
		v := math.Max(0, -u1) + math.Max(0, u1-l) + math.Max(0, -u2) + math.Max(0, u2-l) + math.Max(0, u1-u2)
		e += v
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
	return selfIntersection(pts, 0, len(pts), nil, nil)
}

// arcLengths fills buf (grown as needed) with the arc length from pts[0]
// to each vertex of the closed polyline, the last entry being its total.
func arcLengths(pts []geom.Vec2, buf []float64) []float64 {
	n := len(pts)
	if cap(buf) < n+1 {
		buf = make([]float64, n+1)
	}
	buf = buf[:n+1]
	buf[0] = 0
	for j := 1; j <= n; j++ {
		buf[j] = buf[j-1] + pts[j%n].Sub(pts[j-1]).Len()
	}
	return buf
}

// edgeCandidates lists, appended to cand[:0], the edges of the closed
// polyline pts whose bounding box meets the box lo..hi: the only edges a
// curve confined to that box can cross.
func edgeCandidates(pts []geom.Vec2, lo, hi geom.Vec2, cand []int) []int {
	n := len(pts)
	cand = cand[:0]
	for k := 0; k < n; k++ {
		a, b := pts[k], pts[(k+1)%n]
		if max(a.X, b.X) < lo.X || min(a.X, b.X) > hi.X || max(a.Y, b.Y) < lo.Y || min(a.Y, b.Y) > hi.Y {
			continue
		}
		cand = append(cand, k)
	}
	return cand
}

// selfIntersection sums the shorter arcs of the crossings of the closed
// polyline pts in which at least one edge has index in [lo, hi) (edge i
// runs from pts[i] to pts[i+1]); the full range gives the prior, a piece's
// range gives the part a piece can change. cand, if not nil, lists the
// only edges tested against the range (see edgeCandidates); cum, if not
// nil, is the polyline's arcLengths table.
func selfIntersection(pts []geom.Vec2, lo, hi int, cand []int, cum []float64) float64 {
	n := len(pts)
	e := 0.0
	forCrossings(pts, lo, hi, cand, func(i, k int, ta, tb float64) {
		if cum == nil {
			cum = arcLengths(pts, nil)
		}
		sa := cum[i] + float64(ta*(cum[i+1]-cum[i]))
		sb := cum[k] + float64(tb*(cum[k+1]-cum[k]))
		arc := math.Abs(sb - sa)
		e += math.Min(arc, cum[n]-arc)
	})
	return e
}

// CrossingCount is the number of crossings of the flattened outline with
// itself: zero for a simple outline.
func CrossingCount(b Bezigon) int {
	pts := b.Polyline()
	return countCrossings(pts, 0, len(pts), nil)
}

// countCrossings counts the crossings selfIntersection would sum.
func countCrossings(pts []geom.Vec2, lo, hi int, cand []int) int {
	c := 0
	forCrossings(pts, lo, hi, cand, func(int, int, float64, float64) { c++ })
	return c
}

// forCrossings calls fn(i, k, ta, tb) for every crossing of the closed
// polyline pts between an edge i in the range [lo, hi) and an edge k (a
// pair with both edges in the range is reported once, from the earlier
// edge). Every other edge is first tested against the range's bounding
// box, so a piece costs O(n) plus the few edges that come near it.
func forCrossings(pts []geom.Vec2, lo, hi int, cand []int, fn func(i, k int, ta, tb float64)) {
	n := len(pts)
	if n < 4 {
		return
	}
	m := min(hi-lo, n) // edges in the range
	lo = ((lo % n) + n) % n
	// Bounding box of the range's edges.
	bx0, by0, bx1, by1 := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for r := 0; r <= m; r++ {
		p := pts[(lo+r)%n]
		bx0, by0 = min(bx0, p.X), min(by0, p.Y)
		bx1, by1 = max(bx1, p.X), max(by1, p.Y)
	}
	for c := 0; c < n; c++ {
		k := c
		if cand != nil {
			if c >= len(cand) {
				break
			}
			k = cand[c]
		}
		b0, b1 := pts[k], pts[(k+1)%n]
		if max(b0.X, b1.X) < bx0 || min(b0.X, b1.X) > bx1 || max(b0.Y, b1.Y) < by0 || min(b0.Y, b1.Y) > by1 {
			continue
		}
		rk := ((k-lo)%n + n) % n // k's rank in the range, or ≥ m
		for r := 0; r < m; r++ {
			i := (lo + r) % n
			if k == i || k == (i+1)%n || (k+1)%n == i {
				continue // the edge itself or an adjacent one
			}
			if rk < m && rk < r {
				continue
			}
			a0, a1 := pts[i], pts[(i+1)%n]
			if max(a0.X, a1.X) < min(b0.X, b1.X) || max(b0.X, b1.X) < min(a0.X, a1.X) ||
				max(a0.Y, a1.Y) < min(b0.Y, b1.Y) || max(b0.Y, b1.Y) < min(a0.Y, a1.Y) {
				continue
			}
			if ta, tb, ok := segmentCross(a0, a1, b0, b1); ok {
				fn(i, k, ta, tb)
			}
		}
	}
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

// Prior is the weighted sum of the four priors over the whole bezigon,
// the length prior relative to l0, the initial length (see PriorAt).
func (w Weights) Prior(b Bezigon, l0 float64) float64 {
	return float64(w.Apt*AnglePrior(b)) + float64(w.Hpt*HandlePrior(b)) + float64(w.Opt*HandleOrderPrior(b)) + w.Lpt*LengthPrior(b)/math.Max(l0, 1)
}

// The length prior is measured relative to the initial length l0, as the
// data term is, so it only breaks ties. Taken raw, as the paper writes
// it, it grows with the perimeter while the normalised data term does
// not, and on a long thin shape retracting an end by a pixel saves more
// length than the lost pixel costs: one-pixel lines shrink sweep by
// sweep. A fat shape never shows this, since shrinking it costs data
// along its whole boundary.

func all(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}
