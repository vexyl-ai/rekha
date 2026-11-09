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
)

// MergeSplit adjusts the segment count after the sweeps: one merge pass
// over smooth joints, one split pass over segments that fit badly, then
// one more sweep. The paper keeps N fixed; the Phase 1 fitter chose N
// before the raster was consulted and over-segments anti-aliased edges.
func MergeSplit(b Bezigon, m Mask, img Window, o Options) Bezigon {
	if b.N() < 2 {
		return b
	}
	out := Bezigon{Pts: append([]geom.Vec2(nil), b.Pts...)}
	clampAll(out, img)
	l0 := math.Max(out.Length(), 1)
	out = mergePass(out, m, img, l0, o)
	out = splitPass(out, m, img, l0, o)
	for j := 0; j < out.N(); j++ {
		refinePiece(out, j, m, img, l0, o)
	}
	for _, p := range out.Pts {
		if !finite(p.X) || !finite(p.Y) {
			return b
		}
	}
	return out
}

// dataEnergy is the data term of b over the whole image at l0.
func dataEnergy(b Bezigon, m Mask, img Window, l0 float64) float64 {
	cov := make([]float32, img.W*img.H)
	Coverage(b, img, cov)
	return DataEnergy(cov, m, img, l0)
}

// mergePass tries every joint whose angle is below mergeAngle: the two
// segments become one cubic fitted to their flattened points, the pieces
// around it are re-optimised, and the result stands if the data energy
// rises by less than mergeRise. Never below two segments.
func mergePass(b Bezigon, m Mask, img Window, l0 float64, o Options) Bezigon {
	before := dataEnergy(b, m, img, l0)
	for j := 0; j < b.N() && b.N() > 2; {
		if AnglePrior(b, j) >= mergeAngle {
			j++
			continue
		}
		cand := mergeJoint(b, j)
		k := ((j-1)%cand.N() + cand.N()) % cand.N() // the merged segment
		refinePiece(cand, k-1, m, img, l0, o)
		refinePiece(cand, k, m, img, l0, o)
		after := dataEnergy(cand, m, img, l0)
		if after <= before*(1+mergeRise) {
			b, before = cand, after
			// Joint j is gone; the joint now at index k is the merged
			// segment's start, which the next iteration examines.
			if j > 0 {
				j--
			}
			continue
		}
		j++
	}
	return b
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
// bezigon's mean and could never exceed a multiple of it.
func splitPass(b Bezigon, m Mask, img Window, l0 float64, o Options) Bezigon {
	n := b.N()
	cov := make([]float32, img.W*img.H)
	Coverage(b, img, cov)
	resid, length := make([]float64, n), make([]float64, n)
	var sumR, sumL float64
	for j := 0; j < n; j++ {
		resid[j] = segmentResidual(cov, m, img, b.Segment(j))
		length[j] = math.Max(LengthPrior(b, j), 1)
		sumR += resid[j]
		sumL += length[j]
	}
	// Split from the last segment down so earlier indices stay valid.
	for j := n - 1; j >= 0; j-- {
		per := resid[j] / length[j]
		if per < splitFloor {
			continue
		}
		others := 0.0
		if sumL-length[j] > 0 {
			others = (sumR - resid[j]) / (sumL - length[j])
		}
		if per <= splitFactor*others {
			continue
		}
		b = splitSegment(b, j)
		for _, p := range []int{j - 1, j, j + 1} {
			refinePiece(b, p, m, img, l0, o)
		}
	}
	return b
}

// segmentResidual sums the squared coverage error (0..1 scale) over the
// window of a segment's control points padded by one pixel, clipped to
// the image.
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
			resid += float64(w) * d * d
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
