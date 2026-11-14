package region

import (
	"math"
	"sort"

	"github.com/vexyl-ai/rekha/internal/colour"
)

// Flatten's thresholds: two adjacent colours are one flat colour when
// their lightness is within flatLight and they share a hue, meaning
// both are neutral (chroma under flatNeutral) or both chromatic with
// hue angles within flatHue radians (23°). A neutral and a chromatic
// colour never merge: an outline is not the hair's shade.
const (
	flatLight   = 0.12
	flatHue     = 0.4
	flatNeutral = 0.02
)

// Flatten merges adjacent labels that differ in lightness only into one
// flat colour: the posterised look, in which a region's shading
// collapses into its colour while a hue step stays an edge. A merged
// group takes the colour of its largest member, the mean of that
// label's pixels, since the smaller is the shading of the larger (a
// logo's brand red does not drift when its highlight crescent joins
// it). Pairs of touching labels are visited in ascending distance
// between their palette colours; two groups merge when their colours
// are compatible (see the thresholds), so a chain of shades stops where
// the next shade is too far from the colour of what has merged. labels
// are relabelled in place; the compacted palette is returned.
func Flatten(labels, rgb []uint8, w, h int, palette []colour.OKLab, light float64) []colour.OKLab {
	n := len(palette)
	count := make([]float64, n)
	sum := make([]colour.OKLab, n)
	for p, l := range labels {
		c := colour.FromSRGB8(rgb[3*p], rgb[3*p+1], rgb[3*p+2])
		count[l]++
		sum[l].L += c.L
		sum[l].A += c.A
		sum[l].B += c.B
	}
	type pair struct{ a, b int }
	touch := map[pair]bool{}
	note := func(i, j int) {
		a, b := int(labels[i]), int(labels[j])
		if a == b {
			return
		}
		if a > b {
			a, b = b, a
		}
		touch[pair{a, b}] = true
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := y*w + x
			if x+1 < w {
				note(i, i+1)
			}
			if y+1 < h {
				note(i, i+w)
			}
		}
	}
	pairs := make([]pair, 0, len(touch))
	for k := range touch {
		pairs = append(pairs, k)
	}
	sort.Slice(pairs, func(i, j int) bool {
		di := colour.Dist(palette[pairs[i].a], palette[pairs[i].b])
		dj := colour.Dist(palette[pairs[j].a], palette[pairs[j].b])
		if di != dj {
			return di < dj
		}
		if pairs[i].a != pairs[j].a {
			return pairs[i].a < pairs[j].a
		}
		return pairs[i].b < pairs[j].b
	})

	parent := make([]int, n)
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	// Per label its own mean; per group (its root) the pixel total and
	// the label with the most pixels, whose colour the group carries.
	own := make([]colour.OKLab, n)
	for i := range own {
		if count[i] > 0 {
			own[i] = colour.OKLab{L: sum[i].L / count[i], A: sum[i].A / count[i], B: sum[i].B / count[i]}
		}
	}
	total := append([]float64(nil), count...)
	largest := make([]int, n)
	for i := range largest {
		largest[i] = i
	}
	for _, k := range pairs {
		ga, gb := find(k.a), find(k.b)
		if ga == gb || count[ga] == 0 || count[gb] == 0 || !sameFlat(own[largest[ga]], own[largest[gb]], light) {
			continue
		}
		if total[ga] < total[gb] || (total[ga] == total[gb] && ga > gb) {
			ga, gb = gb, ga
		}
		parent[gb] = ga
		total[ga] += total[gb]
		if la, lb := largest[ga], largest[gb]; count[lb] > count[la] || (count[lb] == count[la] && lb < la) {
			largest[ga] = lb
		}
	}
	out := make([]colour.OKLab, n)
	for i := range out {
		if g := find(i); count[g] > 0 {
			out[i] = own[largest[g]]
		}
	}
	for p, l := range labels {
		labels[p] = uint8(find(int(l)))
	}
	return compact(labels, out)
}

// sameFlat reports whether a and b are shades of one colour under the
// Flatten thresholds.
func sameFlat(a, b colour.OKLab, light float64) bool {
	if math.Abs(a.L-b.L) > light {
		return false
	}
	ca, cb := math.Hypot(a.A, a.B), math.Hypot(b.A, b.B)
	if ca < flatNeutral && cb < flatNeutral {
		return true
	}
	if ca < flatNeutral || cb < flatNeutral {
		return false
	}
	d := math.Abs(math.Atan2(a.B, a.A) - math.Atan2(b.B, b.A))
	if d > math.Pi {
		d = 2*math.Pi - d
	}
	return d <= flatHue
}
