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
	pass(labels, w, h, func(c *component) bool { return len(c.pixels) < minArea })
	pass(labels, w, h, func(c *component) bool {
		if len(c.neighbours) < 2 || float64(len(c.pixels))/(float64(c.perimeter)/2) >= sliverThickness {
			return false
		}
		a, b := palette[c.neighbours[0].label], palette[c.neighbours[1].label]
		if colour.SegmentDist(palette[c.label], a, b) <= tol {
			return true
		}
		return rgb != nil && colour.MixDist(meanColour(rgb, c.pixels), a, b) <= tol
	})
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
// selects into its longest-border neighbour, smallest components first.
func pass(labels []uint8, w, h int, absorb func(*component) bool) {
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
