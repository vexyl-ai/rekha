package refine

import (
	"math"

	"github.com/vexyl-ai/rekha/internal/fit"
	"github.com/vexyl-ai/rekha/internal/geom"
)

// pieceKind says what a piece of a loop is: a free run of refined
// segments nothing fits, a straight line, or an arc of one circle.
type pieceKind uint8

const (
	pieceFree pieceKind = iota
	pieceLine
	pieceArc
)

// piece is one or more consecutive segments of a loop (first..first+
// count-1, never wrapping) and what they lie on: a line (p, dir) or a
// circle (centre, r, ccw).
type piece struct {
	first, count int
	kind         pieceKind
	centre       geom.Vec2 // arc
	r            float64   // arc
	ccw          bool      // arc: the piece goes round the centre with increasing angle
	snapped      bool      // arc: rebuilt as exact arc cubics; centre and r are then the solved circle
	p, dir       geom.Vec2 // line: a point on it and its unit direction, first anchor towards last
}

// hardCuts marks the joints that no piece may span: fixed joints, the
// ends of frozen segments, and joints that turn by arcJointTurn or more.
// It returns nil when no joint is a cut.
func hardCuts(b Bezigon, o Options) []bool {
	n := b.N()
	cut := make([]bool, n)
	cutAny := false
	for a := 0; a < n; a++ {
		prev := (a + n - 1) % n
		cut[a] = o.fixed(a) || o.frozen(a) || o.frozen(prev) || AnglePrior(b, a) >= arcJointTurn*math.Pi/180
		cutAny = cutAny || cut[a]
	}
	if !cutAny {
		return nil
	}
	return cut
}

// fitSpan fits segments first..first+count-1 by one line or one circle
// and reports what fitted. The span is a line when every sample lies
// within tol of the total-least-squares line through them and no circle
// explains it better: either no circle fits or the circle's sagitta
// over the span's chord is under tol, so the two explanations agree to
// within tolerance and the simpler one wins. Else it is an arc when
// every sample lies within min(tol, circleRelTol·r) of the Kåsa
// circle. Else nothing fits.
func fitSpan(b Bezigon, first, count int, tol float64) (piece, bool) {
	pts := samples(b, first, count)
	out := piece{first: first, count: count}
	// The circle is fitted and tested on the interior samples: the span's
	// end anchors are re-placed by the arc solve (a free end moves onto
	// the circle, a tangent end to its foot), and a refined corner anchor
	// wanders along its edges by up to a pixel, which would otherwise
	// cut the last few pixels of an arc off as a piece of their own.
	inner := pts[1 : len(pts)-1]
	centre, r, circleOK := kasa(inner)
	circleOK = circleOK && onCircle(inner, centre, r, math.Min(tol, circleRelTol*r))
	p, dir := fit.FitLine(pts)
	lineOK := dir.LenSq() > 0
	for _, q := range pts {
		if !lineOK {
			break
		}
		lineOK = math.Abs(geom.Cross(dir, q.Sub(p))) <= tol
	}
	if lineOK {
		sagitta := math.Inf(1)
		if circleOK {
			if half := pts[0].Sub(pts[len(pts)-1]).Len() / 2; half <= r {
				sagitta = r - math.Sqrt(math.Max(float64(r*r)-float64(half*half), 0))
			}
		}
		if !circleOK || sagitta < tol {
			out.kind, out.p, out.dir = pieceLine, p, dir
			return out, true
		}
	}
	if circleOK {
		out.kind, out.centre, out.r = pieceArc, centre, r
		a, am := pts[0], pts[len(pts)/2]
		out.ccw = math.Mod(math.Atan2(am.Y-centre.Y, am.X-centre.X)-math.Atan2(a.Y-centre.Y, a.X-centre.X)+4*math.Pi, 2*math.Pi) < sweepBetween(a, pts[len(pts)-1], centre, true)
		return out, true
	}
	return out, false
}

// loopPieces cuts b into pieces: it rotates the loop (and its options)
// to start at a hard cut, or, when there is none, at the joint nearest
// the sample furthest from the loop's Kåsa circle, so every span has
// ends; splits each span between hard cuts where one fit does not hold
// (at the joint nearest the sample of greatest radial deviation from the
// span's circle, or the middle joint when no circle can be fitted);
// re-merges adjacent fitted pieces whose union still fits, so a split
// that landed inside a true arc heals; and returns the pieces in order,
// covering every segment once, free pieces filling the gaps. A frozen
// segment is always a free piece of its own.
func loopPieces(b Bezigon, tol float64, o Options) (Bezigon, Options, []piece) {
	n := b.N()
	if n < 3 {
		if n == 0 {
			return b, o, nil
		}
		return b, o, []piece{{first: 0, count: n}}
	}
	cut := hardCuts(b, o)
	if cut == nil {
		s := 0
		if centre, r, ok := kasa(samples(b, 0, n)); ok {
			pts := samples(b, 0, n)
			worst, k := -1.0, 0
			for i, p := range pts[:len(pts)-1] {
				if d := math.Abs(p.Sub(centre).Len() - r); d > worst {
					worst, k = d, i
				}
			}
			s = int(math.Round(float64(k)/circleSamples)) % n
		}
		b, o = rotateLoop(b, o, s)
		cut = make([]bool, n)
		cut[0] = true
	} else {
		s := 0
		for !cut[s] {
			s++
		}
		b, o = rotateLoop(b, o, s)
		cut = hardCuts(b, o)
	}
	var ps []piece
	for s := 0; s < n; {
		e := s + 1
		for e < n && !cut[e] {
			e++
		}
		if o.frozen(s) {
			ps = append(ps, piece{first: s, count: e - s})
		} else {
			ps = splitPieces(b, s, e-s, tol, ps)
		}
		s = e
	}
	return b, o, mergePieces(b, ps, cut, tol)
}

// rotateLoop rotates b to start at joint s, renumbering o's flags.
func rotateLoop(b Bezigon, o Options, s int) (Bezigon, Options) {
	n := b.N()
	s = ((s % n) + n) % n
	if s == 0 {
		return b, o
	}
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

// splitPieces appends the pieces of span first..first+count-1 to out:
// the span itself when one fit holds, else the pieces of its two halves,
// cut at the joint nearest the span's worst sample, kept within the
// span's middle half so that a smooth curve no circle fits (an ellipse,
// a limaçon), whose worst sample is always at an end, is halved rather
// than peeled one segment at a time (quadratic in the segment count); a
// single segment nothing fits is free.
func splitPieces(b Bezigon, first, count int, tol float64, out []piece) []piece {
	if p, ok := fitSpan(b, first, count, tol); ok {
		return append(out, p)
	}
	if count == 1 {
		return append(out, piece{first: first, count: 1})
	}
	at := count / 2
	pts := samples(b, first, count)
	if centre, r, ok := kasa(pts); ok {
		worst, k := -1.0, 0
		for i, p := range pts {
			if d := math.Abs(p.Sub(centre).Len() - r); d > worst {
				worst, k = d, i
			}
		}
		lo, hi := max(1, count/4), min(count-1, count-count/4)
		at = min(max(int(math.Round(float64(k)/circleSamples)), lo), hi)
	}
	out = splitPieces(b, first, at, tol, out)
	return splitPieces(b, first+at, count-at, tol, out)
}

// mergePieces merges adjacent fitted pieces whose union still fits and
// does not span a hard cut, left to right, until nothing merges.
func mergePieces(b Bezigon, ps []piece, cut []bool, tol float64) []piece {
	for merged := true; merged; {
		merged = false
		for i := 0; i+1 < len(ps); i++ {
			a, c := ps[i], ps[i+1]
			if a.kind == pieceFree || c.kind == pieceFree || cut[c.first] {
				continue
			}
			if u, ok := fitSpan(b, a.first, a.count+c.count, tol); ok {
				ps[i] = u
				ps = append(ps[:i+1], ps[i+2:]...)
				merged = true
				i--
			}
		}
	}
	return ps
}
