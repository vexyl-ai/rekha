package refine

import (
	"math"

	"github.com/vexyl-ai/rekha/internal/geom"
)

const (
	// primRise is the relative rise in data energy a primitive snap may
	// cost over the window it changes; beyond it the snap is reverted.
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

// arcRun is a run of consecutive curved segments first..first+count-1
// (never wrapping) that lie on one circle.
type arcRun struct {
	first, count int
	centre       geom.Vec2
	r            float64
	ccw          bool // the run goes round the centre with increasing angle
}

// FitArcs replaces every run of curved segments that lies on one circle
// by circular-arc cubics: maximal runs bounded by straight or frozen
// segments and by fixed joints, cut where an interior joint turns by
// arcJointTurn or more, whose circleSamples points per segment lie within
// min(tol, circleRelTol·r) of a Kåsa-fitted circle. The run becomes
// pieces of at most pieceSweep with handle 4/3·tan(θ/4)·r. A fixed end
// anchor stays where it is; a free one slides to the circle's tangent
// point on its neighbouring straight segment, and a free end beside a
// curved segment leaves the run alone (an anchor there is nobody's
// tangent point, and the kink it would keep costs more than the guard
// can see). The circle is placed through the kept ends and tangent to
// the lines, and its radius refitted against the mask. A snap that
// raises the data energy over its window beyond primRise is reverted.
// The loop as a whole is never a run (see FitCircle). The options come
// back with Fixed and Frozen renumbered for the result; a loop whose
// first joint is not a run boundary is rotated to start at one.
func FitArcs(b Bezigon, m Mask, img Window, tol float64, o Options) (Bezigon, Options) {
	if b.N() < 2 {
		return b, o
	}
	b, o = rotateToCut(b, tol, o)
	runs := arcRuns(b, tol, o)
	// Later runs first, so earlier indices stay valid.
	for i := len(runs) - 1; i >= 0; i-- {
		run := runs[i]
		if cand, co, ok := guardedSnap(b, o, m, img, run, run.r, 0, tol, true); ok {
			b, o = cand, co
		}
	}
	return b, o
}

// CommonRadius refits the arcs of b whose radii agree within radiusAgree
// of their mean at that mean radius, each under the energy guard. Arcs
// are found as FitArcs finds them; each keeps its segment count.
func CommonRadius(b Bezigon, m Mask, img Window, tol float64, o Options) (Bezigon, Options) {
	if b.N() < 2 {
		return b, o
	}
	b, o = rotateToCut(b, tol, o)
	runs := arcRuns(b, tol, o)
	if len(runs) < 2 {
		return b, o
	}
	mean := 0.0
	for _, r := range runs {
		mean += r.r
	}
	mean /= float64(len(runs))
	var members []arcRun
	sum := 0.0
	for _, r := range runs {
		if math.Abs(r.r-mean) <= radiusAgree*mean {
			members = append(members, r)
			sum += r.r
		}
	}
	if len(members) < 2 {
		return b, o
	}
	rm := sum / float64(len(members))
	for i := len(members) - 1; i >= 0; i-- {
		run := members[i]
		if cand, co, ok := guardedSnap(b, o, m, img, run, rm, run.count, tol, false); ok {
			b, o = cand, co
		}
	}
	return b, o
}

// guardedSnap is snapRun under the energy guard, the radius first
// refitted against the mask within radiusSearch of r when search is set.
// Each candidate is judged over the window of what it changes: the run
// in b, the pieces that replace it, and a neighbour rebuilt as a line
// when its end slid; the residual of the rest of the loop does not pad
// its budget.
func guardedSnap(b Bezigon, o Options, m Mask, img Window, run arcRun, r float64, pieces int, tol float64, search bool) (Bezigon, Options, bool) {
	if run.first == 0 && run.count == b.N() {
		return b, o, false
	}
	l0 := math.Max(b.Length(), 1)
	type trial struct {
		snap
		r            float64
		rise, before float64
		ok           bool
	}
	at := func(r float64) trial {
		sn, ok := snapRun(b, o, run, r, pieces, tol)
		if !ok {
			return trial{rise: math.Inf(1)}
		}
		lo, hi := 0, 0
		if sn.slid[0] {
			lo = 1
		}
		if sn.slid[1] {
			hi = 1
		}
		wl := windowAround(img, segmentsBox(b, run.first-lo, run.first+run.count-1+hi), segmentsBox(sn.b, run.first-lo, run.first+sn.pieces-1+hi))
		if wl.W <= 0 || wl.H <= 0 {
			return trial{rise: math.Inf(1)}
		}
		before := fineDataEnergy(b, m, wl, l0)
		return trial{sn, r, fineDataEnergy(sn.b, m, wl, l0) - before, before, true}
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
	if !best.ok || best.before+best.rise > budget(best.before) {
		return b, o, false
	}
	return best.b, best.o, true
}

// budget is the data energy a snap may reach: before plus primRise of it.
func budget(before float64) float64 {
	return float64(before * (1 + primRise))
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
	if math.Abs(theta)*180/math.Pi < rectAxisSnap || math.Abs(math.Abs(theta)-math.Pi/4)*180/math.Pi < rectAxisSnap {
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

// arcRuns lists the runs of b that lie on circles, in segment order.
func arcRuns(b Bezigon, tol float64, o Options) []arcRun {
	n := b.N()
	cut := cutJoints(b, tol, o)
	if cut == nil {
		return nil // the whole loop is one curved run: not an arc
	}
	var runs []arcRun
	for s := 0; s < n; {
		if !cut[s] || !curved(b, s, tol, o) {
			s++
			continue
		}
		count := 1
		for s+count < n && !cut[s+count] && curved(b, s+count, tol, o) {
			count++
		}
		if run, ok := fitRun(b, s, count, tol); ok {
			runs = append(runs, run)
		}
		s += count
	}
	return runs
}

// curved reports whether segment j can belong to an arc run.
func curved(b Bezigon, j int, tol float64, o Options) bool {
	return !o.frozen(j) && !straightCubic(b.Segment(j), tol)
}

// cutJoints marks the joints that end arc runs: fixed joints, joints
// beside a straight or frozen segment, and joints turning by
// arcJointTurn or more. It returns nil when no joint is a cut.
func cutJoints(b Bezigon, tol float64, o Options) []bool {
	n := b.N()
	cut := make([]bool, n)
	any := false
	for a := 0; a < n; a++ {
		prev := (a + n - 1) % n
		cut[a] = o.fixed(a) || !curved(b, a, tol, o) || !curved(b, prev, tol, o) || AnglePrior(b, a) >= arcJointTurn*math.Pi/180
		any = any || cut[a]
	}
	if !any {
		return nil
	}
	return cut
}

// rotateToCut rotates b to start at a cut joint when its first joint is
// not one, so no arc run wraps round the end of the loop.
func rotateToCut(b Bezigon, tol float64, o Options) (Bezigon, Options) {
	cut := cutJoints(b, tol, o)
	if cut == nil || cut[0] {
		return b, o
	}
	s := 0
	for !cut[s] {
		s++
	}
	n := b.N()
	out := Bezigon{Pts: make([]geom.Vec2, 0, len(b.Pts))}
	out.Pts = append(out.Pts, b.Pts[3*s:]...)
	out.Pts = append(out.Pts, b.Pts[:3*s]...)
	ro := o
	if o.Fixed != nil {
		ro.Fixed = make([]bool, n)
		for k := range ro.Fixed {
			ro.Fixed[k] = o.fixed((k + s) % n)
		}
	}
	if o.Frozen != nil {
		ro.Frozen = make([]bool, n)
		for k := range ro.Frozen {
			ro.Frozen[k] = o.frozen((k + s) % n)
		}
	}
	return out, ro
}

// samples are the run's circleSamples points per segment and its end.
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

// fitRun fits a circle to the run's samples (Kåsa) and accepts the run
// when every sample is within min(tol, circleRelTol·r) of it.
func fitRun(b Bezigon, first, count int, tol float64) (arcRun, bool) {
	pts := samples(b, first, count)
	centre, r, ok := kasa(pts)
	if !ok || !onCircle(pts, centre, r, math.Min(tol, circleRelTol*r)) {
		return arcRun{}, false
	}
	a := pts[0]
	am := pts[len(pts)/2]
	// The run's direction round the centre: the side its middle sample
	// lies on.
	ccw := math.Mod(math.Atan2(am.Y-centre.Y, am.X-centre.X)-math.Atan2(a.Y-centre.Y, a.X-centre.X)+4*math.Pi, 2*math.Pi) < sweepBetween(a, pts[len(pts)-1], centre, true)
	return arcRun{first: first, count: count, centre: centre, r: r, ccw: ccw}, true
}

// runEnd is one end of an arc run: the anchor there, and, when the
// anchor is free and its neighbour a straight segment, that segment
// (slide set), whose far anchor the line runs from or to.
type runEnd struct {
	at    geom.Vec2
	slide bool
	seg   int
	far   geom.Vec2
}

// runEnds describes the two ends of run; ok is false when a free end
// sits beside a curved (or frozen) segment, which no snap may move.
func runEnds(b Bezigon, o Options, run arcRun, tol float64) (ends [2]runEnd, ok bool) {
	n := b.N()
	prev, next := (run.first+n-1)%n, (run.first+run.count)%n
	ends[0] = runEnd{at: b.Pts[3*run.first], seg: prev, far: b.Pts[3*prev]}
	ends[1] = runEnd{at: b.Pts[3*next], seg: next, far: b.Pts[3*((next+1)%n)]}
	if !o.fixed(run.first) {
		if prev == next || o.frozen(prev) || !straightCubic(b.Segment(prev), tol) {
			return ends, false
		}
		ends[0].slide = true
	}
	if !o.fixed(next) {
		if prev == next || o.frozen(next) || !straightCubic(b.Segment(next), tol) {
			return ends, false
		}
		ends[1].slide = true
	}
	return ends, true
}

// snap is a run rebuilt as an arc: the bezigon, the options renumbered
// for it, whether an end slid, and the arc's sweep and piece count.
type snap struct {
	b      Bezigon
	o      Options
	slid   [2]bool // whether each end slid, rebuilding its neighbour as a line
	sweep  float64
	pieces int
}

// snapRun rebuilds the run as an arc of radius r (pieces of at most
// pieceSweep, or exactly pieces when given). The circle goes through
// the fixed ends and is tangent to the sliding ends' lines, whose
// anchors move to the tangent points; when both ends slide the run's
// samples must still lie within min(tol, circleRelTol·r) of it.
func snapRun(b Bezigon, o Options, run arcRun, r float64, pieces int, tol float64) (snap, bool) {
	ends, ok := runEnds(b, o, run, tol)
	if !ok {
		return snap{}, false
	}
	var centre geom.Vec2
	switch {
	case ends[0].slide && ends[1].slide:
		centre, ok = tangentCircle(ends[0].far, ends[0].at, ends[1].at, ends[1].far, r, run.centre)
		ok = ok && onCircle(samples(b, run.first, run.count), centre, r, math.Min(tol, circleRelTol*r))
	case ends[0].slide:
		centre, ok = throughAndTangent(ends[1].at, ends[0].far, ends[0].at, r, run.centre)
	case ends[1].slide:
		centre, ok = throughAndTangent(ends[0].at, ends[1].at, ends[1].far, r, run.centre)
	default:
		centre, ok = centreAt(ends[0].at, ends[1].at, r, run.centre)
	}
	if !ok {
		return snap{}, false
	}
	cand := Bezigon{Pts: append([]geom.Vec2(nil), b.Pts...)}
	var slid [2]bool
	for i, e := range ends {
		if !e.slide {
			continue
		}
		foot := footOf(centre, e.far, e.at)
		// The tangent point stays on the far side of the line's far
		// anchor, else the line would reverse.
		if foot.Sub(e.far).Dot(e.at.Sub(e.far)) <= 0 {
			return snap{}, false
		}
		ends[i].at = foot
		slid[i] = true
		if i == 0 {
			copy(cand.Pts[3*e.seg:], lineCubic(e.far, foot))
		} else {
			copy(cand.Pts[3*e.seg:], lineCubic(foot, e.far))
		}
	}
	a, c := ends[0].at, ends[1].at
	sweep := sweepBetween(a, c, centre, run.ccw)
	if pieces == 0 {
		pieces = max(int(math.Ceil(math.Abs(sweep)/pieceSweep)), 1)
	} else if math.Abs(sweep)/float64(pieces) > pieceSweep {
		return snap{}, false
	}
	pts := arcPieces(centre, r, sweep, pieces, a, c)
	out, co := replaceRun(cand, o, run.first, run.count, pts)
	return snap{out, co, slid, sweep, pieces}, true
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
	return fineDataEnergy(cand, m, wl, l0) <= budget(fineDataEnergy(b, m, wl, l0))
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
