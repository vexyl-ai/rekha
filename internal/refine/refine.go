package refine

import (
	"math"

	"github.com/vexyl-ai/rekha/internal/geom"
)

// Options control the piecewise optimisation.
type Options struct {
	Sweeps  int     // passes over all pieces
	MaxIter int     // L-BFGS iterations per piece per sweep
	Weights Weights // prior weights
	// Fixed marks anchors (per segment start) that never move and whose
	// joints are never merged; Frozen marks segments whose control points
	// never move and that are never split or merged. Either may be nil
	// or shorter than the bezigon, in which case the rest is free.
	Fixed  []bool
	Frozen []bool
}

// fixed reports whether anchor a is fixed, frozen whether segment s is.
func (o Options) fixed(a int) bool  { return a < len(o.Fixed) && o.Fixed[a] }
func (o Options) frozen(s int) bool { return s < len(o.Frozen) && o.Frozen[s] }

// freePoint reports whether point i of a bezigon with n segments may
// move: an anchor that is not fixed and bounds no frozen segment, or a
// handle of a segment that is not frozen.
func (o Options) freePoint(i, n int) bool {
	a := i / 3
	if i%3 == 0 {
		return !o.fixed(a) && !o.frozen(a) && !o.frozen((a+n-1)%n)
	}
	return !o.frozen(a)
}

// pieceFree returns the indices of the points of piece j (segments j and
// j+1) that may move, in point order.
func (o Options) pieceFree(b Bezigon, j int) []int {
	n, m := b.N(), len(b.Pts)
	idx := make([]int, 0, 5)
	for k := 1; k <= 5; k++ {
		if i := (3*j + k) % m; o.freePoint(i, n) {
			idx = append(idx, i)
		}
	}
	return idx
}

// withoutJoint returns the options after joint j and segment j are
// merged away: entries j of Fixed and Frozen are removed.
func (o Options) withoutJoint(j int) Options {
	o.Fixed, o.Frozen = removeAt(o.Fixed, j), removeAt(o.Frozen, j)
	return o
}

// withSplit returns the options after segment j is split in two: a free
// joint and a free segment are inserted at j+1.
func (o Options) withSplit(j int) Options {
	o.Fixed, o.Frozen = insertAt(o.Fixed, j+1), insertAt(o.Frozen, j+1)
	return o
}

func removeAt(v []bool, i int) []bool {
	if i >= len(v) {
		return v
	}
	out := make([]bool, 0, len(v)-1)
	out = append(out, v[:i]...)
	return append(out, v[i+1:]...)
}

func insertAt(v []bool, i int) []bool {
	if i > len(v) {
		return v
	}
	out := make([]bool, 0, len(v)+1)
	out = append(out, v[:i]...)
	out = append(out, false)
	return append(out, v[i:]...)
}

// DefaultOptions are the paper's: three sweeps, thirty iterations.
var DefaultOptions = Options{Sweeps: 3, MaxIter: 30, Weights: DefaultWeights}

const (
	moveLimit    = 2.0  // pixels a piece's points may move per optimisation (trust region)
	windowPad    = 1    // pixels of window beyond the points' reach
	skipResidual = 1e-4 // squared coverage error per pixel of boundary below which a piece is left alone
	sweepMove    = 0.01 // a sweep that moves nothing further than this ends the run
	clampPad     = 1    // control points may leave the image by this much
)

// Energy is the full energy of b against m over the image window.
func Energy(b Bezigon, m Mask, img Window, w Weights) float64 {
	return energyWith(b, m, img, w, math.Max(b.Length(), 1))
}

// energyWith is Energy with the data term normalised by l0, the length
// Refine fixes at its start, so energies before and after a run compare.
func energyWith(b Bezigon, m Mask, img Window, w Weights, l0 float64) float64 {
	if b.N() == 0 {
		return 0
	}
	cov := make([]float32, img.W*img.H)
	Coverage(b, img, cov)
	return DataEnergy(cov, m, img, l0) + w.Prior(b, l0)
}

// Refine optimises b piece by piece (two consecutive segments, outer
// anchors fixed) against the mask m over the image img, in sweeps, and
// returns the result. A bezigon with fewer than two segments, or one whose
// optimisation produced a non-finite point, is returned unchanged.
func Refine(b Bezigon, m Mask, img Window, o Options) Bezigon {
	n := b.N()
	if n < 2 {
		return b
	}
	out := Bezigon{Pts: append([]geom.Vec2(nil), b.Pts...)}
	clampFree(out, img, o)
	l0 := math.Max(out.Length(), 1)
	sc := &scratch{}
	for sweep := 0; sweep < o.Sweeps; sweep++ {
		maxMove := 0.0
		for j := 0; j < n; j++ {
			maxMove = math.Max(maxMove, refinePiece(out, j, m, img, l0, o, sc))
		}
		if maxMove < sweepMove {
			break
		}
	}
	for _, p := range out.Pts {
		if !finite(p.X) || !finite(p.Y) {
			return b
		}
	}
	return out
}

// refinePiece optimises piece j (segments j and j+1, up to five free
// points: the ones o leaves free) and returns how far any point moved. Each point may move moveLimit from
// where it starts, so the curve moves at most that far; the energy is
// therefore evaluated only over the pixels within reach of the starting
// curve (one span per row), where all the change is. Pixels outside the
// image are scored too: the mask reports them empty.
// scratch holds the per-piece buffers refinePiece reuses between calls:
// the whole outline flattened, its arc-length table and the candidate
// edges of the self-intersection prior.
type scratch struct {
	poly []geom.Vec2
	cum  []float64
	cand []int
}

func refinePiece(b Bezigon, j int, m Mask, img Window, l0 float64, o Options, sc *scratch) float64 {
	if sc == nil {
		sc = &scratch{}
	}
	n := b.N()
	j = ((j % n) + n) % n
	idx := o.pieceFree(b, j)
	if len(idx) == 0 {
		return 0
	}
	r := reach(b, 3*j, 3*j+7, img)
	lo, hi := clampPoint(r.lo, img), clampPoint(r.hi, img)
	win := windowOf(lo, hi)
	if win.W <= 0 || win.H <= 0 {
		return 0
	}
	// Edges of the segments that do not move, accumulated once.
	fixed := make([]float32, win.W*win.H)
	var buf [Flatten + 1]geom.Vec2
	for k := 0; k < n; k++ {
		if k == j || k == (j+1)%n {
			continue
		}
		c := b.Segment(k)
		pts := append(flattenInto(buf[:0], c), c[3])
		AccumulateEdges(pts, false, win, fixed)
	}
	start := make([]geom.Vec2, len(idx))
	for k, i := range idx {
		start[k] = b.Pts[i]
	}
	// Per-point trust box, clipped to the image.
	plo, phi := make([]geom.Vec2, len(idx)), make([]geom.Vec2, len(idx))
	for k := range idx {
		plo[k] = clampPoint(start[k].Sub(geom.Vec2{X: moveLimit, Y: moveLimit}), img)
		phi[k] = clampPoint(start[k].Add(geom.Vec2{X: moveLimit, Y: moveLimit}), img)
	}
	// Spans: per window row, the cells the moving curve can touch.
	moving := make([]geom.Vec2, 0, 2*Flatten+1)
	cj, cj1 := b.Segment(j), b.Segment(j+1)
	moving = append(flattenInto(flattenInto(moving[:0], cj), cj1), cj1[3])
	sp := newSpans(moving, win)
	if sp.cells == 0 {
		return 0
	}
	// Mask targets and weights over the spans, and the fixed coverage
	// carried into each span from the cells to its left.
	target, weight := make([]float32, sp.cells), make([]float32, sp.cells)
	left := make([]float32, win.H)
	c := 0
	for y := 0; y < win.H; y++ {
		x0, x1 := sp.x0[y], sp.x1[y]
		if x0 >= x1 {
			continue
		}
		var v float32
		for x := 0; x < x0; x++ {
			v += fixed[y*win.W+x]
		}
		left[y] = v
		for x := x0; x < x1; x++ {
			target[c], weight[c] = m.At(win.X0+x, win.Y0+y)
			c++
		}
	}
	mov := make([]float32, win.W*win.H)
	scale := float64(DataScale * DataScale / l0)
	// The whole outline flattened once; only the piece's points are
	// rewritten per evaluation, and only the edges whose box meets the
	// piece's reach can cross it.
	// Its arc lengths are those at the piece's start: the loop-size
	// penalty is a barrier, not a fit, and the moving edges change the
	// table by at most the piece's own length.
	if o.Weights.Spt != 0 {
		sc.poly = polylineInto(sc.poly[:0], b)
		sc.cum = arcLengths(sc.poly, sc.cum)
		sc.cand = edgeCandidates(sc.poly, r.lo, r.hi, sc.cand[:0])
	}
	poly, cand, cum := sc.poly, sc.cand, sc.cum
	np := n * Flatten
	// evalParts returns the data term and the full piece energy at x, with
	// the points held inside their trust boxes.
	evalParts := func(x []float64) (data, total float64) {
		for k, i := range idx {
			b.Pts[i] = geom.Vec2{X: math.Min(math.Max(x[2*k], plo[k].X), phi[k].X), Y: math.Min(math.Max(x[2*k+1], plo[k].Y), phi[k].Y)}
		}
		cj, cj1 := b.Segment(j), b.Segment(j+1)
		moving = append(flattenInto(flattenInto(moving[:0], cj), cj1), cj1[3])
		AccumulateEdges(moving, false, win, mov)
		c := 0
		for y := 0; y < win.H; y++ {
			x0, x1 := sp.x0[y], sp.x1[y]
			if x0 >= x1 {
				continue
			}
			v := left[y]
			row := y * win.W
			for x := x0; x < x1; x++ {
				v += fixed[row+x] + mov[row+x]
				mov[row+x] = 0
				cov := math.Abs(float64(v))
				if cov > 1 {
					cov = 1
				}
				d := cov - float64(target[c])
				data += float64(float64(weight[c]) * d * d)
				c++
			}
		}
		data *= scale
		w := o.Weights
		e := data + float64(w.Apt*AnglePrior(b, j, j+1, j+2)) + float64(w.Hpt*HandlePrior(b, j, j+1)) + float64(w.Opt*HandleOrderPrior(b, j, j+1)) + w.Lpt*LengthPrior(b, j, j+1)/l0
		if w.Spt != 0 {
			for k := 0; k < 2*Flatten; k++ {
				poly[(Flatten*j+k)%np] = moving[k]
			}
			e += float64(w.Spt * selfIntersection(poly, Flatten*j, Flatten*(j+2), cand, cum))
		}
		return data, e
	}
	eval := func(x []float64) float64 {
		_, e := evalParts(x)
		return e
	}
	x := make([]float64, 2*len(idx))
	for k, p := range start {
		x[2*k], x[2*k+1] = p.X, p.Y
	}
	// Squared coverage error on the 0..1 scale per pixel of the piece's
	// length: the window's area would dilute a boundary error.
	data, _ := evalParts(x)
	resid := data * l0 / (DataScale * DataScale) / math.Max(LengthPrior(b, j, j+1), 1)
	if resid < skipResidual {
		return 0
	}
	x = minimize(x, eval, nil, o.MaxIter)
	eval(x) // leave b at the solution
	move := 0.0
	for k, i := range idx {
		move = math.Max(move, b.Pts[i].Sub(start[k]).Len())
	}
	return move
}

// spans is, per window row, the half-open cell range [x0, x1) a piece's
// curve can touch: the starting polyline's extent in that row grown by
// moveLimit plus the cell the rasteriser spills into.
type spans struct {
	x0, x1 []int
	cells  int
}

func newSpans(pts []geom.Vec2, win Window) spans {
	sp := spans{x0: make([]int, win.H), x1: make([]int, win.H)}
	for y := range sp.x0 {
		sp.x0[y] = win.W
	}
	const pad = moveLimit + 2
	for i := 0; i+1 < len(pts); i++ {
		a, b := pts[i], pts[i+1]
		y0 := int(math.Floor(math.Min(a.Y, b.Y)-pad)) - win.Y0
		y1 := int(math.Ceil(math.Max(a.Y, b.Y)+pad)) - win.Y0
		x0 := int(math.Floor(math.Min(a.X, b.X)-pad)) - win.X0
		x1 := int(math.Ceil(math.Max(a.X, b.X)+pad)) - win.X0
		for y := max(y0, 0); y < min(y1, win.H); y++ {
			sp.x0[y] = min(sp.x0[y], max(x0, 0))
			sp.x1[y] = max(sp.x1[y], min(x1, win.W))
		}
	}
	for y := range sp.x0 {
		if sp.x0[y] < sp.x1[y] {
			sp.cells += sp.x1[y] - sp.x0[y]
		}
	}
	return sp
}

// clampFree moves the free points of b to within clampPad of the image.
func clampFree(b Bezigon, img Window, o Options) {
	n := b.N()
	for i := range b.Pts {
		if o.freePoint(i, n) {
			b.Pts[i] = clampPoint(b.Pts[i], img)
		}
	}
}

func clampPoint(p geom.Vec2, img Window) geom.Vec2 {
	return geom.Vec2{
		X: math.Min(math.Max(p.X, float64(img.X0)-clampPad), float64(img.X0+img.W)+clampPad),
		Y: math.Min(math.Max(p.Y, float64(img.Y0)-clampPad), float64(img.Y0+img.H)+clampPad),
	}
}

// bounds is a point rectangle.
type bounds struct{ lo, hi geom.Vec2 }

// reach is the bounding box of points [from, to) (cyclic indices) grown by
// moveLimit: where those points may end up.
func reach(b Bezigon, from, to int, img Window) bounds {
	m := len(b.Pts)
	r := bounds{lo: geom.Vec2{X: math.Inf(1), Y: math.Inf(1)}, hi: geom.Vec2{X: math.Inf(-1), Y: math.Inf(-1)}}
	for k := from; k < to; k++ {
		p := b.Pts[(k%m+m)%m]
		r.lo.X, r.lo.Y = math.Min(r.lo.X, p.X), math.Min(r.lo.Y, p.Y)
		r.hi.X, r.hi.Y = math.Max(r.hi.X, p.X), math.Max(r.hi.Y, p.Y)
	}
	r.lo = r.lo.Sub(geom.Vec2{X: moveLimit, Y: moveLimit})
	r.hi = r.hi.Add(geom.Vec2{X: moveLimit, Y: moveLimit})
	return r
}

// windowOf is the pixel window covering the rectangle lo..hi plus windowPad.
func windowOf(lo, hi geom.Vec2) Window {
	win := Window{X0: int(math.Floor(lo.X)) - windowPad, Y0: int(math.Floor(lo.Y)) - windowPad}
	win.W = int(math.Ceil(hi.X)) + windowPad - win.X0
	win.H = int(math.Ceil(hi.Y)) + windowPad - win.Y0
	return win
}
