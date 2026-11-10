package refine

import (
	"math"

	"github.com/vexyl-ai/rekha/internal/geom"
)

// kappa is the handle length, as a fraction of the radius, of the
// four-cubic circle approximation (4/3·tan(π/8)).
const kappa = 0.5523

// circleSamples is the number of points per segment the circle fit uses.
const circleSamples = 8

// circleRelTol caps the circle fit tolerance at this fraction of the
// radius: a polygon's deviation from its best circle grows with the
// radius (a square's is 15 % of it), an anti-aliased circle's does not, so
// an absolute tolerance alone takes every small square for a circle.
const circleRelTol = 0.1

// FitCircle fits a circle to the bezigon's outline (Kåsa's algebraic
// least squares on circleSamples points per segment) and reports it when
// the outline has at least three segments and every sample lies within
// min(tol, circleRelTol·r) of the circle.
func FitCircle(b Bezigon, tol float64) (centre geom.Vec2, r float64, ok bool) {
	n := b.N()
	if n < 3 {
		return geom.Vec2{}, 0, false
	}
	pts := make([]geom.Vec2, 0, n*circleSamples)
	for j := 0; j < n; j++ {
		c := b.Segment(j)
		for i := 0; i < circleSamples; i++ {
			pts = append(pts, c.At(float64(i)/circleSamples))
		}
	}
	centre, r, ok = kasa(pts)
	if !ok {
		return geom.Vec2{}, 0, false
	}
	tol = math.Min(tol, circleRelTol*r)
	for _, p := range pts {
		if math.Abs(p.Sub(centre).Len()-r) > tol {
			return geom.Vec2{}, 0, false
		}
	}
	return centre, r, true
}

// SnapCircle replaces b by the four-cubic circle FitCircle finds, from
// b's first anchor and in its direction, unless the rendered data energy
// rises beyond the primitive guard (primRise); ok reports whether it
// did. FitCircle measures the refined outline, so its radius is
// corrected by chordFactor to the circle the mask shows.
func SnapCircle(b Bezigon, m Mask, img Window, tol float64) (Bezigon, bool) {
	c, r, ok := FitCircle(b, tol)
	if !ok {
		return b, false
	}
	cand := Circle(c, r*chordFactor(2*math.Pi/(4*Flatten)), b.Pts[0], polygonArea2(b) > 0)
	if !withinRise(b, cand, m, img, 0, b.N()-1, 0, 3) {
		return b, false
	}
	return cand, true
}

// kasa is Kåsa's algebraic least-squares circle through pts.
func kasa(pts []geom.Vec2) (centre geom.Vec2, r float64, ok bool) {
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
	if det <= 1e-12*(sxx+syy)*(sxx+syy) || !finite(det) {
		return geom.Vec2{}, 0, false
	}
	d := (float64(-sxz*syy) + float64(syz*sxy)) / det
	e := (float64(-syz*sxx) + float64(sxz*sxy)) / det
	f := -sz / float64(len(pts))
	cx, cy := -d/2, -e/2
	r2 := float64(cx*cx) + float64(cy*cy) - f
	if !(r2 > 0) || !finite(r2) {
		return geom.Vec2{}, 0, false
	}
	return geom.Vec2{X: cx + mean.X, Y: cy + mean.Y}, math.Sqrt(r2), true
}

// Circle is the four-cubic circle of the given centre and radius, its
// first anchor at the circle point nearest start, running counter-
// clockwise (positive signed area in y-down coordinates) when ccw.
func Circle(centre geom.Vec2, r float64, start geom.Vec2, ccw bool) Bezigon {
	a0 := 0.0
	if d := start.Sub(centre); d.LenSq() > 0 {
		a0 = math.Atan2(d.Y, d.X)
	}
	step := math.Pi / 2
	if !ccw {
		step = -step
	}
	b := Bezigon{Pts: make([]geom.Vec2, 0, 12)}
	for q := 0; q < 4; q++ {
		a := a0 + float64(float64(q)*step)
		p := geom.Vec2{X: centre.X + float64(r*math.Cos(a)), Y: centre.Y + float64(r*math.Sin(a))}
		next := geom.Vec2{X: centre.X + float64(r*math.Cos(a+step)), Y: centre.Y + float64(r*math.Sin(a+step))}
		// Tangent directions along the direction of travel.
		t0 := geom.Vec2{X: -math.Sin(a), Y: math.Cos(a)}
		t1 := geom.Vec2{X: -math.Sin(a + step), Y: math.Cos(a + step)}
		if !ccw {
			t0, t1 = t0.Scale(-1), t1.Scale(-1)
		}
		b.Pts = append(b.Pts, p, p.Add(t0.Scale(kappa*r)), next.Sub(t1.Scale(kappa*r)))
	}
	return b
}
