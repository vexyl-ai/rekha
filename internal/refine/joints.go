package refine

import (
	"github.com/vexyl-ai/rekha/internal/geom"
)

// endKind says how an arc piece's circle is solved at one of its ends:
// through a point, tangent to the neighbouring line, or free, when the
// end does not constrain the circle and the anchor is moved onto it.
type endKind uint8

const (
	endPoint endKind = iota
	endTangent
	endFree
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
// on. A fixed joint, a joint whose neighbouring segment is frozen (its
// control points may not move), or the joint with an arc neighbour already snapped
// (whose exact cubics the anchor must not disturb), is a point: the
// circle passes through the anchor as it is. A line neighbour is a
// tangent (snapPiece demotes it to free when the tangent circle cannot
// hold the piece's samples, which is what a corner looks like; the turn
// at the joint cannot tell a corner from the kink a refinement leaves at
// a tangent joint). An unsnapped arc or free neighbour, or no neighbour
// because the piece spans the whole loop, is free: the anchor does not
// pin the circle, whose centre then comes from the piece's samples
// (every refined anchor is a little off its circle, and a circle pinned
// through two of them is off by as much all along, which on a clean
// edge is worse than the refinement), and is moved onto the solved
// circle, the neighbour's near handle following so its shape is kept.
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
		case o.fixed(joint) || o.frozen(seg) || m == 1 || (nb.kind == pieceArc && nb.snapped):
		case nb.kind == pieceLine:
			e.kind, e.far = endTangent, far
		default:
			e.kind = endFree
		}
		ends[side] = e
	}
	return ends
}

// demoted is the tangent end e retyped as free, keeping the line it sat
// on so the free end can still be placed where that line meets the
// circle; any other end is returned as it is.
func demoted(e arcEnd) arcEnd {
	if e.kind != endTangent {
		return e
	}
	e.kind = endFree
	return e
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
// ends, the one nearest near (the piece's Kåsa centre) when the ends
// leave a choice: tangent to both lines; through one point and tangent
// to the other line; through both points; tangent to one line, or
// through one point, with the other end free; or near itself when both
// are free. ok is false when no such circle exists (parallel lines, a
// point further than 2r from the line, a chord longer than 2r).
func solveArc(ends [2]arcEnd, r float64, near geom.Vec2) (geom.Vec2, bool) {
	a, c := ends[0], ends[1]
	if a.kind == endFree || (c.kind == endTangent && a.kind != endTangent) {
		a, c = c, a // a is the more constraining end: tangent, then point, then free
	}
	switch {
	case a.kind == endTangent && c.kind == endTangent:
		return tangentCircle(a.far, a.at, c.at, c.far, r, near)
	case a.kind == endTangent && c.kind == endPoint:
		return throughAndTangent(c.at, a.far, a.at, r, near)
	case a.kind == endTangent:
		return tangentOnly(a.far, a.at, r, near)
	case a.kind == endPoint && c.kind == endPoint:
		return centreAt(a.at, c.at, r, near)
	case a.kind == endPoint:
		return throughOnly(a.at, r, near)
	}
	return near, true
}

// tangentOnly is the centre of the circle of radius r tangent to the
// line a–b nearest near: near's projection onto the line of centres.
func tangentOnly(a, b geom.Vec2, r float64, near geom.Vec2) (geom.Vec2, bool) {
	n, ok := lineNormal(a, b, near)
	if !ok {
		return geom.Vec2{}, false
	}
	return near.Add(n.Scale(a.Dot(n) + r - near.Dot(n))), true
}

// throughOnly is the centre of the circle of radius r through p nearest
// near; ok is false when near is p.
func throughOnly(p geom.Vec2, r float64, near geom.Vec2) (geom.Vec2, bool) {
	d := near.Sub(p)
	if d.LenSq() == 0 {
		return geom.Vec2{}, false
	}
	return p.Add(d.Norm().Scale(r)), true
}
