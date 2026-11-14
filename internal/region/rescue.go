package region

import (
	"sort"

	"github.com/vexyl-ai/rekha/internal/colour"
)

// rescueMinArea is the smallest component Rescue considers: below it the
// speck and sliver rules own the pixels.
const rescueMinArea = 16

// rescueMax bounds the palette entries one Rescue adds.
const rescueMax = 32

// rescueCoherence is the share of a component's pixels an entry must
// fit (lie within tol of) to count as a colour of the region. The mean
// is taken when it fits that share, or when the component's own entry
// does not either: anti-aliased glyph strokes are black pixels with grey
// ones among them, whose mean is a dark grey none of them has while
// black fits most of them, and they keep black; a rim light running from
// cream to white fits neither its white label nor its mean, and takes
// the mean, the colour nearest all of its pixels.
const rescueCoherence = 1.0 / 3

// Rescue gives a palette entry to a region whose pixels the quantiser
// never saw as a colour of their own: a small object far from every
// palette entry (a badge of a few hundred pixels in a photograph) is
// labelled whatever is nearest, which may be a grey. For each component
// of at least rescueMinArea (or minArea, if larger) pixels whose mean
// pixel colour is more than tol from its own palette colour, and either
// that mean or no colour fits the component (see rescueCoherence): when
// an existing entry is within tol of that mean the component
// moves to the nearest such entry; otherwise the mean is appended as a
// new entry and the component relabelled to it. Candidates are taken by descending
// gain, pixels × distance², while the gain is at least that of sixteen
// pixels at twice the tolerance, at most rescueMax new entries and 256
// in all; a candidate within tol of an entry added in this pass shares
// it. The palette is compacted afterwards; a nil rgb returns it as is.
func Rescue(labels, rgb []uint8, w, h int, palette []colour.OKLab, minArea int, tol float64) []colour.OKLab {
	if rgb == nil {
		return palette
	}
	type candidate struct {
		c    *component
		mean colour.OKLab
		gain float64
	}
	comps := components(labels, w, h)
	var cands []candidate
	for i := range comps {
		c := &comps[i]
		if len(c.pixels) < max(minArea, rescueMinArea) {
			continue
		}
		m := meanLab(rgb, c.pixels)
		d := colour.Dist(m, palette[c.label])
		if d <= tol || (!coherent(rgb, c.pixels, m, tol) && coherent(rgb, c.pixels, palette[c.label], tol)) {
			continue
		}
		best, bd := -1, tol
		for j, p := range palette {
			if dj := colour.Dist(m, p); dj <= bd {
				best, bd = j, dj
			}
		}
		if best >= 0 {
			for _, p := range c.pixels {
				labels[p] = uint8(best)
			}
			continue
		}
		cands = append(cands, candidate{c, m, float64(len(c.pixels)) * d * d})
	}
	sort.SliceStable(cands, func(a, b int) bool { return cands[a].gain > cands[b].gain })
	minGain := float64(rescueMinArea) * (2 * tol) * (2 * tol)
	var added []int
	for _, k := range cands {
		if k.gain < minGain {
			break
		}
		to := -1
		for _, j := range added {
			if colour.Dist(k.mean, palette[j]) <= tol {
				to = j
				break
			}
		}
		if to < 0 {
			if len(added) >= rescueMax || len(palette) >= 256 {
				continue
			}
			palette = append(palette, k.mean)
			to = len(palette) - 1
			added = append(added, to)
		}
		for _, p := range k.c.pixels {
			labels[p] = uint8(to)
		}
	}
	return compact(labels, palette)
}

// meanLab is the mean of the pixels' OKLab colours: what a k-means
// centroid is, so a component made of one cluster's pixels sits on its
// own entry. (The OKLab of the mean sRGB is not: near black a few levels
// of sRGB are a tenth of OKLab's range, and the sRGB mean of a dark
// band lands nearer the next entry than its own.)
func meanLab(rgb []uint8, pixels []int32) colour.OKLab {
	var l, a, b float64
	for _, p := range pixels {
		c := colour.FromSRGB8(rgb[3*p], rgb[3*p+1], rgb[3*p+2])
		l += c.L
		a += c.A
		b += c.B
	}
	n := float64(len(pixels))
	return colour.OKLab{L: l / n, A: a / n, B: b / n}
}

// coherent reports whether at least rescueCoherence of the pixels are
// within tol of m.
func coherent(rgb []uint8, pixels []int32, m colour.OKLab, tol float64) bool {
	within := 0
	for _, p := range pixels {
		if colour.Dist(colour.FromSRGB8(rgb[3*p], rgb[3*p+1], rgb[3*p+2]), m) <= tol {
			within++
		}
	}
	return float64(within) >= rescueCoherence*float64(len(pixels))
}
