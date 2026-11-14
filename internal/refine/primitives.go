package refine

import (
	"math"

	"github.com/vexyl-ai/rekha/internal/fit"
	"github.com/vexyl-ai/rekha/internal/geom"
)

const (
	// primRise is the relative rise in data energy a primitive snap may
	// cost over the window it changes; beyond it the snap is reverted
	// unless the absolute floor primAbs (guard.go) admits it.
	// Candidates are judged by fineDataEnergy, what they render, not by
	// the Flatten-chord polygon the optimiser moved: that polygon sits
	// r·(π/32)²/12 (0.02 px at r = 24) inside its curve, which the
	// refinement paid for by inflating the curve, so judged as polygons
	// a true arc always looks worse than the inflated fit it replaces.
	primRise = 0.02
	// radiusSearch is the relative range either side of the Kåsa radius
	// over which an arc's radius is refitted against the mask, and
	// radiusSteps the golden-section steps spent on it (1e-3 of the
	// range).
	radiusSearch = 0.1
	radiusSteps  = 16
	// rectAngleTol is how far, in degrees, every turn of a four-sided
	// loop may be from 90° for it to be a rectangle.
	rectAngleTol = 2.0
	// arcJointTurn is the turn, in degrees, at a joint between two curved
	// segments beyond which the joint ends an arc run.
	arcJointTurn = 5.0
	// radiusAgree is the spread, as a fraction of their mean, within which
	// the arcs of one loop are refitted to one radius.
	radiusAgree = 0.10
	// rectAxisSnap is the orientation, in degrees, within which a
	// rectangle is taken as axis-aligned.
	rectAxisSnap = 1.0
	// pieceSweep is the most an arc piece sweeps: a right angle with a
	// little slack, so a corner refined to 91° stays one cubic (the
	// cubic's radial error at 95° is still under 0.001·r).
	pieceSweep = math.Pi / 2 * 1.06
)

// chordFactor is the ratio of the radius of the circle a mask shows to
// the radius of the outline the refinement matched to it: the energy
// flattens every cubic into Flatten chords, so the refined outline is a
// polygon with chords of angle theta, whose area is sin(θ)/θ of its
// circle's; the circle with the polygon's area has the radius the mask
// shows (0.08 % smaller for 90° pieces: 0.08 px at r = 96). A Kåsa
// radius measured on the refined outline is corrected by it before the
// candidate is judged.
func chordFactor(theta float64) float64 {
	if theta <= 0 {
		return 1
	}
	return math.Sqrt(math.Sin(theta) / theta)
}

// SnapPieces rebuilds the loop piece by piece. loopPieces cuts it into
// line, arc and free pieces; every arc piece, later pieces first so
// earlier indices stay valid, becomes circular-arc cubics of at most
// pieceSweep with handle 4/3·tan(θ/4)·r, its circle solved from the
// joints at its ends (arcEnds: tangent to a neighbouring line, through a
// point otherwise) at each trial radius of a golden-section search
// around its Kåsa radius, the radius the mask likes best kept; then
// every line piece becomes one line cubic between the anchors its
// neighbours settled, a joint between two lines first moved to their
// intersection (rebuildLines), so collinear refined segments collapse
// into one; between the two, a fillet of a pixel or three between two
// lines becomes their corner (sharpenCorners). Each rebuild is one snap under the energy guard
// (withinBudget) over the window of what it changes; a rejected piece
// keeps its control points and anchors, so the loop stays continuous.
// The loop as a whole is never a piece (see FitCircle). The options come
// back with Fixed and Frozen renumbered for the result; the loop is
// rotated to start at a piece boundary.
func SnapPieces(b Bezigon, m Mask, img Window, tol float64, o Options) (Bezigon, Options) {
	if b.N() < 3 {
		return b, o
	}
	b, o, ps := loopPieces(b, tol, o)
	// Corners are sharpened before the arcs are snapped, so an arc whose
	// corner a fillet hid ends at that corner, typed by its line
	// neighbour.
	b, o, ps = sharpenCorners(b, o, ps, m, img, tol)
	for i := len(ps) - 1; i >= 0; i-- {
		if ps[i].kind != pieceArc {
			continue
		}
		sn, ok := guardedSnap(b, o, m, img, ps, i, ps[i].r, 0, tol, true)
		if !ok {
			continue
		}
		b, o = sn.b, sn.o
		shiftPieces(ps, i+1, sn.pieces-ps[i].count)
		ps[i].count, ps[i].centre, ps[i].r, ps[i].snapped = sn.pieces, sn.centre, sn.r, true
	}
	return rebuildLines(b, o, m, img, ps, tol)
}

// shiftPieces moves the first segment of pieces ps[from:] by delta.
func shiftPieces(ps []piece, from, delta int) {
	for k := from; k < len(ps); k++ {
		ps[k].first += delta
	}
}

// CommonRadius refits the arcs of b whose radii agree within radiusAgree
// of their mean at that mean radius, each under the energy guard. Arcs
// are the arc pieces loopPieces finds; each keeps its segment count.
func CommonRadius(b Bezigon, m Mask, img Window, tol float64, o Options) (Bezigon, Options) {
	if b.N() < 3 {
		return b, o
	}
	b, o, ps := loopPieces(b, tol, o)
	var arcs []int
	mean := 0.0
	for i, p := range ps {
		if p.kind == pieceArc && p.count < b.N() {
			arcs = append(arcs, i)
			mean += p.r
		}
	}
	if len(arcs) < 2 {
		return b, o
	}
	mean /= float64(len(arcs))
	var members []int
	sum := 0.0
	for _, i := range arcs {
		if math.Abs(ps[i].r-mean) <= radiusAgree*mean {
			members = append(members, i)
			sum += ps[i].r
		} else if c, r, ok := exactArc(b, ps[i]); ok {
			// An arc outside the agreement that SnapPieces already laid
			// as exact cubics keeps them: a member beside it ends at
			// their shared anchor instead of sliding it (arcEnds).
			ps[i].centre, ps[i].r, ps[i].snapped = c, r, true
		}
	}
	if len(members) < 2 {
		return b, o
	}
	rm := sum / float64(len(members))
	for k := len(members) - 1; k >= 0; k-- {
		i := members[k]
		if sn, ok := guardedSnap(b, o, m, img, ps, i, rm, ps[i].count, tol, false); ok {
			b, o = sn.b, sn.o
			ps[i].centre, ps[i].r, ps[i].snapped = sn.centre, sn.r, true
		}
	}
	return b, o
}

// exactArc reports whether every segment of arc piece p is an exact arc
// cubic of one circle, as arcPieces lays them, and returns that circle.
// The circle comes from the first segment alone: its centre is where the
// normal at the start anchor (the handle is tangent) meets the chord's
// perpendicular bisector; then every anchor must lie on it and every
// handle must equal the one arcPieces would place, within exactTol of
// the radius. A refined cubic is off by hundredths of a pixel and fails.
func exactArc(b Bezigon, p piece) (centre geom.Vec2, r float64, ok bool) {
	n := b.N()
	c := b.Segment(p.first % n)
	h := c[1].Sub(c[0])
	chord := c[3].Sub(c[0])
	if h.LenSq() == 0 || chord.LenSq() == 0 {
		return geom.Vec2{}, 0, false
	}
	centre, ok = fit.Intersect(c[0], geom.Vec2{X: -h.Y, Y: h.X}, c[0].Add(c[3]).Scale(0.5), geom.Vec2{X: -chord.Y, Y: chord.X})
	if !ok {
		return geom.Vec2{}, 0, false
	}
	r = c[0].Sub(centre).Len()
	tol := exactTol * (1 + r)
	for j := 0; j < p.count; j++ {
		c := b.Segment((p.first + j) % n)
		for _, a := range []geom.Vec2{c[0], c[3]} {
			if math.Abs(a.Sub(centre).Len()-r) > tol {
				return geom.Vec2{}, 0, false
			}
		}
		exact := arcPieces(centre, r, sweepBetween(c[0], c[3], centre, p.ccw), 1, c[0], c[3])
		if exact[1].Sub(c[1]).Len() > tol || exact[2].Sub(c[2]).Len() > tol {
			return geom.Vec2{}, 0, false
		}
	}
	return centre, r, true
}

// exactTol is the relative slack exactArc allows for the rounding in an
// arc cubic's construction.
const exactTol = 1e-9

// guardedSnap is snapPiece on arc piece ps[i] under the energy guard, the
// radius first refitted against the mask within radiusSearch of r when
// search is set. Each candidate is judged over the window of what it
// changes: the piece in b, the cubics that replace it, and a neighbour
// whose end moved; the residual of the rest of the loop does not pad its
// budget.
func guardedSnap(b Bezigon, o Options, m Mask, img Window, ps []piece, i int, r float64, pieces int, tol float64, search bool) (snap, bool) {
	pc := ps[i]
	if pc.count >= b.N() {
		return snap{}, false
	}
	l0 := math.Max(b.Length(), 1)
	type trial struct {
		snap
		rise, before float64
		length       float64 // boundary length of the segments the snap changes
		ok           bool
	}
	at := func(r float64) trial {
		sn, ok := snapPiece(b, o, ps, i, r, pieces, tol)
		if !ok {
			return trial{rise: math.Inf(1)}
		}
		lo, hi := 0, 0
		if sn.moved[0] {
			lo = 1
		}
		if sn.moved[1] {
			hi = 1
		}
		wl := windowAround(img, segmentsBox(b, pc.first-lo, pc.first+pc.count-1+hi), segmentsBox(sn.b, pc.first-lo, pc.first+sn.pieces-1+hi))
		if wl.W <= 0 || wl.H <= 0 {
			return trial{rise: math.Inf(1)}
		}
		before := fineDataEnergy(b, m, wl, l0)
		length := segmentsLength(b, pc.first-lo, pc.count+lo+hi)
		return trial{sn, fineDataEnergy(sn.b, m, wl, l0) - before, before, length, true}
	}
	best := at(r)
	if search {
		// Golden-section search for the radius the mask likes best.
		const phi = 0.6180339887498949
		lo, hi := r*(1-radiusSearch), r*(1+radiusSearch)
		x1, x2 := hi-phi*(hi-lo), lo+phi*(hi-lo)
		f1, f2 := at(x1), at(x2)
		for i := 0; i < radiusSteps; i++ {
			if f1.rise < f2.rise {
				hi, x2, f2 = x2, x1, f1
				x1 = hi - phi*(hi-lo)
				f1 = at(x1)
			} else {
				lo, x1, f1 = x1, x2, f2
				x2 = lo + phi*(hi-lo)
				f2 = at(x2)
			}
		}
		for _, t := range []trial{f1, f2} {
			if t.rise < best.rise {
				best = t
			}
		}
	}
	if !best.ok || !withinBudget(best.before, best.before+best.rise, l0, best.length) {
		return snap{}, false
	}
	return best.snap, true
}

// FitRectangle snaps a loop of four straight segments whose every turn
// is within rectAngleTol of 90° to the least-squares rectangle: its
// orientation the circular mean of the edge directions modulo 90°
// (within rectAxisSnap of the axes: exactly axis-aligned), its centre the
// mean of the corners, its extents the means of opposite sides. It is
// kept when every corner moves at most min(tol, 0.1·shortest side) and
// the energy guard holds; ok reports whether it was.
func FitRectangle(b Bezigon, m Mask, img Window, tol float64) (Bezigon, bool) {
	if b.N() != 4 {
		return b, false
	}
	var corners, edges [4]geom.Vec2
	for j := 0; j < 4; j++ {
		c := b.Segment(j)
		if !straightCubic(c, tol) {
			return b, false
		}
		corners[j], edges[j] = c[0], c[3].Sub(c[0])
	}
	var sx, sy float64
	for j, e := range edges {
		if e.LenSq() == 0 {
			return b, false
		}
		if d := angleBetween(e, edges[(j+1)%4])*180/math.Pi - 90; math.Abs(d) > rectAngleTol {
			return b, false
		}
		a := 4 * math.Atan2(e.Y, e.X)
		sx, sy = sx+math.Cos(a), sy+math.Sin(a)
	}
	theta := math.Atan2(sy, sx) / 4
	// theta lies in (-45°, 45°]; a diamond is 45°, not an axis case.
	if math.Abs(theta)*180/math.Pi < rectAxisSnap {
		theta = 0
	}
	u := geom.Vec2{X: math.Cos(theta), Y: math.Sin(theta)}
	w := geom.Vec2{X: -u.Y, Y: u.X}
	var centre geom.Vec2
	for _, c := range corners {
		centre = centre.Add(c)
	}
	centre = centre.Scale(0.25)
	// Opposite sides average; edge 0's axis is whichever of u, w it
	// lies closer to.
	a, c := 0.5*(edges[0].Len()+edges[2].Len()), 0.5*(edges[1].Len()+edges[3].Len())
	eu, ew := a, c
	if math.Abs(edges[0].Dot(u)) < math.Abs(edges[0].Dot(w)) {
		eu, ew = c, a
	}
	limit := math.Min(tol, 0.1*math.Min(a, c))
	var snapped [4]geom.Vec2
	for j, p := range corners {
		d := p.Sub(centre)
		su, sw := math.Copysign(1, d.Dot(u)), math.Copysign(1, d.Dot(w))
		snapped[j] = centre.Add(u.Scale(su * eu / 2)).Add(w.Scale(sw * ew / 2))
		if snapped[j].Sub(p).Len() > limit {
			return b, false
		}
	}
	out := Bezigon{Pts: make([]geom.Vec2, 0, 12)}
	for j := range snapped {
		out.Pts = append(out.Pts, lineCubic(snapped[j], snapped[(j+1)%4])...)
	}
	if !withinRise(b, out, m, img, 0, 3, 0, 3) {
		return b, false
	}
	return out, true
}

// lineCubic is the anchor and two handles of the line a–c as a cubic.
func lineCubic(a, c geom.Vec2) []geom.Vec2 {
	d := c.Sub(a)
	return []geom.Vec2{a, a.Add(d.Scale(1.0 / 3)), a.Add(d.Scale(2.0 / 3))}
}

// samples are the span's circleSamples points per segment and its end.
func samples(b Bezigon, first, count int) []geom.Vec2 {
	pts := make([]geom.Vec2, 0, count*circleSamples+1)
	for k := 0; k < count; k++ {
		c := b.Segment(first + k)
		for i := 0; i < circleSamples; i++ {
			pts = append(pts, c.At(float64(i)/circleSamples))
		}
	}
	return append(pts, b.Pts[3*((first+count)%b.N())])
}

// onCircle reports whether every point is within lim of the circle.
func onCircle(pts []geom.Vec2, centre geom.Vec2, r, lim float64) bool {
	for _, p := range pts {
		if math.Abs(p.Sub(centre).Len()-r) > lim {
			return false
		}
	}
	return true
}

// snap is an arc piece rebuilt as arc cubics: the bezigon, the options
// renumbered for it, the ends as solved (a tangent end holds its foot),
// whether each end's anchor moved (changing the neighbour there), the
// circle, and the arc's sweep and cubic count.
type snap struct {
	b      Bezigon
	o      Options
	ends   [2]arcEnd
	moved  [2]bool
	centre geom.Vec2
	r      float64
	sweep  float64
	pieces int
}

// snapPiece rebuilds arc piece ps[i] as an arc of radius r (cubics of at
// most pieceSweep, or exactly pieces when given) whose circle satisfies
// the piece's typed ends (arcEnds). A tangent circle must hold the
// piece's samples within min(tol, circleRelTol·r); when it cannot, the
// tangent ends are demoted to free, both ends tangent first, then each
// alone, then neither, which is how a corner is told from the kink a
// refinement leaves at a tangent joint. ok is false when no typing
// yields a circle (points further apart than 2r, a tangent foot beyond
// the line's far anchor).
func snapPiece(b Bezigon, o Options, ps []piece, i int, r float64, pieces int, tol float64) (snap, bool) {
	pc := ps[i]
	typed := arcEnds(b, o, ps, i, tol)
	lim := math.Min(tol, circleRelTol*r)
	pts := samples(b, pc.first, pc.count)
	tries := [][2]arcEnd{typed}
	t0, t1 := typed[0].kind == endTangent, typed[1].kind == endTangent
	if t0 && t1 {
		tries = append(tries, [2]arcEnd{demoted(typed[0]), typed[1]}, [2]arcEnd{typed[0], demoted(typed[1])})
	}
	if t0 || t1 {
		tries = append(tries, [2]arcEnd{demoted(typed[0]), demoted(typed[1])})
	}
	for _, ends := range tries {
		if sn, ok := buildArc(b, o, pc, ends, r, pieces, lim, pts); ok {
			return sn, true
		}
	}
	return snap{}, false
}

// buildArc is the piece rebuilt as the arc of radius r through, tangent
// to or free at its ends: the circle is solved, a tangent circle checked
// against the samples, the end anchors moved to the tangent feet (the
// line re-laid straight to the foot) or onto the circle (the neighbour's
// near handle following), and the arc cubics put in the piece's place.
func buildArc(b Bezigon, o Options, pc piece, ends [2]arcEnd, r float64, pieces int, lim float64, pts []geom.Vec2) (snap, bool) {
	centre, ok := solveArc(ends, r, pc.centre)
	if !ok {
		return snap{}, false
	}
	if (ends[0].kind == endTangent || ends[1].kind == endTangent) && !onCircle(pts, centre, r, lim) {
		return snap{}, false
	}
	n := b.N()
	joints := [2]int{pc.first, (pc.first + pc.count) % n}
	cand := Bezigon{Pts: append([]geom.Vec2(nil), b.Pts...)}
	var moved [2]bool
	for side, e := range ends {
		switch e.kind {
		case endTangent:
			foot := footOf(centre, e.far, e.at)
			// The tangent point stays on the far side of the line's far
			// anchor, else the line would reverse.
			if foot.Sub(e.far).Dot(e.at.Sub(e.far)) <= 0 {
				return snap{}, false
			}
			ends[side].at = foot
		case endFree:
			ends[side].at = freeEndOn(e, centre, r)
		}
		if ends[side].at != b.Pts[3*joints[side]] {
			moved[side] = true
			cand.Pts[3*joints[side]] = ends[side].at
		}
	}
	for side, e := range ends {
		if !moved[side] {
			continue
		}
		if e.kind == endTangent {
			// The line is the arc's tangent: it is re-laid straight to
			// the foot.
			copy(cand.Pts[3*e.seg:], lineCubic(cand.Pts[3*e.seg], cand.Pts[3*((e.seg+1)%n)]))
			continue
		}
		// A free end: the neighbour's end follows the anchor and its
		// shape is kept, so its near handle moves with it.
		d := e.at.Sub(b.Pts[3*joints[side]])
		h := 3*e.seg + 1
		if side == 0 {
			h = 3*e.seg + 2
		}
		cand.Pts[h] = cand.Pts[h].Add(d)
	}
	a, c := ends[0].at, ends[1].at
	sweep := sweepBetween(a, c, centre, pc.ccw)
	if pieces == 0 {
		pieces = max(int(math.Ceil(math.Abs(sweep)/pieceSweep)), 1)
	} else if math.Abs(sweep)/float64(pieces) > pieceSweep {
		return snap{}, false
	}
	out, co := replaceRun(cand, o, pc.first, pc.count, arcPieces(centre, r, sweep, pieces, a, c))
	return snap{out, co, ends, moved, centre, r, sweep, pieces}, true
}

// freeEndOn is where a free end's anchor goes on the solved circle: the
// corner where its line neighbour meets the circle (the intersection
// nearest the anchor), or, beside anything else or when the line misses
// the circle, the anchor's radial projection.
func freeEndOn(e arcEnd, centre geom.Vec2, r float64) geom.Vec2 {
	if e.line {
		d := e.at.Sub(e.far)
		if d.LenSq() > 0 {
			best, bd := geom.Vec2{}, math.Inf(1)
			for _, x := range fit.LineCircle(e.far, d.Norm(), centre, r) {
				if dd := x.Sub(e.at).Len(); dd < bd {
					best, bd = x, dd
				}
			}
			if bd < math.Inf(1) {
				return best
			}
		}
	}
	return projectOnCircle(e.at, centre, r)
}

// rebuildLines rebuilds the line pieces of ps in b. First every free
// joint between two line pieces is moved to the intersection of their
// fitted lines when that lies within min(tol, 0.1·shorter chord) of the
// anchor, the two segments meeting there straightened with it; then,
// later pieces first, each line piece becomes one line cubic between its
// end anchors, so collinear refined segments collapse into one. Each
// move is a snap under the energy guard; a rejected one keeps the loop
// as it was.
func rebuildLines(b Bezigon, o Options, m Mask, img Window, ps []piece, tol float64) (Bezigon, Options) {
	n, mp := b.N(), len(ps)
	if mp < 2 {
		return b, o
	}
	chord := func(p piece) float64 {
		return b.Pts[3*((p.first+p.count)%n)].Sub(b.Pts[3*p.first]).Len()
	}
	for i, p := range ps {
		q := ps[(i+1)%mp]
		joint := (p.first + p.count) % n
		if p.kind != pieceLine || q.kind != pieceLine || o.fixed(joint) {
			continue
		}
		p1, d1 := fit.FitLine(samples(b, p.first, p.count))
		p2, d2 := fit.FitLine(samples(b, q.first, q.count))
		x, ok := fit.Intersect(p1, d1, p2, d2)
		if !ok {
			continue
		}
		anchor := b.Pts[3*joint]
		if d := x.Sub(anchor).Len(); d > math.Min(tol, 0.1*math.Min(chord(p), chord(q))) || d < 1e-9 {
			continue
		}
		prev := (joint + n - 1) % n
		cand := Bezigon{Pts: append([]geom.Vec2(nil), b.Pts...)}
		cand.Pts[3*joint] = x
		copy(cand.Pts[3*prev:], lineCubic(cand.Pts[3*prev], x))
		copy(cand.Pts[3*joint:], lineCubic(x, cand.Pts[3*((joint+1)%n)]))
		// The window is the two segments meeting at the joint: joint-1
		// may be -1 (segment ranges are cyclic but never run backwards).
		if withinRise(b, cand, m, img, joint-1, joint, joint-1, joint) {
			b = cand
		}
	}
	for i := mp - 1; i >= 0; i-- {
		p := ps[i]
		if p.kind != pieceLine {
			continue
		}
		// A single segment stays as the refinement left it: Straightened
		// already judged its bow, and a chord in its place saves no node.
		if p.count < 2 {
			continue
		}
		a, c := b.Pts[3*p.first], b.Pts[3*((p.first+p.count)%n)]
		cand, co := replaceRun(b, o, p.first, p.count, lineCubic(a, c))
		if withinRise(b, cand, m, img, p.first, p.first+p.count-1, p.first, p.first) {
			b, o = cand, co
			n = b.N() // later pieces (lower indices) are unaffected; the loop is shorter
		}
	}
	return b, o
}

// footOf is the foot of the perpendicular from p onto the line a–b.
func footOf(p, a, b geom.Vec2) geom.Vec2 {
	d := b.Sub(a)
	if d.LenSq() == 0 {
		return a
	}
	return a.Add(d.Scale(p.Sub(a).Dot(d) / d.LenSq()))
}

// lineNormal is the unit normal of the line a–b pointing towards near,
// and the signed offset the line of the circle centres has from it.
func lineNormal(a, b, near geom.Vec2) (geom.Vec2, bool) {
	d := b.Sub(a)
	if d.LenSq() == 0 {
		return geom.Vec2{}, false
	}
	n := geom.Vec2{X: -d.Y, Y: d.X}.Norm()
	if near.Sub(a).Dot(n) < 0 {
		n = n.Scale(-1)
	}
	return n, true
}

// tangentCircle is the centre of the circle of radius r tangent to the
// lines p0–p1 and q0–q1 on the side of near; ok is false when the lines
// are parallel.
func tangentCircle(p0, p1, q0, q1 geom.Vec2, r float64, near geom.Vec2) (geom.Vec2, bool) {
	np, okp := lineNormal(p0, p1, near)
	nq, okq := lineNormal(q0, q1, near)
	if !okp || !okq {
		return geom.Vec2{}, false
	}
	// The centre is where the lines offset by r towards near meet:
	// centre·np = p0·np + r, centre·nq = q0·nq + r.
	kp, kq := p0.Dot(np)+r, q0.Dot(nq)+r
	det := float64(np.X*nq.Y) - float64(np.Y*nq.X)
	if math.Abs(det) < 1e-9 {
		return geom.Vec2{}, false
	}
	return geom.Vec2{X: (float64(kp*nq.Y) - float64(kq*np.Y)) / det, Y: (float64(np.X*kq) - float64(nq.X*kp)) / det}, true
}

// throughAndTangent is the centre of the circle of radius r through p
// and tangent to the line a–b, the one nearer near; ok is false when p
// is further than 2r from the line.
func throughAndTangent(p, a, b geom.Vec2, r float64, near geom.Vec2) (geom.Vec2, bool) {
	n, ok := lineNormal(a, b, near)
	if !ok {
		return geom.Vec2{}, false
	}
	// Centres lie on the line offset by r; the one r from p is found
	// along it from p's foot on that offset line.
	h := a.Dot(n) + r - p.Dot(n) // signed distance from p to the offset line
	if math.Abs(h) > r {
		return geom.Vec2{}, false
	}
	foot := p.Add(n.Scale(h))
	along := geom.Vec2{X: -n.Y, Y: n.X}.Scale(math.Sqrt(float64(r*r) - float64(h*h)))
	c1, c2 := foot.Add(along), foot.Sub(along)
	if c1.Sub(near).LenSq() <= c2.Sub(near).LenSq() {
		return c1, true
	}
	return c2, true
}

// sweepBetween is the signed angle from a to c about centre: positive
// (increasing angle) when ccw, else negative.
func sweepBetween(a, c, centre geom.Vec2, ccw bool) float64 {
	a0, a1 := math.Atan2(a.Y-centre.Y, a.X-centre.X), math.Atan2(c.Y-centre.Y, c.X-centre.X)
	s := math.Mod(a1-a0+4*math.Pi, 2*math.Pi)
	if ccw {
		return s
	}
	return s - 2*math.Pi
}

// centreAt is the centre of the circle of radius r through a and c on
// the side of near; ok is false when the chord is longer than 2r.
func centreAt(a, c geom.Vec2, r float64, near geom.Vec2) (geom.Vec2, bool) {
	d := c.Sub(a)
	half := d.Len() / 2
	if half == 0 || r < half {
		return geom.Vec2{}, false
	}
	mid := a.Add(c).Scale(0.5)
	nrm := geom.Vec2{X: -d.Y, Y: d.X}.Norm()
	h := math.Sqrt(float64(r*r) - float64(half*half))
	if near.Sub(mid).Dot(nrm) < 0 {
		h = -h
	}
	return mid.Add(nrm.Scale(h)), true
}

// arcPieces is the control polygon (anchor, handle, handle per piece) of
// the arc of the circle (centre, r) from a to c sweeping sweep radians
// in the given number of pieces; the first anchor is a itself and the
// last handle leads to c.
func arcPieces(centre geom.Vec2, r, sweep float64, pieces int, a, c geom.Vec2) []geom.Vec2 {
	step := sweep / float64(pieces)
	k := float64(4.0/3*math.Tan(math.Abs(step)/4)) * r
	a0 := math.Atan2(a.Y-centre.Y, a.X-centre.X)
	at := func(ang float64) geom.Vec2 {
		return geom.Vec2{X: centre.X + float64(r*math.Cos(ang)), Y: centre.Y + float64(r*math.Sin(ang))}
	}
	tangent := func(ang float64) geom.Vec2 {
		t := geom.Vec2{X: -math.Sin(ang), Y: math.Cos(ang)}
		if step < 0 {
			t = t.Scale(-1)
		}
		return t
	}
	out := make([]geom.Vec2, 0, 3*pieces)
	for i := 0; i < pieces; i++ {
		s, e := a0+float64(float64(i)*step), a0+float64(float64(i+1)*step)
		ps, pe := at(s), at(e)
		if i == 0 {
			ps = a
		}
		if i == pieces-1 {
			pe = c
		}
		out = append(out, ps, ps.Add(tangent(s).Scale(k)), pe.Sub(tangent(e).Scale(k)))
	}
	return out
}

// replaceRun returns b with segments first..first+count-1 (not wrapping)
// replaced by the segments of pts, and the options renumbered: the run's
// first joint keeps its flags, the new interior joints and segments are
// free.
func replaceRun(b Bezigon, o Options, first, count int, pts []geom.Vec2) (Bezigon, Options) {
	n := b.N()
	out := Bezigon{Pts: make([]geom.Vec2, 0, len(b.Pts)-3*count+len(pts))}
	out.Pts = append(out.Pts, b.Pts[:3*first]...)
	out.Pts = append(out.Pts, pts...)
	out.Pts = append(out.Pts, b.Pts[3*(first+count):]...)
	ro := o
	renumber := func(flags []bool, get func(int) bool) []bool {
		if flags == nil {
			return nil
		}
		res := make([]bool, 0, n-count+len(pts)/3)
		for k := 0; k < first; k++ {
			res = append(res, get(k))
		}
		res = append(res, get(first))
		for k := 1; k < len(pts)/3; k++ {
			res = append(res, false)
		}
		for k := first + count; k < n; k++ {
			res = append(res, get(k))
		}
		return res
	}
	ro.Fixed = renumber(o.Fixed, o.fixed)
	ro.Frozen = renumber(o.Frozen, func(s int) bool {
		if s == first {
			return false // the first new segment is not the old one
		}
		return o.frozen(s)
	})
	return out, ro
}

// fineDataEnergy is localDataEnergy with CoverageFine: the data energy of
// what b renders over wl.
func fineDataEnergy(b Bezigon, m Mask, wl Window, l0 float64) float64 {
	cov := make([]float32, wl.W*wl.H)
	CoverageFine(b, wl, cov)
	return DataEnergy(cov, m, wl, l0)
}

// withinRise reports whether cand, which differs from b only in segments
// bFrom..bTo of b (cFrom..cTo of cand, cyclic), keeps the rendered data
// energy over that window within budget of b's.
func withinRise(b, cand Bezigon, m Mask, img Window, bFrom, bTo, cFrom, cTo int) bool {
	l0 := math.Max(b.Length(), 1)
	wl := windowAround(img, segmentsBox(b, bFrom, bTo), segmentsBox(cand, cFrom, cTo))
	if wl.W <= 0 || wl.H <= 0 {
		return true
	}
	n := b.N()
	return withinBudget(fineDataEnergy(b, m, wl, l0), fineDataEnergy(cand, m, wl, l0), l0, segmentsLength(b, bFrom, ((bTo-bFrom)%n+n)%n+1))
}

// polygonArea2 is twice the signed area of the flattened outline.
func polygonArea2(b Bezigon) float64 {
	pts := b.Polyline()
	a := 0.0
	for i := range pts {
		a += geom.Cross(pts[i], pts[(i+1)%len(pts)])
	}
	return a
}
