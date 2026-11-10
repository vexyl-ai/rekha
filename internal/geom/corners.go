package geom

import "math"

// cornerArm is the arc length, in pixels, over which the incoming and
// outgoing directions at a vertex are measured. Single-pixel jogs left by
// digitisation would otherwise read as sharp turns.
const cornerArm = 3.0

// Corners returns the indices of vertices of a closed polygon whose
// absolute turn angle exceeds thresholdDeg, in ascending order. The turn is
// measured between the direction from the point at least cornerArm behind
// the vertex and the direction to the point at least cornerArm ahead.
func Corners(poly []Vec2, thresholdDeg float64) []int {
	var out []int
	for i := range poly {
		if Turn(poly, i) > thresholdDeg {
			out = append(out, i)
		}
	}
	return out
}

// Turn is the absolute turn, in degrees, at vertex i of a closed polygon,
// measured over arms of at least cornerArm on each side.
func Turn(poly []Vec2, i int) float64 {
	cur := poly[i]
	a := cur.Sub(armPoint(poly, i, -1))
	b := armPoint(poly, i, +1).Sub(cur)
	return math.Abs(math.Atan2(Cross(a, b), a.Dot(b))) * 180 / math.Pi
}

// armPoint walks from vertex i in direction step (±1) until the walked arc
// length reaches cornerArm, or half the polygon has been traversed, and
// returns the vertex reached.
func armPoint(poly []Vec2, i, step int) Vec2 {
	n := len(poly)
	dist := 0.0
	j := i
	for k := 0; k < n/2; k++ {
		next := (j + step + n) % n
		dist += poly[next].Sub(poly[j]).Len()
		j = next
		if dist >= cornerArm {
			break
		}
	}
	return poly[j]
}

// TurnOpen is Turn for an open polyline: the arms stop at its ends, and
// the ends themselves turn by zero.
func TurnOpen(poly []Vec2, i int) float64 {
	cur := poly[i]
	a := cur.Sub(armPointOpen(poly, i, -1))
	b := armPointOpen(poly, i, +1).Sub(cur)
	if a == (Vec2{}) || b == (Vec2{}) {
		return 0
	}
	return math.Abs(math.Atan2(Cross(a, b), a.Dot(b))) * 180 / math.Pi
}

// armPointOpen is armPoint without wrapping: the walk stops at the
// polyline's end.
func armPointOpen(poly []Vec2, i, step int) Vec2 {
	dist := 0.0
	j := i
	for j+step >= 0 && j+step < len(poly) && dist < cornerArm {
		dist += poly[j+step].Sub(poly[j]).Len()
		j += step
	}
	return poly[j]
}
