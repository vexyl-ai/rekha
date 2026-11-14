package refine

import (
	"math"

	"github.com/vexyl-ai/rekha/internal/geom"
)

const (
	// primAbs is the absolute floor of the primitive guard: a snap may
	// add this much data energy, scaled to the loop, per pixel of the
	// boundary it touches, however small the energy already was. In
	// coverage terms it is primAbs/DataScale² ≈ 0.008 of squared
	// coverage error per boundary pixel. The Pokéball's inner disc
	// (r = 170 px, every sample within 0.3 px of a circle) costs 0.16 per
	// pixel to snap, which the relative clause alone (2 % of an already
	// tiny energy) refused; the wrong candidates that clause rightly
	// rejects cost 4 to 9. The spec started at 1.0; the bench moved it to
	// 0.5 because at 1.0 a snap on a photographic logo (8_1sasa11) stood
	// at 0.95 dB of whole-image loss and at 0.5 it is refused
	// (bench/results/2026-11-14-primitives-v2.md).
	primAbs = 0.5
	// primRelMax caps what the floor may admit: a snap that makes the
	// fit more than this much worse is the wrong model, however little
	// it costs in absolute terms. On a noise-free mask the energies are
	// tiny and a quarter ellipse snapped to a circle costs only 0.13 per
	// pixel while raising the energy tenfold; the Pokéball's rings raise
	// theirs by 5 to 8 %. The spec started at 0.25; the bench moved it to
	// 0.15 with the floor.
	primRelMax = 0.15
)

// withinBudget reports whether a snap that takes the data energy over
// its window from before to after may stand: the rise is within
// primRise of before, or it is within primRelMax of before and, per
// pixel of the length of boundary the snap changes, within primAbs. l0
// is the loop length the energies were divided by (DataEnergy), length
// the polyline length of the changed segments of the loop before the
// snap.
func withinBudget(before, after, l0, length float64) bool {
	rise := after - before
	if rise <= primRise*before {
		return true
	}
	return rise <= primRelMax*before && rise*math.Max(l0, 1)/math.Max(length, 1) <= primAbs
}

// segmentsLength is the polyline length of count segments of b from
// segment from (cyclic; count is capped at the loop), at Flatten chords
// per segment. The count is explicit because a span may cover the whole
// loop, which a (from, to) pair cannot say.
func segmentsLength(b Bezigon, from, count int) float64 {
	n := b.N()
	if n == 0 || count <= 0 {
		return 0
	}
	count = min(count, n)
	var pts []geom.Vec2
	for k := 0; k < count; k++ {
		pts = flattenInto(pts, b.Segment(((from+k)%n+n)%n))
	}
	pts = append(pts, b.Pts[3*(((from+count)%n+n)%n)])
	return polylineLength(pts, false)
}
