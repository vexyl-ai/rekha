package region

import (
	"sort"

	"github.com/vexyl-ai/rekha/internal/colour"
)

const (
	// reclaimMinArea is the smallest blob Reclaim considers.
	reclaimMinArea = 64
	// reclaimInterior is the share of a blob's pixels that must have all
	// four neighbours in the blob: a strand of edge pixels has none, a
	// three-pixel strip a third, a band of shading nearly all.
	reclaimInterior = 0.5
	// reclaimCoherence is the share of a blob that must lie within tol of
	// its mean: a plateau does, a blur from one colour to a far one does
	// not.
	reclaimCoherence = 0.5
	// reclaimMax bounds the palette entries one Reclaim adds.
	reclaimMax = 32
)

// Reclaim gives a palette entry to a blob the quantiser folded into its
// surroundings: shading too small for a colour group of its own (a
// shadow under a chin, a few thousand pixels in a megapixel render) is
// labelled the colour around it, and Rescue, which judges whole
// components, sees a component whose mean is that colour. Reclaim looks
// inside: the pixels farther than tol from their label's colour, taken
// as 4-connected blobs of one label. A blob of at least reclaimMinArea
// pixels, with at least reclaimInterior of them interior (every
// in-image 4-neighbour in the blob), and reclaimCoherence of them within
// tol of the blob's mean, moves to the nearest existing entry within tol
// of that mean, or has the mean appended as a new entry. Candidates for
// new entries are taken by descending gain, pixels × distance², while
// the gain is at least that of reclaimMinArea pixels at twice the
// tolerance, at most reclaimMax new entries and 256 in all; a candidate
// within tol of an entry added in this pass shares it. The palette is
// compacted afterwards; a nil rgb returns it as is.
func Reclaim(labels, rgb []uint8, w, h int, palette []colour.OKLab, tol float64) []colour.OKLab {
	if rgb == nil {
		return palette
	}
	n := w * h
	far := make([]bool, n)
	for p := 0; p < n; p++ {
		far[p] = colour.Dist(colour.FromSRGB8(rgb[3*p], rgb[3*p+1], rgb[3*p+2]), palette[labels[p]]) > tol
	}
	type candidate struct {
		pixels []int32
		mean   colour.OKLab
		gain   float64
	}
	var cands []candidate
	blob := make([]int32, n) // blob id per pixel, 0 for none
	var stack []int32
	id := int32(0)
	for start := 0; start < n; start++ {
		if !far[start] || blob[start] != 0 {
			continue
		}
		id++
		l := labels[start]
		var pix []int32
		blob[start] = id
		stack = append(stack[:0], int32(start))
		for len(stack) > 0 {
			p := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			pix = append(pix, p)
			x, y := int(p)%w, int(p)/w
			visit := func(q int32) {
				if far[q] && blob[q] == 0 && labels[q] == l {
					blob[q] = id
					stack = append(stack, q)
				}
			}
			if x > 0 {
				visit(p - 1)
			}
			if x < w-1 {
				visit(p + 1)
			}
			if y > 0 {
				visit(p - int32(w))
			}
			if y < h-1 {
				visit(p + int32(w))
			}
		}
		if len(pix) < reclaimMinArea {
			continue
		}
		interior := 0
		for _, p := range pix {
			x, y := int(p)%w, int(p)/w
			if (x == 0 || blob[p-1] == id) && (x == w-1 || blob[p+1] == id) &&
				(y == 0 || blob[p-int32(w)] == id) && (y == h-1 || blob[p+int32(w)] == id) {
				interior++
			}
		}
		if float64(interior) < reclaimInterior*float64(len(pix)) {
			continue
		}
		m := meanLab(rgb, pix)
		if !coherent(rgb, pix, m, tol, reclaimCoherence) {
			continue
		}
		best, bd := -1, tol
		for j, c := range palette {
			if dj := colour.Dist(m, c); dj <= bd {
				best, bd = j, dj
			}
		}
		if best >= 0 {
			for _, p := range pix {
				labels[p] = uint8(best)
			}
			continue
		}
		d := colour.Dist(m, palette[l])
		cands = append(cands, candidate{pix, m, float64(len(pix)) * d * d})
	}
	sort.SliceStable(cands, func(a, b int) bool { return cands[a].gain > cands[b].gain })
	minGain := float64(reclaimMinArea) * (2 * tol) * (2 * tol)
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
			if len(added) >= reclaimMax || len(palette) >= 256 {
				continue
			}
			palette = append(palette, k.mean)
			to = len(palette) - 1
			added = append(added, to)
		}
		for _, p := range k.pixels {
			labels[p] = uint8(to)
		}
	}
	return compact(labels, palette)
}
