package refine

import "math"

// Optimiser constants: finite-difference step in pixels, L-BFGS memory,
// Armijo constant and the two stopping tolerances (relative energy
// decrease and largest coordinate move in pixels).
const (
	fdStep      = 0.01
	lbfgsMemory = 5
	armijoC     = 1e-4
	maxHalvings = 20
	relStop     = 1e-6
	moveStop    = 0.005
)

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// gradient is the central finite-difference gradient of f at x; a
// coordinate whose perturbed values are not finite gets zero slope.
func gradient(x []float64, f func([]float64) float64, g []float64) {
	for i := range x {
		xi := x[i]
		x[i] = xi + fdStep
		fp := f(x)
		x[i] = xi - fdStep
		fm := f(x)
		x[i] = xi
		if finite(fp) && finite(fm) {
			g[i] = (fp - fm) / (2 * fdStep)
		} else {
			g[i] = 0
		}
	}
}

// minimize runs L-BFGS with a finite-difference gradient and Armijo
// backtracking from x0 and returns the best point found: never one with a
// higher f than x0, and never a non-finite one. ok, if not nil, says
// whether the step from x to xn is admissible; the line search halves the
// step until it is. It keeps iterates on this side of a barrier that a
// long step could otherwise jump over without ever evaluating it.
func minimize(x0 []float64, f func([]float64) float64, ok func(x, xn []float64) bool, maxIter int) []float64 {
	n := len(x0)
	x := append([]float64(nil), x0...)
	fx := f(x)
	if !finite(fx) {
		return x0
	}
	g := make([]float64, n)
	gradient(x, f, g)
	var sHist, yHist [][]float64
	d := make([]float64, n)
	xn := make([]float64, n)
	gn := make([]float64, n)
	for it := 0; it < maxIter; it++ {
		direction(g, sHist, yHist, d)
		if dot(d, g) >= 0 { // not a descent direction: fall back to steepest descent
			for i := range d {
				d[i] = -g[i]
			}
		}
		gd := dot(g, d)
		if gd == 0 {
			break
		}
		// Start with a step that moves no coordinate more than one pixel.
		step := 1.0
		if m := maxAbs(d); m > 1 {
			step = 1 / m
		}
		accepted := false
		var fn float64
		halvings := 0
		for ; halvings < maxHalvings; halvings++ {
			for i := range x {
				xn[i] = x[i] + step*d[i]
			}
			if ok != nil && !ok(x, xn) {
				step /= 2
				continue
			}
			fn = f(xn)
			if finite(fn) && fn <= fx+armijoC*step*gd {
				accepted = true
				break
			}
			step /= 2
		}
		if !accepted {
			break
		}
		gradient(xn, f, gn)
		s, y := make([]float64, n), make([]float64, n)
		for i := range s {
			s[i] = xn[i] - x[i]
			y[i] = gn[i] - g[i]
		}
		if dot(s, y) > 1e-12 {
			sHist, yHist = append(sHist, s), append(yHist, y)
			if len(sHist) > lbfgsMemory {
				sHist, yHist = sHist[1:], yHist[1:]
			}
		}
		rel := (fx - fn) / math.Max(math.Abs(fx), 1e-12)
		move := maxAbs(s)
		copy(x, xn)
		copy(g, gn)
		fx = fn
		// A small decrease after backtracking says the trial step was too
		// long, not that the minimum is near; only an unshortened step that
		// barely helps, or a step that barely moves, ends the search.
		if move < moveStop || (rel < relStop && halvings == 0) {
			break
		}
	}
	return x
}

// direction computes d = -H·g by the L-BFGS two-loop recursion.
func direction(g []float64, sHist, yHist [][]float64, d []float64) {
	q := append([]float64(nil), g...)
	k := len(sHist)
	alpha := make([]float64, k)
	for i := k - 1; i >= 0; i-- {
		rho := 1 / dot(yHist[i], sHist[i])
		alpha[i] = rho * dot(sHist[i], q)
		axpy(q, -alpha[i], yHist[i])
	}
	if k > 0 {
		gamma := dot(sHist[k-1], yHist[k-1]) / dot(yHist[k-1], yHist[k-1])
		for i := range q {
			q[i] *= gamma
		}
	}
	for i := 0; i < k; i++ {
		rho := 1 / dot(yHist[i], sHist[i])
		beta := rho * dot(yHist[i], q)
		axpy(q, alpha[i]-beta, sHist[i])
	}
	for i := range d {
		d[i] = -q[i]
	}
}

func dot(a, b []float64) float64 {
	s := 0.0
	for i := range a {
		s += a[i] * b[i]
	}
	return s
}

// axpy does y += a·x.
func axpy(y []float64, a float64, x []float64) {
	for i := range y {
		y[i] += a * x[i]
	}
}

func maxAbs(v []float64) float64 {
	m := 0.0
	for _, x := range v {
		m = math.Max(m, math.Abs(x))
	}
	return m
}
