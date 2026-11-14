package fit

// Products in this file are wrapped in float64() so that arm64 does not
// fuse them into FMAs and drift from amd64: the goldens are shared.

import (
	"math"

	"github.com/vexyl-ai/rekha/internal/geom"
)

// Ellipse is an ellipse by centre, semi-axes A ≥ B and the direction
// Theta of the A axis, in (−π/2, π/2].
type Ellipse struct {
	Centre geom.Vec2
	A, B   float64
	Theta  float64
}

// ellipseMinPoints is the fewest points FitEllipse accepts: a conic has
// five coefficients, so five points fit any conic exactly and say
// nothing about whether the shape is one.
const ellipseMinPoints = 6

// FitEllipse is the algebraic least-squares ellipse through pts: the
// conic a·x² + b·xy + c·y² + d·x + e·y = 1 on the data centred on its
// mean (the mean of a closed outline lies inside it, so the constant
// term is safely fixed), solved by the normal equations. ok is false
// with fewer than ellipseMinPoints points, a singular system (collinear
// points), a conic that is not an ellipse (b² − 4ac ≥ 0, or a centre
// the curve does not enclose) or axes that are not finite. The fit is
// biased on noisy short arcs; on a smooth closed outline a fraction of
// a pixel off the curve the bias is far below any tolerance used here.
func FitEllipse(pts []geom.Vec2) (Ellipse, bool) {
	if len(pts) < ellipseMinPoints {
		return Ellipse{}, false
	}
	var mean geom.Vec2
	for _, p := range pts {
		mean = mean.Add(p)
	}
	mean = mean.Scale(1 / float64(len(pts)))
	var m [5][5]float64
	var g [5]float64
	for _, p := range pts {
		x, y := p.X-mean.X, p.Y-mean.Y
		row := [5]float64{float64(x * x), float64(x * y), float64(y * y), x, y}
		for i := 0; i < 5; i++ {
			for j := 0; j < 5; j++ {
				m[i][j] += float64(row[i] * row[j])
			}
			g[i] += row[i]
		}
	}
	co, ok := solve5(m, g)
	if !ok {
		return Ellipse{}, false
	}
	a, b, c, d, e := co[0], co[1], co[2], co[3], co[4]
	disc := float64(4*a*c) - float64(b*b)
	if !(disc > 0) {
		return Ellipse{}, false
	}
	// The centre is where the gradient vanishes; the conic there is
	// a(x−x0)² + b(x−x0)(y−y0) + c(y−y0)² = k.
	x0 := (float64(b*e) - float64(2*c*d)) / disc
	y0 := (float64(b*d) - float64(2*a*e)) / disc
	k := 1 - (float64(a*x0*x0) + float64(b*x0*y0) + float64(c*y0*y0) + float64(d*x0) + float64(e*y0))
	if !(k > 0) {
		return Ellipse{}, false
	}
	// Eigenvalues of [[a, b/2], [b/2, c]] / k; the longer axis is the
	// eigenvector of the smaller one.
	half, diff := (a+c)/2, (a-c)/2
	rad := math.Sqrt(float64(diff*diff) + float64(b*b)/4)
	l1, l2 := (half-rad)/k, (half+rad)/k
	if !(l1 > 0) {
		return Ellipse{}, false
	}
	theta := 0.5*math.Atan2(b, a-c) + math.Pi/2
	for theta > math.Pi/2 {
		theta -= math.Pi
	}
	for theta <= -math.Pi/2 {
		theta += math.Pi
	}
	el := Ellipse{Centre: geom.Vec2{X: x0 + mean.X, Y: y0 + mean.Y}, A: 1 / math.Sqrt(l1), B: 1 / math.Sqrt(l2), Theta: theta}
	if math.IsNaN(el.A) || math.IsInf(el.A, 0) || math.IsNaN(el.B) || math.IsInf(el.B, 0) {
		return Ellipse{}, false
	}
	return el, true
}

// At is the point at parametric angle phi.
func (e Ellipse) At(phi float64) geom.Vec2 {
	return e.toImage(geom.Vec2{X: float64(e.A * math.Cos(phi)), Y: float64(e.B * math.Sin(phi))})
}

// toImage maps a point in the ellipse's frame (centre at the origin, the
// A axis along x) to image coordinates.
func (e Ellipse) toImage(q geom.Vec2) geom.Vec2 {
	cs, sn := math.Cos(e.Theta), math.Sin(e.Theta)
	return geom.Vec2{X: e.Centre.X + float64(q.X*cs) - float64(q.Y*sn), Y: e.Centre.Y + float64(q.X*sn) + float64(q.Y*cs)}
}

// toFrame is the inverse of toImage.
func (e Ellipse) toFrame(p geom.Vec2) geom.Vec2 {
	cs, sn := math.Cos(e.Theta), math.Sin(e.Theta)
	d := p.Sub(e.Centre)
	return geom.Vec2{X: float64(d.X*cs) + float64(d.Y*sn), Y: float64(-d.X*sn) + float64(d.Y*cs)}
}

// ellipseDistSteps is the Newton step count of Dist.
const ellipseDistSteps = 8

// Dist is the distance from p to the ellipse: Newton on the parametric
// angle, from the angle of p in the ellipse's frame, which converges
// for points within a few per cent of the curve, the only points the
// callers ask about.
func (e Ellipse) Dist(p geom.Vec2) float64 {
	q := e.toFrame(p)
	phi := math.Atan2(q.Y/e.B, q.X/e.A)
	for i := 0; i < ellipseDistSteps; i++ {
		cs, sn := math.Cos(phi), math.Sin(phi)
		ex, ey := float64(e.A*cs)-q.X, float64(e.B*sn)-q.Y // E(phi) − q
		dx, dy := float64(-e.A*sn), float64(e.B*cs)        // E'(phi)
		g := float64(ex*dx) + float64(ey*dy)
		gp := float64(dx*dx) + float64(dy*dy) - float64(ex*e.A*cs) - float64(ey*e.B*sn)
		if gp <= 0 {
			break
		}
		step := g / gp
		if step > 0.5 {
			step = 0.5
		} else if step < -0.5 {
			step = -0.5
		}
		phi -= step
	}
	return geom.Vec2{X: float64(e.A*math.Cos(phi)) - q.X, Y: float64(e.B*math.Sin(phi)) - q.Y}.Len()
}

// solve5 solves the 5×5 system a·x = g by Gaussian elimination with
// partial pivoting; ok is false when a is singular, judged against the
// scale of its entries.
func solve5(a [5][5]float64, g [5]float64) ([5]float64, bool) {
	scale := 0.0
	for i := range a {
		for j := range a[i] {
			scale = math.Max(scale, math.Abs(a[i][j]))
		}
	}
	for col := 0; col < 5; col++ {
		piv := col
		for row := col + 1; row < 5; row++ {
			if math.Abs(a[row][col]) > math.Abs(a[piv][col]) {
				piv = row
			}
		}
		if math.Abs(a[piv][col]) <= 1e-10*scale {
			return [5]float64{}, false
		}
		a[col], a[piv] = a[piv], a[col]
		g[col], g[piv] = g[piv], g[col]
		for row := col + 1; row < 5; row++ {
			f := a[row][col] / a[col][col]
			for k := col; k < 5; k++ {
				a[row][k] -= float64(f * a[col][k])
			}
			g[row] -= float64(f * g[col])
		}
	}
	var x [5]float64
	for row := 4; row >= 0; row-- {
		s := g[row]
		for k := row + 1; k < 5; k++ {
			s -= float64(a[row][k] * x[k])
		}
		x[row] = s / a[row][row]
	}
	return x, true
}
