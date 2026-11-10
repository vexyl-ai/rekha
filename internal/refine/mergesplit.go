package refine

import (
	"math"

	"github.com/vexyl-ai/rekha/internal/fit"
	"github.com/vexyl-ai/rekha/internal/geom"
)

const (
	// mergeAngle is the largest joint angle, in radians (0.5°), at which
	// the two segments around a joint are candidates for one cubic.
	mergeAngle = 0.5 * math.Pi / 180
	// mergeRise is the relative rise in data energy a merge may cost.
	mergeRise = 0.01
	// splitFactor: a segment whose data residual per pixel of its length
	// exceeds this multiple of the other segments' mean is split in two.
	splitFactor = 3.0
	// splitFloor is the squared coverage error per pixel of length below
	// which a segment is never split, however its neighbours fit: an RMS
	// edge error of 0.1 px.
	splitFloor = 0.01
	// mergeMaxDev is the furthest, in pixels, a merged cubic may lie from
	// the two curves it replaces and still be tried: the piece optimiser
	// moves a point by at most moveLimit, so a cubic further out than
	// that is almost never brought back within the energy budget (on the
	// big-poster corpus, 12 of 685 such tries were), and the two
	// optimisations it would cost are the dearest part of the pass.
	mergeMaxDev = moveLimit
)

// MergeSplit adjusts the segment count after the sweeps: one merge pass
// over smooth joints, one split pass over segments that fit badly, then
// one more sweep. The paper keeps N fixed; the Phase 1 fitter chose N
// before the raster was consulted and over-segments anti-aliased edges.
func MergeSplit(b Bezigon, m Mask, img Window, o Options) Bezigon {
	out, _ := MergeSplitTracked(b, m, img, o)
	return out
}

// MergeSplitTracked is MergeSplit that also returns the options with
// Fixed and Frozen renumbered for the segments of the result, so a
// caller can find the joints it marked.
func MergeSplitTracked(b Bezigon, m Mask, img Window, o Options) (Bezigon, Options) {
	if b.N() < 2 {
		return b, o
	}
	oIn := o
	out := Bezigon{Pts: append([]geom.Vec2(nil), b.Pts...)}
	clampFree(out, img, o)
	l0 := math.Max(out.Length(), 1)
	sc := &scratch{}
	// The passes change the segment count, so they carry the options'
	// Fixed and Frozen along with their renumbering.
	out, o = mergePass(out, m, img, l0, o, sc)
	out, o = splitPass(out, m, img, l0, o, sc)
	for j := 0; j < out.N(); j++ {
		refinePiece(out, j, m, img, l0, o, sc)
	}
	for _, p := range out.Pts {
		if !finite(p.X) || !finite(p.Y) {
			return b, oIn
		}
	}
	return out, o
}

// dataEnergy is the data term of b over the whole image at l0, taken
// over the loop's window (see loopWindow).
func dataEnergy(b Bezigon, m Mask, img Window, l0 float64) float64 {
	win := loopWindow(b, m, img)
	cov := make([]float32, win.W*win.H)
	Coverage(b, win, cov)
	return DataEnergy(cov, m, win, l0)
}

// localDataEnergy is the data term of b over the window wl alone. For
// two outlines that differ only inside wl the difference of their local
// terms is the difference of their whole-image terms: a change confined
// to a box alters no coverage outside it, since the rows to its right
// see the same net winding from a path with the same ends.
func localDataEnergy(b Bezigon, m Mask, wl Window, l0 float64) float64 {
	cov := make([]float32, wl.W*wl.H)
	Coverage(b, wl, cov)
	return DataEnergy(cov, m, wl, l0)
}

// segmentsBox is the bounding box of the control points of segments
// from..to (cyclic, inclusive).
func segmentsBox(b Bezigon, from, to int) bounds {
	n := b.N()
	r := bounds{lo: geom.Vec2{X: math.Inf(1), Y: math.Inf(1)}, hi: geom.Vec2{X: math.Inf(-1), Y: math.Inf(-1)}}
	for k := from; k <= to; k++ {
		for _, p := range b.Segment(((k % n) + n) % n) {
			r.lo.X, r.lo.Y = math.Min(r.lo.X, p.X), math.Min(r.lo.Y, p.Y)
			r.hi.X, r.hi.Y = math.Max(r.hi.X, p.X), math.Max(r.hi.Y, p.Y)
		}
	}
	return r
}

// windowAround is the pixel window inside img that holds every box,
// padded by one pixel.
func windowAround(img Window, boxes ...bounds) Window {
	lo, hi := geom.Vec2{X: math.Inf(1), Y: math.Inf(1)}, geom.Vec2{X: math.Inf(-1), Y: math.Inf(-1)}
	for _, r := range boxes {
		lo.X, lo.Y = math.Min(lo.X, r.lo.X), math.Min(lo.Y, r.lo.Y)
		hi.X, hi.Y = math.Max(hi.X, r.hi.X), math.Max(hi.Y, r.hi.Y)
	}
	x0 := max(int(math.Floor(lo.X))-1, img.X0)
	y0 := max(int(math.Floor(lo.Y))-1, img.Y0)
	x1 := min(int(math.Ceil(hi.X))+1, img.X0+img.W)
	y1 := min(int(math.Ceil(hi.Y))+1, img.Y0+img.H)
	return Window{X0: x0, Y0: y0, W: max(x1-x0, 0), H: max(y1-y0, 0)}
}

// mergePass tries every joint whose angle is below mergeAngle: the two
// segments become one cubic fitted to their flattened points, the pieces
// around it are re-optimised, and the result stands if the data energy
// stays within mergeRise of where the pass started, globally and over
// the merge's own window. Never below two segments. A fixed joint, or
// one beside a frozen segment, is never merged.
func mergePass(b Bezigon, m Mask, img Window, l0 float64, o Options, sc *scratch) (Bezigon, Options) {
	// Every accepted merge is judged against the energy the pass started
	// with, so the whole pass costs at most mergeRise.
	before := dataEnergy(b, m, img, l0)
	budget := float64(before * (1 + mergeRise))
	for j := 0; j < b.N() && b.N() > 2; {
		if o.fixed(j) || o.frozen(j) || o.frozen((j+b.N()-1)%b.N()) || AnglePrior(b, j) >= mergeAngle {
			j++
			continue
		}
		cand := mergeJoint(b, j)
		co := o.withoutJoint(j)
		k := ((j-1)%cand.N() + cand.N()) % cand.N() // the merged segment
		if mergeDeviation(b, j, cand.Segment(k)) > mergeMaxDev {
			j++
			continue
		}
		refinePiece(cand, k-1, m, img, l0, co, sc)
		refinePiece(cand, k, m, img, l0, co, sc)
		// Only segments j-2..j+1 of b (k-1..k+1 of cand) changed, so the
		// loop's crossing count moves by the crossings those have.
		if rangeCrossings(cand, k-1, k+1) > rangeCrossings(b, j-2, j+1) {
			j++
			continue // the merged cubic crosses the outline
		}
		// Only segments j-2..j+1 of b (k-1..k+1 of cand) changed: the
		// energy moves by the local difference.
		wl := windowAround(img, segmentsBox(b, j-2, j+1), segmentsBox(cand, k-1, k+1))
		localBefore, localAfter := localDataEnergy(b, m, wl, l0), localDataEnergy(cand, m, wl, l0)
		after := before - localBefore + localAfter
		// The merge must also hold locally: a long outline's budget would
		// otherwise pay for rounding off one corner. The local allowance
		// is mergeRise of the local energy plus an RMS edge error of
		// √splitFloor px over the merged segment.
		localBudget := float64(localBefore*(1+mergeRise)) + float64(DataScale*DataScale*splitFloor)*math.Max(LengthPrior(cand, k), 1)/l0
		if after <= budget && localAfter <= localBudget {
			b, before, o = cand, after, co
			// Joint j is gone; the joint now at index k is the merged
			// segment's start, which the next iteration examines.
			if j > 0 {
				j--
			}
			continue
		}
		j++
	}
	return b, o
}

// mergeJoint replaces the segments either side of joint j by one cubic
// fitted to their flattened points, keeping the outer end tangents.
func mergeJoint(b Bezigon, j int) Bezigon {
	n := b.N()
	prev := ((j-1)%n + n) % n
	cp, cn := b.Segment(prev), b.Segment(j)
	var buf [2*Flatten + 1]geom.Vec2
	pts := append(flattenInto(flattenInto(buf[:0], cp), cn), cn[3])
	t0 := tangentOr(cp[1].Sub(cp[0]), cp[3].Sub(cp[0]))
	t1 := tangentOr(cn[2].Sub(cn[3]), cn[0].Sub(cn[3]))
	c := fit.FitOne(pts, t0, t1)
	out := Bezigon{Pts: make([]geom.Vec2, 0, 3*(n-1))}
	for k := 0; k < n; k++ {
		switch k {
		case prev:
			out.Pts = append(out.Pts, c[0], c[1], c[2])
		case j:
		default:
			s := b.Segment(k)
			out.Pts = append(out.Pts, s[0], s[1], s[2])
		}
	}
	return out
}

// mergeDeviation is the furthest any flattened point of the two segments
// around joint j lies from the flattened cubic c that would replace them.
func mergeDeviation(b Bezigon, j int, c fit.Cubic) float64 {
	n := b.N()
	prev := ((j-1)%n + n) % n
	cp, cn := b.Segment(prev), b.Segment(j)
	var buf [2*Flatten + 1]geom.Vec2
	pts := append(flattenInto(flattenInto(buf[:0], cp), cn), cn[3])
	var cbuf [Flatten + 1]geom.Vec2
	cpts := append(flattenInto(cbuf[:0], c), c[3])
	worst := 0.0
	for _, p := range pts {
		best := math.Inf(1)
		for i := 0; i+1 < len(cpts); i++ {
			best = math.Min(best, segmentDistance(p, cpts[i], cpts[i+1]))
		}
		worst = math.Max(worst, best)
	}
	return worst
}

// segmentDistance is the distance from p to the segment a–b.
func segmentDistance(p, a, b geom.Vec2) float64 {
	d := b.Sub(a)
	l2 := d.LenSq()
	if l2 == 0 {
		return p.Sub(a).Len()
	}
	t := math.Min(math.Max(p.Sub(a).Dot(d)/l2, 0), 1)
	return p.Sub(a.Add(d.Scale(t))).Len()
}

// tangentOr is the unit direction of v, or of alt when v is degenerate.
func tangentOr(v, alt geom.Vec2) geom.Vec2 {
	if v.LenSq() < minHandle*minHandle {
		v = alt
	}
	if v.LenSq() == 0 {
		return geom.Vec2{X: 1}
	}
	return v.Norm()
}

// splitPass halves every segment whose squared coverage error per pixel
// of its length exceeds splitFactor times the other segments' mean (and
// splitFloor), then re-optimises the pieces around each new joint.
// Residuals are measured once, before any split. The mean leaves the
// segment itself out: with few segments one bad fit dominates the whole
// bezigon's mean and could never exceed a multiple of it. A frozen
// segment is never split.
func splitPass(b Bezigon, m Mask, img Window, l0 float64, o Options, sc *scratch) (Bezigon, Options) {
	n := b.N()
	win := loopWindow(b, m, img)
	cov := make([]float32, win.W*win.H)
	Coverage(b, win, cov)
	resid, length := make([]float64, n), make([]float64, n)
	var sumR, sumL float64
	for j := 0; j < n; j++ {
		resid[j] = segmentResidual(cov, m, win, b.Segment(j))
		length[j] = math.Max(LengthPrior(b, j), 1)
		sumR += resid[j]
		sumL += length[j]
	}
	// Split from the last segment down so earlier indices stay valid.
	for j := n - 1; j >= 0; j-- {
		per := resid[j] / length[j]
		if o.frozen(j) || per < splitFloor {
			continue
		}
		others := 0.0
		if sumL-length[j] > 0 {
			others = (sumR - resid[j]) / (sumL - length[j])
		}
		if per <= splitFactor*others {
			continue
		}
		b, o = splitSegment(b, j), o.withSplit(j)
		for _, p := range []int{j - 1, j, j + 1} {
			refinePiece(b, p, m, img, l0, o, sc)
		}
	}
	return b, o
}

// segmentResidual sums the squared coverage error (0..1 scale) over the
// window of a segment's control points padded by one pixel, clipped to
// img, the window cov was rasterised over.
func segmentResidual(cov []float32, m Mask, img Window, c fit.Cubic) float64 {
	x0, y0, x1, y1 := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, p := range c {
		x0, y0 = math.Min(x0, p.X), math.Min(y0, p.Y)
		x1, y1 = math.Max(x1, p.X), math.Max(y1, p.Y)
	}
	ix0 := max(int(math.Floor(x0))-1, img.X0)
	iy0 := max(int(math.Floor(y0))-1, img.Y0)
	ix1 := min(int(math.Ceil(x1))+1, img.X0+img.W)
	iy1 := min(int(math.Ceil(y1))+1, img.Y0+img.H)
	resid := 0.0
	for y := iy0; y < iy1; y++ {
		for x := ix0; x < ix1; x++ {
			target, w := m.At(x, y)
			d := float64(cov[(y-img.Y0)*img.W+(x-img.X0)] - target)
			resid += float64(float64(w) * d * d)
		}
	}
	return resid
}

// splitSegment halves segment j at t = 0.5 by de Casteljau.
func splitSegment(b Bezigon, j int) Bezigon {
	n := b.N()
	c := b.Segment(j)
	p01, p12, p23 := c[0].Add(c[1]).Scale(0.5), c[1].Add(c[2]).Scale(0.5), c[2].Add(c[3]).Scale(0.5)
	p012, p123 := p01.Add(p12).Scale(0.5), p12.Add(p23).Scale(0.5)
	mid := p012.Add(p123).Scale(0.5)
	out := Bezigon{Pts: make([]geom.Vec2, 0, 3*(n+1))}
	for k := 0; k < n; k++ {
		s := b.Segment(k)
		if k == j {
			out.Pts = append(out.Pts, s[0], p01, p012, mid, p123, p23)
		} else {
			out.Pts = append(out.Pts, s[0], s[1], s[2])
		}
	}
	return out
}
