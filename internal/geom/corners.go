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
	n := len(poly)
	var out []int
	for i := 0; i < n; i++ {
		cur := poly[i]
		a := cur.Sub(armPoint(poly, i, -1))
		b := armPoint(poly, i, +1).Sub(cur)
		cross := Cross(a, b)
		dot := a.Dot(b)
		deg := math.Abs(math.Atan2(cross, dot)) * 180 / math.Pi
		if deg > thresholdDeg {
			out = append(out, i)
		}
	}
	return out
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
