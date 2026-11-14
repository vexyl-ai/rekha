package refine

import (
	"math"

	"github.com/vexyl-ai/rekha/internal/fit"
	"github.com/vexyl-ai/rekha/internal/geom"
)

const (
	// cornerFillet is the longest chord, in pixels, of a piece between
	// two edges that is taken for a rounded corner the trace should not
	// have: anti-aliasing and the refinement round a sharp corner by a
	// pixel or three, and where an arc meets a line the refined corner
	// anchor drifts another pixel along the line (annular-sector's inner
	// corners leave a 3.2 px sliver).
	cornerFillet = 4.0
	// cornerReach is how far the two lines' intersection may lie from
	// the fillet's midpoint for the fillet to be their corner.
	cornerReach = 2 * cornerFillet
)

// sharpenCorners drops every fillet of b: a piece of any kind whose
// chord is at most cornerFillet, whose joints are free and segments not
// frozen, between two pieces of chord over cornerFillet that are each a
// line or an arc and that meet (line with line, line with circle, or
// circle with circle) within cornerReach of the fillet's midpoint. The
// previous piece is ended, and the next begun, at that corner: a line is
// re-laid straight to it, an arc's last cubic as the arc of its circle
// to it (the arc snap that follows re-lays the whole arc exactly, ending
// where its line neighbour meets the circle). Each drop is a snap under
// the energy guard over the fillet and the two piece ends it changes;
// later pieces first, so earlier indices stay valid. The pieces come
// back renumbered for the result.
func sharpenCorners(b Bezigon, o Options, ps []piece, m Mask, img Window, tol float64) (Bezigon, Options, []piece) {
	for i := len(ps) - 1; i >= 0 && len(ps) >= 3; i-- {
		n, mp := b.N(), len(ps)
		p := ps[i]
		ip, in := (i+mp-1)%mp, (i+1)%mp
		prev, next := ps[ip], ps[in]
		if ip == in || prev.kind == pieceFree || next.kind == pieceFree {
			continue
		}
		j0, j1 := p.first, (p.first+p.count)%n
		if o.fixed(j0) || o.fixed(j1) || anyFrozen(o, j0, p.count) {
			continue
		}
		if pieceChord(b, p) > cornerFillet || pieceChord(b, prev) <= cornerFillet || pieceChord(b, next) <= cornerFillet {
			continue
		}
		mid := b.Pts[3*j0].Add(b.Pts[3*j1]).Scale(0.5)
		x, ok := cornerBetween(b, prev, next, mid)
		if !ok || x.Sub(mid).Len() > cornerReach {
			continue
		}
		cand := Bezigon{Pts: append([]geom.Vec2(nil), b.Pts...)}
		moveEnd(cand, (j0+n-1)%n, true, x, prev)
		moveEnd(cand, j1, false, x, next)
		cand, co := removeRun(cand, o, j0, p.count)
		if !withinRise(b, cand, m, img, j0-1, j0+p.count, j0-1, j0) {
			continue
		}
		b, o = cand, co
		shiftPieces(ps, i+1, -p.count)
		ps = append(ps[:i], ps[i+1:]...)
	}
	return b, o, ps
}

// cornerBetween is where the fits of two line or arc pieces meet: the
// intersection of the two lines, or the one of the two line–circle or
// circle–circle intersections nearest mid. ok is false when they do not
// meet.
func cornerBetween(b Bezigon, prev, next piece, mid geom.Vec2) (geom.Vec2, bool) {
	line := func(p piece) (geom.Vec2, geom.Vec2) { return fit.FitLine(samples(b, p.first, p.count)) }
	var cands []geom.Vec2
	switch {
	case prev.kind == pieceLine && next.kind == pieceLine:
		p1, d1 := line(prev)
		p2, d2 := line(next)
		return fit.Intersect(p1, d1, p2, d2)
	case prev.kind == pieceLine:
		p1, d1 := line(prev)
		cands = fit.LineCircle(p1, d1, next.centre, next.r)
	case next.kind == pieceLine:
		p2, d2 := line(next)
		cands = fit.LineCircle(p2, d2, prev.centre, prev.r)
	default:
		cands = fit.CircleCircle(prev.centre, prev.r, next.centre, next.r)
	}
	best, bd := geom.Vec2{}, math.Inf(1)
	for _, x := range cands {
		if d := x.Sub(mid).Len(); d < bd {
			best, bd = x, d
		}
	}
	return best, bd < math.Inf(1)
}

// moveEnd moves one end of segment seg of b, the last or first segment
// of piece p, to x: its end anchor when end is set, else its start. A
// line piece's segment is re-laid straight; an arc piece's is re-laid as
// the arc of the piece's circle between its other anchor and x (a
// handle merely moved, or the cubic stretched, leaves the arc sagging
// inside its circle by half a pixel).
func moveEnd(b Bezigon, seg int, end bool, x geom.Vec2, p piece) {
	n := b.N()
	s, e := 3*seg, 3*((seg+1)%n)
	a, d := b.Pts[s], b.Pts[e]
	if end {
		d = x
	} else {
		a = x
	}
	if p.kind == pieceLine {
		copy(b.Pts[s:s+3], lineCubic(a, d))
	} else {
		copy(b.Pts[s:s+3], arcPieces(p.centre, p.r, sweepBetween(a, d, p.centre, p.ccw), 1, a, d))
	}
	b.Pts[e] = d
}

// anyFrozen reports whether any of segments first..first+count-1 is frozen.
func anyFrozen(o Options, first, count int) bool {
	for k := first; k < first+count; k++ {
		if o.frozen(k) {
			return true
		}
	}
	return false
}

// pieceChord is the distance between the piece's end anchors.
func pieceChord(b Bezigon, p piece) float64 {
	return b.Pts[3*((p.first+p.count)%b.N())].Sub(b.Pts[3*p.first]).Len()
}

// removeRun returns b without segments first..first+count-1 (not
// wrapping), the joint at first gone and the one after the run kept,
// and the options with those segments' flags dropped.
func removeRun(b Bezigon, o Options, first, count int) (Bezigon, Options) {
	out := Bezigon{Pts: make([]geom.Vec2, 0, len(b.Pts)-3*count)}
	out.Pts = append(out.Pts, b.Pts[:3*first]...)
	out.Pts = append(out.Pts, b.Pts[3*(first+count):]...)
	drop := func(flags []bool) []bool {
		if flags == nil {
			return nil
		}
		res := make([]bool, 0, len(flags)-count)
		res = append(res, flags[:first]...)
		return append(res, flags[first+count:]...)
	}
	ro := o
	ro.Fixed, ro.Frozen = drop(o.Fixed), drop(o.Frozen)
	return out, ro
}
