package region

import (
	"sort"

	"github.com/vexyl-ai/rekha/internal/colour"
)

// sliverThickness is the mean thickness, in pixels, below which a
// component is a candidate anti-aliasing band.
const sliverThickness = 1.5

// maxPasses bounds the despeckle and sliver passes; each pass recomputes
// components after the previous pass's relabelling.
const maxPasses = 4

// Cleanup edits labels (w×h, indices into palette) in place and returns
// the compacted palette:
//
//   - despeckle: a component with fewer than minArea pixels joins the
//     neighbouring label sharing the longest border (ties: lower label);
//   - slivers: a component thinner than 1.5 px on average is an
//     anti-aliasing band, and joins the longest-border neighbour, when
//     its colour lies within tol of the OKLab segment between its two
//     longest-border neighbours' colours, or when the mean of its pixels
//     (rgb, w×h×3 sRGB, nil to skip) lies within tol of the sRGB mixture
//     of those two colours: anti-aliased pixels between two colours
//     quantise to whatever palette entry is nearest, which may be a third
//     colour of the image far from the segment, so the pixels decide, not
//     the label. A component with one neighbour is kept;
//   - compaction: unused palette entries are dropped and labels renumbered
//     by first appearance in scan order.
func Cleanup(labels, rgb []uint8, w, h int, palette []colour.OKLab, minArea int, tol float64) []colour.OKLab {
	pass(labels, w, h, func(c *component) bool { return len(c.pixels) < minArea }, nil)
	// A sliver's pixels go to whichever of the two neighbours each is
	// nearer: it is the anti-aliased edge between them, and every pixel
	// of it leans one way or the other. (Specks keep the wholesale rule:
	// splitting them leaves orphan pixels that become specks of their
	// own.)
	var nearer func(c *component, p int32) uint8
	if rgb != nil {
		nearer = func(c *component, p int32) uint8 {
			a, b := c.neighbours[0].label, c.neighbours[1].label
			px := colour.FromSRGB8(rgb[3*p], rgb[3*p+1], rgb[3*p+2])
			if colour.Dist(px, palette[b]) < colour.Dist(px, palette[a]) {
				return b
			}
			return a
		}
	}
	pass(labels, w, h, func(c *component) bool {
		if len(c.neighbours) < 2 || float64(len(c.pixels))/(float64(c.perimeter)/2) >= sliverThickness {
			return false
		}
		a, b := palette[c.neighbours[0].label], palette[c.neighbours[1].label]
		if colour.SegmentDist(palette[c.label], a, b) <= tol {
			return true
		}
		return rgb != nil && colour.MixDist(meanColour(rgb, c.pixels), a, b) <= tol
	}, nearer)
	return compact(labels, palette)
}

// meanColour is the OKLab of the mean sRGB of the pixels.
func meanColour(rgb []uint8, pixels []int32) colour.OKLab {
	var r, g, b int
	for _, p := range pixels {
		r += int(rgb[3*p])
		g += int(rgb[3*p+1])
		b += int(rgb[3*p+2])
	}
	n := len(pixels)
	round := func(v int) uint8 { return uint8((2*v + n) / (2 * n)) }
	return colour.FromSRGB8(round(r), round(g), round(b))
}

// pass relabels, for up to maxPasses rounds, every component that absorb
// selects, smallest components first: each pixel to the label target
// gives it, or, with a nil target, the whole component to its
// longest-border neighbour.
func pass(labels []uint8, w, h int, absorb func(*component) bool, target func(*component, int32) uint8) {
	for round := 0; round < maxPasses; round++ {
		comps := components(labels, w, h)
		order := make([]int, len(comps))
		for i := range order {
			order[i] = i
		}
		sort.SliceStable(order, func(a, b int) bool { return len(comps[order[a]].pixels) < len(comps[order[b]].pixels) })
		changed := false
		for _, i := range order {
			c := &comps[i]
			if len(c.neighbours) == 0 || !absorb(c) {
				continue
			}
			to := c.neighbours[0].label
			for _, p := range c.pixels {
				if target != nil {
					to = target(c, p)
				}
				labels[p] = to
			}
			changed = true
		}
		if !changed {
			return
		}
	}
}

// compact renumbers labels by first appearance and returns the matching
// palette.
func compact(labels []uint8, palette []colour.OKLab) []colour.OKLab {
	remap := make([]int, len(palette))
	for i := range remap {
		remap[i] = -1
	}
	var out []colour.OKLab
	for i, l := range labels {
		if remap[l] < 0 {
			remap[l] = len(out)
			out = append(out, palette[l])
		}
		labels[i] = uint8(remap[l])
	}
	return out
}
