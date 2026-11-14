package refine

import (
	"math"

	"github.com/vexyl-ai/rekha/internal/geom"
)

// endKind says how an arc piece's circle is solved at one of its ends:
// through a point, or tangent to the neighbouring line.
type endKind uint8

const (
	endPoint endKind = iota
	endTangent
)

// arcEnd is one end of an arc piece: for a point end the point the
// circle passes through; for a tangent end the neighbouring line's other
// anchor (the line runs far→at into the arc, or at→far out of it). seg
// is the neighbouring segment the end touches, and line whether that
// neighbour is a line piece, whose end is rebuilt when the anchor moves.
type arcEnd struct {
	kind endKind
	at   geom.Vec2
	far  geom.Vec2
	seg  int
	line bool
}

// arcEnds types the two ends of arc piece ps[i] from the joints they sit
// on: a fixed joint is a point at the anchor; a line neighbour the arc
// meets within arcJointTurn is a tangent; a line neighbour it meets at a
// corner is a point at the anchor's radial projection onto the piece's
// circle; an arc neighbour is a point at the mean of the anchor's
// projections onto both circles (both pass within lim of it, so the
// kink is at most lim); a free neighbour, or no neighbour because the
// piece spans the whole loop, is a point at the anchor.
func arcEnds(b Bezigon, o Options, ps []piece, i int, tol float64) [2]arcEnd {
	n, m := b.N(), len(ps)
	pc := ps[i]
	var ends [2]arcEnd
	for side := 0; side < 2; side++ {
		var joint, seg int
		var nb piece
		var far geom.Vec2
		if side == 0 {
			joint = pc.first
			seg = (pc.first + n - 1) % n
			nb = ps[(i+m-1)%m]
			far = b.Pts[3*nb.first]
		} else {
			joint = (pc.first + pc.count) % n
			seg = joint
			nb = ps[(i+1)%m]
			far = b.Pts[3*((nb.first+nb.count)%n)]
		}
		anchor := b.Pts[3*joint]
		e := arcEnd{kind: endPoint, at: anchor, seg: seg, line: nb.kind == pieceLine}
		switch {
		case o.fixed(joint) || m == 1:
		case nb.kind == pieceLine:
			if AnglePrior(b, joint) < arcJointTurn*math.Pi/180 {
				e.kind, e.far = endTangent, far
			} else {
				e.at = projectOnCircle(anchor, pc.centre, pc.r)
			}
		case nb.kind == pieceArc:
			e.at = projectOnCircle(anchor, pc.centre, pc.r).Add(projectOnCircle(anchor, nb.centre, nb.r)).Scale(0.5)
		}
		ends[side] = e
	}
	return ends
}

// projectOnCircle is p moved radially onto the circle (centre, r); the
// centre itself stays put.
func projectOnCircle(p, centre geom.Vec2, r float64) geom.Vec2 {
	d := p.Sub(centre)
	l := d.Len()
	if l == 0 {
		return p
	}
	return centre.Add(d.Scale(r / l))
}

// solveArc is the centre of the circle of radius r that satisfies both
// ends: tangent to both lines, through one point and tangent to the
// other line, or through both points, on the side of near; ok is false
// when no such circle exists (parallel lines, a point further than 2r
// from the line, a chord longer than 2r).
func solveArc(ends [2]arcEnd, r float64, near geom.Vec2) (geom.Vec2, bool) {
	switch {
	case ends[0].kind == endTangent && ends[1].kind == endTangent:
		return tangentCircle(ends[0].far, ends[0].at, ends[1].at, ends[1].far, r, near)
	case ends[0].kind == endTangent:
		return throughAndTangent(ends[1].at, ends[0].far, ends[0].at, r, near)
	case ends[1].kind == endTangent:
		return throughAndTangent(ends[0].at, ends[1].at, ends[1].far, r, near)
	}
	return centreAt(ends[0].at, ends[1].at, r, near)
}
