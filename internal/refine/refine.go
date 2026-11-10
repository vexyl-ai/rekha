package refine

import (
	"math"

	"github.com/vexyl-ai/rekha/internal/fit"
	"github.com/vexyl-ai/rekha/internal/geom"
	"github.com/vexyl-ai/rekha/internal/par"
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
	moveLimit    = 2.0   // pixels a piece's points may move per optimisation (trust region)
	windowPad    = 1     // pixels of window beyond the points' reach
	skipResidual = 1e-4  // squared coverage error per pixel of boundary below which a piece is left alone
	sweepMove    = 0.01  // a sweep that moves nothing further than this ends the run
	sweepStop    = 0.005 // a sweep that lowers the energy by less than this share of its start ends the run
	// parallelPieces is the least pieces in a batch worth spreading over
	// workers: loops are refined in parallel already, and a batch of a
	// few pieces costs more to hand out than to run.
	parallelPieces = 16
	clampPad       = 1 // control points may leave the image by this much
)

// sweepHook, when set, is called at the start of every sweep (tests).
var sweepHook func()

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
	win := loopWindow(b, m, img)
	cov := make([]float32, win.W*win.H)
	Coverage(b, win, cov)
	return DataEnergy(cov, m, win, l0) + w.Prior(b, l0)
}

// Refine optimises b piece by piece (two consecutive segments, outer
// anchors fixed) against the mask m over the image img, in sweeps, and
// returns the result. A bezigon with fewer than two segments, or one whose
// optimisation produced a non-finite point, is returned unchanged.
//
// Within a sweep the pieces run in three classes, j ≡ 0, 1, 2 (mod 3):
// pieces of one class share no moving point, so each class is optimised
// in parallel from a snapshot of the loop taken when the class starts and
// written back together. Every piece reads the same snapshot whatever
// the worker count, so the result is deterministic.
func Refine(b Bezigon, m Mask, img Window, o Options) Bezigon {
	n := b.N()
	if n < 2 {
		return b
	}
	out := Bezigon{Pts: append([]geom.Vec2(nil), b.Pts...)}
	clampFree(out, img, o)
	l0 := math.Max(out.Length(), 1)
	type result struct {
		pts              []geom.Vec2
		move, start, end float64
	}
	workers := par.Workers(n)
	scratches := make([]*scratch, workers)
	for w := range scratches {
		scratches[w] = &scratch{}
	}
	var snapPoly []geom.Vec2
	for sweep := 0; sweep < o.Sweeps; sweep++ {
		if sweepHook != nil {
			sweepHook()
		}
		maxMove, start, drop := 0.0, 0.0, 0.0
		for r := 0; r < 3; r++ {
			for _, pieces := range batches(out, classPieces(n, r), img) {
				results := make([]result, len(pieces))
				copies := make([]Bezigon, workers) // one snapshot per worker
				run := par.DoW
				if len(pieces) < parallelPieces {
					run = par.Inline
				}
				// The snapshot's polyline, flattened once per batch and
				// handed to each worker's scratch: every piece reads it
				// and puts its own part back.
				snap := polylineInto(snapPoly[:0], out)
				snapPoly = snap
				run(len(pieces), func(w, i int) {
					wb := &copies[w]
					if wb.Pts == nil {
						wb.Pts = append([]geom.Vec2(nil), out.Pts...)
						sc := scratches[w]
						sc.poly = append(sc.poly[:0], snap...)
						sc.polyOK = true
					}
					j := pieces[i]
					idx := o.pieceFree(*wb, j)
					mv, st, en := refinePiece(*wb, j, m, img, l0, o, scratches[w])
					pts := make([]geom.Vec2, len(idx))
					for k, p := range idx {
						pts[k] = wb.Pts[p]
						wb.Pts[p] = out.Pts[p] // back to the snapshot for the next piece
					}
					results[i] = result{pts: pts, move: mv, start: st, end: en}
				})
				for i, j := range pieces {
					for k, p := range o.pieceFree(out, j) {
						out.Pts[p] = results[i].pts[k]
					}
					maxMove = math.Max(maxMove, results[i].move)
					start += results[i].start
					drop += results[i].start - results[i].end
				}
			}
		}
		if maxMove < sweepMove || drop < sweepStop*start {
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

// classPieces lists the pieces of residue class r (mod 3) of a loop of n
// segments. Piece j moves the handles of segments j and j+1 and the
// anchor between them, so pieces three apart share nothing, except that
// the last piece wraps round to the first: when n ≡ 1 (mod 3) piece n-1
// would sit in class 0 with piece 0, so it joins class 1 instead, where
// its nearest neighbour is two pieces away.
func classPieces(n, r int) []int {
	var out []int
	for j := r; j < n; j += 3 {
		if n%3 == 1 && n >= 4 && j == n-1 {
			continue
		}
		out = append(out, j)
	}
	if r == 1 && n%3 == 1 && n >= 4 {
		out = append(out, n-1)
	}
	return out
}

// batches splits the pieces of one class into runs that are optimised
// together: pieces whose reach boxes overlap go to different runs, so
// two curves moved at once can never cross each other (each stays inside
// its box) and the crossing test against the snapshot holds for the
// written-back result. Runs are filled greedily in piece order.
func batches(b Bezigon, pieces []int, img Window) [][]int {
	boxes := make([]bounds, len(pieces))
	for i, j := range pieces {
		boxes[i] = reach(b, 3*j, 3*j+7, img)
	}
	overlap := func(a, c bounds) bool {
		return a.lo.X <= c.hi.X && c.lo.X <= a.hi.X && a.lo.Y <= c.hi.Y && c.lo.Y <= a.hi.Y
	}
	var out [][]int
	var runBoxes [][]bounds
	for i, j := range pieces {
		placed := false
		for r := range out {
			clear := true
			for _, bx := range runBoxes[r] {
				if overlap(bx, boxes[i]) {
					clear = false
					break
				}
			}
			if clear {
				out[r] = append(out[r], j)
				runBoxes[r] = append(runBoxes[r], boxes[i])
				placed = true
				break
			}
		}
		if !placed {
			out = append(out, []int{j})
			runBoxes = append(runBoxes, []bounds{boxes[i]})
		}
	}
	return out
}

// scratch holds the per-piece buffers refinePiece reuses between calls:
// the whole outline flattened and the candidate edges of the crossing
// test.
type scratch struct {
	poly   []geom.Vec2
	polyOK bool // poly is the flattened outline of the bezigon being refined
	cand   []int
	fixed  []float32 // coverage of the segments that do not move
	mov    []float32 // coverage of the moving segments, zeroed after use
	target []float32 // mask over the spans
	weight []float32
	left   []float32 // fixed coverage carried into each row's span
}

// floats returns buf resized to n zeroed entries, reusing its storage.
func floats(buf []float32, n int) []float32 {
	if cap(buf) < n {
		return make([]float32, n)
	}
	buf = buf[:n]
	clear(buf)
	return buf
}

// refinePiece optimises piece j (segments j and j+1, up to five free
// points: the ones o leaves free) and returns how far any point moved
// and the piece's energy before and after. Each point may move moveLimit
// from where it starts, so the curve moves at most that far; the energy
// is therefore evaluated only over the pixels within reach of the
// starting curve (one span per row), where all the change is. Pixels
// outside the image are scored too: the mask reports them empty. A step
// that makes the piece's curve cross the outline more often than it did
// at the start is refused.
func refinePiece(b Bezigon, j int, m Mask, img Window, l0 float64, o Options, sc *scratch) (move, start, end float64) {
	if sc == nil {
		sc = &scratch{}
	}
	n := b.N()
	j = ((j % n) + n) % n
	idx := o.pieceFree(b, j)
	if len(idx) == 0 {
		return 0, 0, 0
	}
	r := reach(b, 3*j, 3*j+7, img)
	lo, hi := clampPoint(r.lo, img), clampPoint(r.hi, img)
	win := windowOf(lo, hi)
	if win.W <= 0 || win.H <= 0 {
		return 0, 0, 0
	}
	// Edges of the segments that do not move, accumulated once. A cubic
	// stays inside its control points' box, so one that lies above or
	// below the window's rows, or wholly to its right, deposits nothing
	// and is not flattened; one to the left still folds its winding into
	// the first cell of each row it crosses.
	fixed := floats(sc.fixed, win.W*win.H)
	sc.fixed = fixed
	var buf [Flatten + 1]geom.Vec2
	top, bottom, right := float64(win.Y0)-1, float64(win.Y0+win.H)+1, float64(win.X0+win.W)+1
	for k := 0; k < n; k++ {
		if k == j || k == (j+1)%n {
			continue
		}
		c := b.Segment(k)
		if r := cubicBox(c); r.hi.Y < top || r.lo.Y > bottom || r.lo.X > right {
			continue
		}
		pts := append(flattenInto(buf[:0], c), c[3])
		AccumulateEdges(pts, false, win, fixed)
	}
	first := make([]geom.Vec2, len(idx))
	for k, i := range idx {
		first[k] = b.Pts[i]
	}
	// Per-point trust box, clipped to the image.
	plo, phi := make([]geom.Vec2, len(idx)), make([]geom.Vec2, len(idx))
	for k := range idx {
		plo[k] = clampPoint(first[k].Sub(geom.Vec2{X: moveLimit, Y: moveLimit}), img)
		phi[k] = clampPoint(first[k].Add(geom.Vec2{X: moveLimit, Y: moveLimit}), img)
	}
	// Spans: per window row, the cells the moving curve can touch.
	moving := make([]geom.Vec2, 0, 2*Flatten+1)
	cj, cj1 := b.Segment(j), b.Segment(j+1)
	moving = append(flattenInto(flattenInto(moving[:0], cj), cj1), cj1[3])
	sp := newSpans(moving, win)
	if sp.cells == 0 {
		return 0, 0, 0
	}
	// Mask targets and weights over the spans, and the fixed coverage
	// carried into each span from the cells to its left.
	target, weight := floats(sc.target, sp.cells), floats(sc.weight, sp.cells)
	left := floats(sc.left, win.H)
	sc.target, sc.weight, sc.left = target, weight, left
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
	mov := floats(sc.mov, win.W*win.H)
	sc.mov = mov
	scale := float64(DataScale * DataScale / l0)
	// The whole outline flattened once (or taken from the scratch when
	// the caller says it already holds b's); only the piece's points are
	// rewritten per step, and only the edges whose box meets the piece's
	// reach can cross it. The crossing count at the start is the most a
	// step may leave. The piece's part of the polyline is put back on
	// return so the scratch still describes b.
	if !sc.polyOK {
		sc.poly = polylineInto(sc.poly[:0], b)
	}
	sc.cand = edgeCandidates(sc.poly, r.lo, r.hi, sc.cand[:0])
	poly, cand := sc.poly, sc.cand
	np := n * Flatten
	var orig [2 * Flatten]geom.Vec2
	for k := range orig {
		orig[k] = poly[(Flatten*j+k)%np]
	}
	defer func() {
		for k := range orig {
			poly[(Flatten*j+k)%np] = orig[k]
		}
	}()
	crossings0 := countCrossings(poly, Flatten*j, Flatten*(j+2), cand)
	// setPiece writes x into the piece's points, held inside their trust
	// boxes, and flattens the two segments into moving.
	setPiece := func(x []float64) {
		for k, i := range idx {
			b.Pts[i] = geom.Vec2{X: math.Min(math.Max(x[2*k], plo[k].X), phi[k].X), Y: math.Min(math.Max(x[2*k+1], plo[k].Y), phi[k].Y)}
		}
		cj, cj1 := b.Segment(j), b.Segment(j+1)
		moving = append(flattenInto(flattenInto(moving[:0], cj), cj1), cj1[3])
	}
	// evalParts returns the data term and the full piece energy at x.
	evalParts := func(x []float64) (data, total float64) {
		setPiece(x)
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
		// moving is the two segments flattened, so its length is their
		// LengthPrior.
		e := data + float64(w.Apt*AnglePrior(b, j, j+1, j+2)) + float64(w.Hpt*HandlePrior(b, j, j+1)) + float64(w.Opt*HandleOrderPrior(b, j, j+1)) + w.Lpt*polylineLength(moving, false)/l0
		return data, e
	}
	eval := func(x []float64) float64 {
		_, e := evalParts(x)
		return e
	}
	// ok refuses a step whose curve crosses the outline more often.
	ok := func(_, xn []float64) bool {
		setPiece(xn)
		for k := 0; k < 2*Flatten; k++ {
			poly[(Flatten*j+k)%np] = moving[k]
		}
		return countCrossings(poly, Flatten*j, Flatten*(j+2), cand) <= crossings0
	}
	x := make([]float64, 2*len(idx))
	for k, p := range first {
		x[2*k], x[2*k+1] = p.X, p.Y
	}
	// Squared coverage error on the 0..1 scale per pixel of the piece's
	// length: the window's area would dilute a boundary error.
	data, start := evalParts(x)
	resid := data * l0 / (DataScale * DataScale) / math.Max(LengthPrior(b, j, j+1), 1)
	if resid < skipResidual {
		return 0, start, start
	}
	x = minimize(x, eval, ok, o.MaxIter)
	end = eval(x) // leave b at the solution
	for k, i := range idx {
		move = math.Max(move, b.Pts[i].Sub(first[k]).Len())
	}
	return move, start, end
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

// cubicBox is the box of a cubic's control points, which holds the curve.
func cubicBox(c fit.Cubic) bounds {
	r := bounds{lo: c[0], hi: c[0]}
	for _, p := range c[1:] {
		r.lo.X, r.lo.Y = math.Min(r.lo.X, p.X), math.Min(r.lo.Y, p.Y)
		r.hi.X, r.hi.Y = math.Max(r.hi.X, p.X), math.Max(r.hi.Y, p.Y)
	}
	return r
}

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
