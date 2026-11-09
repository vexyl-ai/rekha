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
//   - slivers: a component thinner than 1.5 px on average whose colour
//     lies within tol of the OKLab segment between its two longest-border
//     neighbours' colours is an anti-aliasing band and joins the
//     longest-border neighbour; a component with one neighbour is kept;
//   - compaction: unused palette entries are dropped and labels renumbered
//     by first appearance in scan order.
func Cleanup(labels []uint8, w, h int, palette []colour.OKLab, minArea int, tol float64) []colour.OKLab {
	pass(labels, w, h, func(c *component) bool { return len(c.pixels) < minArea })
	pass(labels, w, h, func(c *component) bool {
		if len(c.neighbours) < 2 || float64(len(c.pixels))/(float64(c.perimeter)/2) >= sliverThickness {
			return false
		}
		a, b := palette[c.neighbours[0].label], palette[c.neighbours[1].label]
		return segmentDist(palette[c.label], a, b) <= tol
	})
	return compact(labels, palette)
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

// segmentDist is the distance from c to the segment a→b in OKLab.
func segmentDist(c, a, b colour.OKLab) float64 {
	dl, da, db := b.L-a.L, b.A-a.A, b.B-a.B
	den := float64(dl*dl) + float64(da*da) + float64(db*db)
	t := 0.0
	if den > 0 {
		t = (float64((c.L-a.L)*dl) + float64((c.A-a.A)*da) + float64((c.B-a.B)*db)) / den
		t = min(max(t, 0), 1)
	}
	return colour.Dist(c, colour.OKLab{L: a.L + t*dl, A: a.A + t*da, B: a.B + t*db})
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
