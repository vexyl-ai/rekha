package refine

import (
	"math"

	"github.com/vexyl-ai/rekha/internal/fit"
	"github.com/vexyl-ai/rekha/internal/geom"
)

const (
	// ellipseMinRatio is the least ratio of the semi-axes the ellipse
	// snap accepts: under it the shape is a circle, which the circle
	// snap has already judged and declined, and an ellipse a hundredth
	// off round would only take a circle's four arcs (and a DXF CIRCLE)
	// away from it.
	ellipseMinRatio = 1.01
	// ellipseMinAxis is the shortest semi-axis in pixels the snap
	// accepts: below it four cubics cannot be judged against the mask.
	ellipseMinAxis = 4.0
)

// FitEllipse fits an ellipse to the bezigon's outline (the algebraic
// least squares of fit.FitEllipse on circleSamples points per segment)
// and reports it when the outline has at least four segments, the axes
// are in a ratio of at least ellipseMinRatio with the shorter at least
// ellipseMinAxis, and every sample lies within min(tol, circleRelTol·B)
// of the ellipse: the relative cap, as the circle's, keeps a small
// rounded box from being an ellipse at any scale.
func FitEllipse(b Bezigon, tol float64) (fit.Ellipse, bool) {
	n := b.N()
	if n < 4 {
		return fit.Ellipse{}, false
	}
	pts := make([]geom.Vec2, 0, n*circleSamples)
	for j := 0; j < n; j++ {
		c := b.Segment(j)
		for i := 0; i < circleSamples; i++ {
			pts = append(pts, c.At(float64(i)/circleSamples))
		}
	}
	e, ok := fit.FitEllipse(pts)
	if !ok || e.B < ellipseMinAxis || e.A < ellipseMinRatio*e.B {
		return fit.Ellipse{}, false
	}
	lim := math.Min(tol, circleRelTol*e.B)
	for _, p := range pts {
		if e.Dist(p) > lim {
			return fit.Ellipse{}, false
		}
	}
	return e, true
}

// SnapEllipse replaces b by the four-cubic ellipse FitEllipse finds,
// from b's first anchor and in its direction, unless the rendered data
// energy rises beyond the primitive guard (withinBudget); ok reports
// whether it did. As SnapCircle, it corrects the axes for the
// refinement's polygon sag (chordFactor) and then searches one scale on
// both axes against the mask by golden section within radiusSearch; the
// centre and the angle are not searched, since the fit places them to a
// hundredth of a pixel on an outline this smooth and the sag is radial.
func SnapEllipse(b Bezigon, m Mask, img Window, tol float64) (Bezigon, bool) {
	e, ok := FitEllipse(b, tol)
	if !ok {
		return b, false
	}
	ccw := polygonArea2(b) > 0
	sag := chordFactor(2 * math.Pi / float64(b.N()*Flatten))
	e.A *= sag
	e.B *= sag
	l0 := math.Max(b.Length(), 1)
	at := func(s float64) Bezigon {
		return EllipseBezigon(fit.Ellipse{Centre: e.Centre, A: float64(e.A * s), B: float64(e.B * s), Theta: e.Theta}, b.Pts[0], ccw)
	}
	wl := windowAround(img, segmentsBox(b, 0, b.N()-1), segmentsBox(at(1+radiusSearch), 0, 3))
	if wl.W <= 0 || wl.H <= 0 {
		return b, false // nothing to judge the snap by
	}
	energy := func(s float64) float64 { return fineDataEnergy(at(s), m, wl, l0) }
	best, eBest := 1.0, energy(1)
	const phi = 0.6180339887498949
	lo, hi := 1-radiusSearch, 1+radiusSearch
	x1, x2 := hi-float64(phi*(hi-lo)), lo+float64(phi*(hi-lo))
	f1, f2 := energy(x1), energy(x2)
	for i := 0; i < radiusSteps; i++ {
		if f1 < f2 {
			hi, x2, f2 = x2, x1, f1
			x1 = hi - float64(phi*(hi-lo))
			f1 = energy(x1)
		} else {
			lo, x1, f1 = x1, x2, f2
			x2 = lo + float64(phi*(hi-lo))
			f2 = energy(x2)
		}
	}
	for _, t := range [][2]float64{{x1, f1}, {x2, f2}} {
		if t[1] < eBest {
			best, eBest = t[0], t[1]
		}
	}
	if !withinBudget(fineDataEnergy(b, m, wl, l0), eBest, l0, l0) {
		return b, false
	}
	return at(best), true
}

// EllipseBezigon is the four-cubic ellipse: the four-cubic circle
// (Circle) under the affine map of the unit circle onto e, its first
// anchor at the ellipse point whose parametric angle is nearest start,
// running counter-clockwise (positive signed area in y-down
// coordinates) when ccw. Bézier curves are affine-invariant, so the
// circle approximation's relative error of 2.7e-4 carries over, bounded
// by 2.7e-4·A.
func EllipseBezigon(e fit.Ellipse, start geom.Vec2, ccw bool) Bezigon {
	cs, sn := math.Cos(e.Theta), math.Sin(e.Theta)
	// The frame coordinates of start give its parametric angle.
	d := start.Sub(e.Centre)
	fx, fy := float64(d.X*cs)+float64(d.Y*sn), float64(-d.X*sn)+float64(d.Y*cs)
	a0 := 0.0
	if fx != 0 || fy != 0 {
		a0 = math.Atan2(fy/e.B, fx/e.A)
	}
	step := math.Pi / 2
	if !ccw {
		step = -step
	}
	b := Bezigon{Pts: make([]geom.Vec2, 0, 12)}
	for q := 0; q < 4; q++ {
		a := a0 + float64(float64(q)*step)
		p := e.At(a)
		next := e.At(a + step)
		// Derivatives along the direction of travel, scaled by kappa as
		// the circle's unit tangents are.
		t0 := ellipseDeriv(e, a)
		t1 := ellipseDeriv(e, a+step)
		if !ccw {
			t0, t1 = t0.Scale(-1), t1.Scale(-1)
		}
		b.Pts = append(b.Pts, p, p.Add(t0.Scale(kappa)), next.Sub(t1.Scale(kappa)))
	}
	return b
}

// ellipseDeriv is dE/dphi at phi in image coordinates, whose length is
// the radius's worth of handle the affine map gives a unit tangent.
func ellipseDeriv(e fit.Ellipse, phi float64) geom.Vec2 {
	cs, sn := math.Cos(e.Theta), math.Sin(e.Theta)
	qx, qy := float64(-e.A*math.Sin(phi)), float64(e.B*math.Cos(phi))
	return geom.Vec2{X: float64(qx*cs) - float64(qy*sn), Y: float64(qx*sn) + float64(qy*cs)}
}
