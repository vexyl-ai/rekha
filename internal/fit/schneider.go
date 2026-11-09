package fit

import (
	"math"

	"github.com/vexyl-ai/rekha/internal/geom"
)

const (
	maxIterations = 12
	minFitPoints  = 4
	// tangentWindow is the least number of points on each side that feed
	// a tangent estimate. Traced pixel boundaries zigzag, so adjacent
	// points alone give directions quantised to multiples of 45°. The
	// window grows with the run (see halfWindow): a tangent error of θ
	// moves a cubic by about θ times its handle length, so long runs need
	// proportionally better tangents.
	tangentWindow    = 8
	maxTangentWindow = 128
	// biasTol is the largest mean signed offset, in pixels, a fit may have
	// from its points. The maximum-error test alone lets a curve run down
	// one side of a run as long as it stays within tolerance, and on a
	// feature thinner than twice the tolerance both sides then fit the
	// same curve and the feature vanishes. Crack points straddle the true
	// edge by half a pixel, so a chord through two of them can sit half a
	// pixel from the edge while being faithful; the cap is that plus
	// slack, the same band maxStairDev uses in geom.
	biasTol = 0.75
)

// FitOpen fits pts with the given unit end tangents: t0 points into the
// curve at pts[0], t1 points into the curve at pts[len-1]. maxErr is the
// largest allowed point-to-curve distance. Fewer than four points give one
// straight segment.
func FitOpen(pts []geom.Vec2, t0, t1 geom.Vec2, maxErr float64) []Segment {
	if len(pts) < 2 {
		return nil
	}
	var out []Segment
	fitCubic(pts, t0, t1, maxErr*maxErr, &out)
	return out
}

// FitRun fits a run between two corners. End tangents are estimated by a
// least-squares line through the first and last k points,
// k = max(2, min(4, len/3)).
func FitRun(pts []geom.Vec2, maxErr float64) []Segment {
	n := len(pts)
	if n < minFitPoints {
		if n < 2 {
			return nil
		}
		return []Segment{line(pts[0], pts[n-1])}
	}
	k := max(2, min(4, n/3))
	t0 := lineDirection(pts[:k])
	t1 := lineDirection(reverse(pts[n-k:]))
	return FitOpen(pts, t0, t1, maxErr)
}

// FitClosed fits a loop with no corners. It splits at the point farthest
// from the centroid, estimates one tangent there from both neighbours, and
// fits the loop as a single run so the join is G1.
func FitClosed(pts []geom.Vec2, maxErr float64) []Segment {
	n := len(pts)
	if n < 3 {
		return nil
	}
	var c geom.Vec2
	for _, p := range pts {
		c = c.Add(p)
	}
	c = c.Scale(1 / float64(n))
	far, farD := 0, -1.0
	for i, p := range pts {
		if d := p.Sub(c).Len(); d > farD {
			far, farD = i, d
		}
	}
	start := smoothPoint(pts, far, true)
	run := make([]geom.Vec2, 0, n+1)
	run = append(run, start)
	for i := 1; i < n; i++ {
		run = append(run, pts[(far+i)%n])
	}
	run = append(run, start)
	k := min(halfWindow(n), n/2)
	window := make([]geom.Vec2, 0, 2*k+1)
	for i := -k; i <= k; i++ {
		window = append(window, pts[(far+i+n)%n])
	}
	t := lineDirection(window)
	if n+1 < minFitPoints {
		return []Segment{line(run[0], run[n])}
	}
	return FitOpen(run, t, t.Scale(-1), maxErr)
}

func reverse(p []geom.Vec2) []geom.Vec2 {
	out := make([]geom.Vec2, len(p))
	for i, v := range p {
		out[len(p)-1-i] = v
	}
	return out
}

// lineDirection returns the unit direction of the least-squares line
// through pts (principal axis), oriented from pts[0] toward the last point.
// halfWindow is the number of points on each side of a point used to
// estimate its tangent in a run of n points: a sixth of the run, within
// [tangentWindow, maxTangentWindow].
func halfWindow(n int) int {
	return max(tangentWindow, min(n/6, maxTangentWindow))
}

// centerTangent estimates the forward tangent at pts[i] from halfWindow
// neighbours on each side.
func centerTangent(pts []geom.Vec2, i int) geom.Vec2 {
	k := min(halfWindow(len(pts)), i, len(pts)-1-i)
	if k < 1 {
		k = 1
	}
	lo, hi := max(i-k, 0), min(i+k, len(pts)-1)
	t := lineDirection(pts[lo : hi+1])
	if t == (geom.Vec2{}) {
		t = pts[hi].Sub(pts[lo]).Norm()
	}
	return t
}

func lineDirection(pts []geom.Vec2) geom.Vec2 {
	var mean geom.Vec2
	for _, p := range pts {
		mean = mean.Add(p)
	}
	mean = mean.Scale(1 / float64(len(pts)))
	var sxx, sxy, syy float64
	for _, p := range pts {
		d := p.Sub(mean)
		sxx += d.X * d.X
		sxy += d.X * d.Y
		syy += d.Y * d.Y
	}
	// Principal eigenvector of [[sxx sxy][sxy syy]].
	var dir geom.Vec2
	if sxy == 0 && sxx == 0 && syy == 0 {
		dir = pts[len(pts)-1].Sub(pts[0])
	} else {
		theta := 0.5 * math.Atan2(2*sxy, sxx-syy)
		dir = geom.Vec2{X: math.Cos(theta), Y: math.Sin(theta)}
	}
	if dir.Dot(pts[len(pts)-1].Sub(pts[0])) < 0 {
		dir = dir.Scale(-1)
	}
	return dir.Norm()
}

func fitCubic(pts []geom.Vec2, t0, t1 geom.Vec2, errSq float64, out *[]Segment) {
	n := len(pts)
	if n < minFitPoints {
		*out = append(*out, line(pts[0], pts[n-1]))
		return
	}
	if straight(pts, errSq) {
		*out = append(*out, line(pts[0], pts[n-1]))
		return
	}
	// Least squares under chord-length parameterisation, then Newton
	// reparameterisation to convergence. Schneider accepts the first
	// iterate within tolerance and gates the iterations on the error being
	// within 4× tolerance; an early iterate is systematically biased (on a
	// digitised circle it dips half a pixel inward), so we keep iterating
	// while the fit improves and accept the best one.
	u := chordLength(pts)
	bez := generateBezier(pts, u, t0, t1)
	maxSq, worst, bias := maxError(pts, bez, u)
	best, bestSq, bestWorst, bestBias := bez, maxSq, worst, bias
	for i := 0; i < maxIterations; i++ {
		u = reparameterize(pts, u, bez)
		bez = generateBezier(pts, u, t0, t1)
		maxSq, worst, bias = maxError(pts, bez, u)
		if maxSq < bestSq {
			best, bestSq, bestWorst, bestBias = bez, maxSq, worst, bias
		} else if maxSq > bestSq*1.01 {
			break
		}
	}
	if bestSq <= errSq && bestBias <= biasTol {
		*out = append(*out, Segment{P: best})
		return
	}
	// Split with a shared centre tangent. Schneider splits at the worst
	// point, but on noisy pixel boundaries that lands anywhere and
	// shatters the run into tiny pieces, so balanced halves are the rule.
	// When the worst point sits in the outer tenth of the run the error
	// is local to that end (typically a corner placed off the edge), and
	// splitting there keeps the rest of the run whole.
	split := midpoint(pts)
	if edge := max(2, n/10); bestWorst < edge || bestWorst >= n-edge {
		split = max(2, min(bestWorst, n-3))
	}
	tc := centerTangent(pts, split).Scale(-1) // points "backwards" at split
	// The shared endpoint is the local mean, not the raw crack vertex, so
	// the joint sits on the edge rather than on a pixel corner.
	mid := smoothPoint(pts, split, false)
	left := append(append(make([]geom.Vec2, 0, split+1), pts[:split]...), mid)
	right := append([]geom.Vec2{mid}, pts[split+1:]...)
	fitCubic(left, t0, tc, errSq, out)
	fitCubic(right, tc.Scale(-1), t1, errSq, out)
}

// straight reports whether every point lies within sqrt(errSq) of the
// chord from the first point to the last, and the points' mean signed
// offset from the chord is within biasTol.
func straight(pts []geom.Vec2, errSq float64) bool {
	a, b := pts[0], pts[len(pts)-1]
	ab := b.Sub(a)
	l2 := ab.Dot(ab)
	sum := 0.0
	for _, p := range pts[1 : len(pts)-1] {
		ap := p.Sub(a)
		var d2 float64
		if l2 == 0 {
			d2 = ap.Dot(ap)
		} else {
			cross := ab.X*ap.Y - ab.Y*ap.X
			d2 = cross * cross / l2
			sum += cross / math.Sqrt(l2)
		}
		if d2 > errSq {
			return false
		}
	}
	return math.Abs(sum)/float64(len(pts)-2) <= biasTol
}

// midpoint returns the index of the point nearest half the run's chord
// length, clamped so both halves keep at least two points.
func midpoint(pts []geom.Vec2) int {
	u := chordLength(pts)
	i := 0
	for i < len(u)-1 && u[i] < 0.5 {
		i++
	}
	return max(1, min(i, len(pts)-2))
}

func chordLength(pts []geom.Vec2) []float64 {
	u := make([]float64, len(pts))
	for i := 1; i < len(pts); i++ {
		u[i] = u[i-1] + pts[i].Sub(pts[i-1]).Len()
	}
	total := u[len(u)-1]
	if total == 0 {
		for i := range u {
			u[i] = float64(i) / float64(len(u)-1)
		}
		return u
	}
	for i := range u {
		u[i] /= total
	}
	return u
}

func bern(t float64) (b0, b1, b2, b3 float64) {
	mt := 1 - t
	return mt * mt * mt, 3 * mt * mt * t, 3 * mt * t * t, t * t * t
}

// generateBezier solves the 2×2 least-squares system for the two handle
// lengths, falling back to a third of the chord when it is degenerate.
func generateBezier(pts []geom.Vec2, u []float64, t0, t1 geom.Vec2) Cubic {
	first, last := pts[0], pts[len(pts)-1]
	var c00, c01, c11, x0, x1 float64
	for i, p := range pts {
		b0, b1, b2, b3 := bern(u[i])
		a0 := t0.Scale(b1)
		a1 := t1.Scale(b2)
		c00 += a0.Dot(a0)
		c01 += a0.Dot(a1)
		c11 += a1.Dot(a1)
		tmp := p.Sub(first.Scale(b0 + b1)).Sub(last.Scale(b2 + b3))
		x0 += a0.Dot(tmp)
		x1 += a1.Dot(tmp)
	}
	det := c00*c11 - c01*c01
	var al, ar float64
	if math.Abs(det) > 1e-12 {
		al = (x0*c11 - x1*c01) / det
		ar = (c00*x1 - c01*x0) / det
	}
	seg := last.Sub(first).Len()
	eps := 1e-6 * seg
	if al < eps || ar < eps {
		al, ar = seg/3, seg/3
	}
	return Cubic{first, first.Add(t0.Scale(al)), last.Add(t1.Scale(ar)), last}
}

func maxError(pts []geom.Vec2, bez Cubic, u []float64) (maxSq float64, worst int, bias float64) {
	worst = len(pts) / 2
	sum := 0.0
	for i := 1; i < len(pts)-1; i++ {
		d := pts[i].Sub(bez.At(u[i]))
		if sq := d.Dot(d); sq > maxSq {
			maxSq, worst = sq, i
		}
		// Signed offset along the curve normal.
		if tan := bez.d1(u[i]); tan != (geom.Vec2{}) {
			sum += (tan.X*d.Y - tan.Y*d.X) / tan.Len()
		}
	}
	if len(pts) > 2 {
		bias = math.Abs(sum) / float64(len(pts)-2)
	}
	return maxSq, worst, bias
}

// reparameterize performs one Newton–Raphson step per point.
func reparameterize(pts []geom.Vec2, u []float64, bez Cubic) []float64 {
	out := make([]float64, len(u))
	for i, p := range pts {
		t := u[i]
		q := bez.At(t).Sub(p)
		q1 := bez.d1(t)
		q2 := bez.d2(t)
		den := q1.Dot(q1) + q.Dot(q2)
		if math.Abs(den) < 1e-12 {
			out[i] = t
			continue
		}
		nt := t - q.Dot(q1)/den
		out[i] = math.Max(0, math.Min(1, nt))
	}
	return out
}
