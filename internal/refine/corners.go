package refine

import (
	"github.com/vexyl-ai/rekha/internal/fit"
	"github.com/vexyl-ai/rekha/internal/geom"
)

const (
	// cornerFillet is the longest chord, in pixels, of a piece between
	// two lines that is taken for a rounded corner the trace should not
	// have: anti-aliasing and the refinement round a sharp corner by a
	// pixel or three.
	cornerFillet = 3.0
	// cornerReach is how far the two lines' intersection may lie from
	// the fillet's midpoint for the fillet to be their corner.
	cornerReach = 2 * cornerFillet
)

// sharpenCorners drops every fillet of b: a piece of any kind whose
// chord is at most cornerFillet, whose joints are free and segments not
// frozen, between two line pieces of chord over cornerFillet whose
// fitted lines meet within cornerReach of the fillet's midpoint. The
// previous line is ended, and the next begun, at the intersection. Each
// drop is a snap under the energy guard over the fillet and the two
// line ends it changes; later pieces first, so earlier indices stay
// valid. The pieces come back renumbered for the result.
func sharpenCorners(b Bezigon, o Options, ps []piece, m Mask, img Window, tol float64) (Bezigon, Options, []piece) {
	for i := len(ps) - 1; i >= 0 && len(ps) >= 3; i-- {
		n, mp := b.N(), len(ps)
		p := ps[i]
		ip, in := (i+mp-1)%mp, (i+1)%mp
		prev, next := ps[ip], ps[in]
		if ip == in || prev.kind != pieceLine || next.kind != pieceLine {
			continue
		}
		j0, j1 := p.first, (p.first+p.count)%n
		if o.fixed(j0) || o.fixed(j1) || anyFrozen(o, j0, p.count) {
			continue
		}
		if pieceChord(b, p) > cornerFillet || pieceChord(b, prev) <= cornerFillet || pieceChord(b, next) <= cornerFillet {
			continue
		}
		p1, d1 := fit.FitLine(samples(b, prev.first, prev.count))
		p2, d2 := fit.FitLine(samples(b, next.first, next.count))
		x, ok := fit.Intersect(p1, d1, p2, d2)
		if !ok || x.Sub(b.Pts[3*j0].Add(b.Pts[3*j1]).Scale(0.5)).Len() > cornerReach {
			continue
		}
		cand := Bezigon{Pts: append([]geom.Vec2(nil), b.Pts...)}
		last := (j0 + n - 1) % n // the previous line's last segment
		copy(cand.Pts[3*last:], lineCubic(cand.Pts[3*last], x))
		copy(cand.Pts[3*j1:], lineCubic(x, cand.Pts[3*((j1+1)%n)]))
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
