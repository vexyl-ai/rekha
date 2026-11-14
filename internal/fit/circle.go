package fit

// Products in this file are wrapped in float64() so that arm64 does not
// fuse them into FMAs and drift from amd64: the goldens are shared.

import (
	"math"

	"github.com/vexyl-ai/rekha/internal/geom"
)

// Kasa is Kåsa's algebraic least-squares circle through pts; ok is
// false with fewer than three points or collinear ones.
func Kasa(pts []geom.Vec2) (centre geom.Vec2, r float64, ok bool) {
	if len(pts) < 3 {
		return geom.Vec2{}, 0, false
	}
	// Centre the data on its mean for conditioning, then solve the
	// normal equations of x² + y² + D·x + E·y + F = 0.
	var mean geom.Vec2
	for _, p := range pts {
		mean = mean.Add(p)
	}
	mean = mean.Scale(1 / float64(len(pts)))
	var sxx, sxy, syy, sxz, syz, sz float64
	for _, p := range pts {
		x, y := p.X-mean.X, p.Y-mean.Y
		z := float64(x*x) + float64(y*y)
		sxx += float64(x * x)
		sxy += float64(x * y)
		syy += float64(y * y)
		sxz += float64(x * z)
		syz += float64(y * z)
		sz += z
	}
	// With centred data Σx = Σy = 0, so D and E decouple from F:
	// [sxx sxy; sxy syy]·[D E]ᵀ = -[sxz syz]ᵀ, F = -(sz + 0)/m.
	det := float64(sxx*syy) - float64(sxy*sxy)
	if det <= 1e-12*(sxx+syy)*(sxx+syy) || math.IsNaN(det) || math.IsInf(det, 0) {
		return geom.Vec2{}, 0, false
	}
	d := (float64(-sxz*syy) + float64(syz*sxy)) / det
	e := (float64(-syz*sxx) + float64(sxz*sxy)) / det
	f := -sz / float64(len(pts))
	cx, cy := -d/2, -e/2
	r2 := float64(cx*cx) + float64(cy*cy) - f
	if !(r2 > 0) || math.IsNaN(r2) || math.IsInf(r2, 0) {
		return geom.Vec2{}, 0, false
	}
	return geom.Vec2{X: cx + mean.X, Y: cy + mean.Y}, math.Sqrt(r2), true
}

// CircleGeometric is the least-squares circle through pts in the
// geometric sense: the one minimising the sum of squared radial
// distances, found by Gauss-Newton from Kåsa's algebraic fit, which on
// a short arc of noisy points underestimates the radius (crack points
// on 50° of an r = 80 circle give Kåsa r = 74). ok is false when Kåsa
// fails or the iteration does not settle.
func CircleGeometric(pts []geom.Vec2) (centre geom.Vec2, r float64, ok bool) {
	centre, r, ok = Kasa(pts)
	if !ok {
		return
	}
	for iter := 0; iter < 20; iter++ {
		// Normal equations JᵀJ·δ = -Jᵀres for δ = (dcx, dcy, dr), with
		// res_i = |p_i - c| - r and J_i = (-(x_i-cx)/d_i, -(y_i-cy)/d_i, -1).
		var a [3][3]float64
		var g [3]float64
		for _, p := range pts {
			dx, dy := p.X-centre.X, p.Y-centre.Y
			d := math.Hypot(dx, dy)
			if d == 0 {
				continue
			}
			j := [3]float64{-dx / d, -dy / d, -1}
			res := d - r
			for u := 0; u < 3; u++ {
				g[u] -= float64(j[u] * res)
				for v := 0; v < 3; v++ {
					a[u][v] += float64(j[u] * j[v])
				}
			}
		}
		delta, solved := solve3(a, g)
		if !solved {
			return centre, r, false
		}
		centre.X += delta[0]
		centre.Y += delta[1]
		r += delta[2]
		if !(r > 0) || math.IsNaN(r) {
			return centre, r, false
		}
		if math.Abs(delta[0])+math.Abs(delta[1])+math.Abs(delta[2]) < 1e-7 {
			return centre, r, true
		}
	}
	return centre, r, true
}

// solve3 solves the 3×3 system a·x = g by Gaussian elimination with
// partial pivoting; ok is false when a is singular.
func solve3(a [3][3]float64, g [3]float64) ([3]float64, bool) {
	for col := 0; col < 3; col++ {
		piv := col
		for row := col + 1; row < 3; row++ {
			if math.Abs(a[row][col]) > math.Abs(a[piv][col]) {
				piv = row
			}
		}
		if math.Abs(a[piv][col]) < 1e-12 {
			return [3]float64{}, false
		}
		a[col], a[piv] = a[piv], a[col]
		g[col], g[piv] = g[piv], g[col]
		for row := col + 1; row < 3; row++ {
			f := a[row][col] / a[col][col]
			for k := col; k < 3; k++ {
				a[row][k] -= float64(f * a[col][k])
			}
			g[row] -= float64(f * g[col])
		}
	}
	var x [3]float64
	for row := 2; row >= 0; row-- {
		s := g[row]
		for k := row + 1; k < 3; k++ {
			s -= float64(a[row][k] * x[k])
		}
		x[row] = s / a[row][row]
	}
	return x, true
}

// LineCircle is where the line p+t·d (d unit) meets the circle: two
// points, or none.
func LineCircle(p, d, centre geom.Vec2, r float64) []geom.Vec2 {
	foot := p.Add(d.Scale(centre.Sub(p).Dot(d)))
	h2 := float64(r*r) - foot.Sub(centre).LenSq()
	if h2 < 0 {
		return nil
	}
	h := math.Sqrt(h2)
	return []geom.Vec2{foot.Add(d.Scale(h)), foot.Sub(d.Scale(h))}
}

// CircleCircle is where two circles meet: two points, or none.
func CircleCircle(c0 geom.Vec2, r0 float64, c1 geom.Vec2, r1 float64) []geom.Vec2 {
	dv := c1.Sub(c0)
	dist := dv.Len()
	if dist == 0 || dist > r0+r1 || dist < math.Abs(r0-r1) {
		return nil
	}
	a := (float64(r0*r0) - float64(r1*r1) + float64(dist*dist)) / (2 * dist)
	h2 := float64(r0*r0) - float64(a*a)
	if h2 < 0 {
		return nil
	}
	h := math.Sqrt(h2)
	u := dv.Scale(1 / dist)
	mid := c0.Add(u.Scale(a))
	n := geom.Vec2{X: -u.Y, Y: u.X}
	return []geom.Vec2{mid.Add(n.Scale(h)), mid.Sub(n.Scale(h))}
}
