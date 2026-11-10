package gradient

import (
	"math"
	"sort"
)

// maxLabels is the palette size a uint8 label map can address.
const maxLabels = 256

// Result of Merge: labels renumbered compactly by first appearance.
type Result struct {
	Labels   []uint8 // new label per pixel
	N        int     // number of new labels
	Ramps    []*Ramp // per new label; nil for a flat label
	OldToNew []int   // per old label: its new label, or -1 when every pixel of it joined a ramp
}

// component is one 4-connected run of equal labels.
type component struct {
	label uint8
	first int32 // scan-order index of its first pixel
	m     moments
}

// edge is the boundary between two components: 4-adjacent pixel pairs
// and the sum of their sRGB distances.
type edge struct {
	pairs int
	step  float64
}

// group is a set of components merged so far (union–find over
// components, the root carries the sums).
type group struct {
	m      moments
	first  int32
	comps  int
	merged bool // more than one component
}

// Merge groups adjacent components of labels (w×h, values below n, rgb
// three bytes per pixel) that together fit one linear ramp within tol
// (RMS sRGB distance, channels 0..1), greedily, closest mean colours
// first, until no pair qualifies. A pair also needs a soft boundary: the
// mean colour step across it no more than three times the ramp's slope
// per pixel, or tol. Merged groups whose ramp changes colour by less
// than tol end to end stay flat.
func Merge(rgb []uint8, labels []uint8, w, h, n int, tol float64) Result {
	np := w * h
	comp := components(labels, w, h)
	ncomp := 0
	for _, c := range comp {
		ncomp = max(ncomp, int(c)+1)
	}
	comps := make([]component, ncomp)
	for i := range comps {
		comps[i].first = -1
	}
	// The fit sees interior pixels only: a pixel on a component's edge
	// is anti-aliased against whatever lies outside and would pull a
	// small band's residual over the tolerance.
	for p := 0; p < np; p++ {
		c := &comps[comp[p]]
		if c.first < 0 {
			c.first = int32(p)
			c.label = labels[p]
		}
		if interior(labels, w, h, p) {
			c.m.add(float64(p%w)+0.5, float64(p/w)+0.5, colourOf(rgb, p))
		}
	}
	// Boundaries between components.
	edges := map[[2]int32]*edge{}
	bump := func(p, q int) {
		a, b := comp[p], comp[q]
		if a == b {
			return
		}
		if a > b {
			a, b = b, a
		}
		k := [2]int32{a, b}
		e := edges[k]
		if e == nil {
			e = &edge{}
			edges[k] = e
		}
		e.pairs++
		e.step += dist(colourOf(rgb, p), colourOf(rgb, q))
	}
	for p := 0; p < np; p++ {
		if p%w+1 < w {
			bump(p, p+1)
		}
		if p+w < np {
			bump(p, p+w)
		}
	}
	edgeKeys := make([][2]int32, 0, len(edges))
	for k := range edges {
		edgeKeys = append(edgeKeys, k)
	}
	sort.Slice(edgeKeys, func(i, j int) bool {
		if edgeKeys[i][0] != edgeKeys[j][0] {
			return edgeKeys[i][0] < edgeKeys[j][0]
		}
		return edgeKeys[i][1] < edgeKeys[j][1]
	})
	// Adjacency between live groups, kept up to date as groups merge so
	// a group can absorb any number of neighbours in one pass.
	adj := make([]map[int32]*edge, ncomp)
	for i := range adj {
		adj[i] = map[int32]*edge{}
	}
	for _, k := range edgeKeys {
		adj[k[0]][k[1]] = edges[k]
		adj[k[1]][k[0]] = edges[k]
	}

	// Union–find over components; the root is the lower index, which is
	// the lower first pixel.
	parent := make([]int32, ncomp)
	groups := make([]group, ncomp)
	for i := range parent {
		parent[i] = int32(i)
		groups[i] = group{m: comps[i].m, first: comps[i].first, comps: 1}
	}
	var find func(i int32) int32
	find = func(i int32) int32 {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	mergedGroups := 0
	type cand struct {
		a, b  int32
		dmean float64
	}
	for {
		// Candidate pairs of live groups, closest mean colours first.
		var cands []cand
		for a := range groups {
			if find(int32(a)) != int32(a) || groups[a].m.n < minPixels {
				continue
			}
			for b := range adj[a] {
				if b > int32(a) && groups[b].m.n >= minPixels {
					cands = append(cands, cand{a: int32(a), b: b, dmean: dist(groups[a].m.mean(), groups[b].m.mean())})
				}
			}
		}
		sort.Slice(cands, func(i, j int) bool {
			if cands[i].dmean != cands[j].dmean {
				return cands[i].dmean < cands[j].dmean
			}
			if cands[i].a != cands[j].a {
				return cands[i].a < cands[j].a
			}
			return cands[i].b < cands[j].b
		})
		mergedThisPass := false
		for _, c := range cands {
			a, b := find(c.a), find(c.b)
			if a == b {
				continue
			}
			if a > b {
				a, b = b, a
			}
			e := adj[a][b]
			ga, gb := &groups[a], &groups[b]
			if e == nil || ga.m.n < minPixels || gb.m.n < minPixels {
				continue
			}
			if !ga.merged && !gb.merged && mergedGroups+1+n > maxLabels {
				continue // no label free for a new ramp
			}
			m := ga.m
			m.merge(&gb.m)
			f := m.fit()
			if !f.ok || f.rms > tol {
				continue
			}
			if e.step/float64(e.pairs) > math.Max(3*f.slope, tol) {
				continue
			}
			if !sidesFollow(ga, gb, f) {
				continue
			}
			// Merge b into a: sums, flags, and b's neighbours become a's.
			if !ga.merged && !gb.merged {
				mergedGroups++
			} else if ga.merged && gb.merged {
				mergedGroups--
			}
			parent[b] = a
			ga.m = m
			ga.comps += gb.comps
			ga.merged = true
			for x, ex := range adj[b] {
				delete(adj[x], b)
				if x == a {
					continue
				}
				if ea := adj[a][x]; ea != nil {
					ea.pairs += ex.pairs
					ea.step += ex.step
				} else {
					adj[a][x] = ex
					adj[x][a] = ex
				}
			}
			adj[b] = nil
			mergedThisPass = true
		}
		if !mergedThisPass {
			break
		}
	}

	// Every group with enough interior pixels whose fit is within tol is
	// a ramp candidate; its extent along u is measured over all its
	// pixels. A candidate at least minLength long that changes colour by
	// at least max(tol, minSpan) end to end becomes a ramp and needs a
	// label of its own, as does a merged group; other groups keep their
	// old label.
	type extent struct {
		f          fitted
		tmin, tmax float64
	}
	ext := map[int32]*extent{}
	var roots []int32
	for i := range groups {
		if find(int32(i)) != int32(i) {
			continue
		}
		roots = append(roots, int32(i))
		g := &groups[i]
		if g.m.n < minPixels {
			continue
		}
		if f := g.m.fit(); f.ok && f.rms <= tol {
			ext[int32(i)] = &extent{f: f, tmin: math.Inf(1), tmax: math.Inf(-1)}
		}
	}
	for p := 0; p < np; p++ {
		e := ext[find(comp[p])]
		if e == nil {
			continue
		}
		t := float64(e.f.u[0]*(float64(p%w)+0.5)) + float64(e.f.u[1]*(float64(p/w)+0.5))
		e.tmin = math.Min(e.tmin, t)
		e.tmax = math.Max(e.tmax, t)
	}
	// Sample the colour along each candidate from the pixels interior to
	// the whole group (every 4-neighbour in the same group), so a band
	// boundary's blend and the group's own anti-aliased edge do not pull
	// a stop.
	binsOf := map[int32]*bins{}
	for r, e := range ext {
		if e.tmax-e.tmin >= minLength {
			binsOf[r] = newBins(e.tmin, e.tmax)
		}
	}
	for p := 0; p < np; p++ {
		r := find(comp[p])
		b := binsOf[r]
		if b == nil {
			continue
		}
		x, y := p%w, p/w
		if x > 0 && find(comp[p-1]) != r || x+1 < w && find(comp[p+1]) != r || y > 0 && find(comp[p-w]) != r || y+1 < h && find(comp[p+w]) != r {
			continue
		}
		e := ext[r]
		t := float64(e.f.u[0]*(float64(x)+0.5)) + float64(e.f.u[1]*(float64(y)+0.5))
		b.add(t, colourOf(rgb, p))
	}
	sort.Slice(roots, func(i, j int) bool { return groups[roots[i]].first < groups[roots[j]].first })
	fresh := map[int32]int{}
	rampOf := map[int32]*Ramp{}
	rampFor := func(r int32) *Ramp {
		b := binsOf[r]
		if b == nil {
			return nil
		}
		return rampFrom(&groups[r].m, ext[r].f, b, tol)
	}
	// Merged groups always need a label (the rounds reserved one each);
	// lone ramps take what is left, in scan order.
	for _, r := range roots {
		if groups[r].merged {
			fresh[r] = n + len(fresh)
			rampOf[r] = rampFor(r)
		}
	}
	for _, r := range roots {
		if groups[r].merged || n+len(fresh) >= maxLabels {
			continue
		}
		if rp := rampFor(r); rp != nil {
			fresh[r] = n + len(fresh)
			rampOf[r] = rp
		}
	}
	pre := make([]int, ncomp)
	for i := range comps {
		if l, ok := fresh[find(int32(i))]; ok {
			pre[i] = l
		} else {
			pre[i] = int(comps[i].label)
		}
	}
	// Compact by first appearance.
	remap := make([]int, n+len(fresh))
	for i := range remap {
		remap[i] = -1
	}
	res := Result{Labels: make([]uint8, np), OldToNew: make([]int, n)}
	var ramps []*Ramp
	for p := 0; p < np; p++ {
		l := pre[comp[p]]
		if remap[l] < 0 {
			remap[l] = len(ramps)
			ramps = append(ramps, rampOf[find(comp[p])])
		}
		res.Labels[p] = uint8(remap[l])
	}
	for i := range res.OldToNew {
		res.OldToNew[i] = remap[i]
	}
	res.N = len(ramps)
	res.Ramps = ramps
	return res
}

// sidesFollow reports whether the two sides of a candidate merge are
// themselves ramps of the union's slope: a band of a true ramp carries
// the ramp's slope inside it, a flat colour carries none, so two flats
// (or a flat and a ramp) across a soft step never pass. A side too thin
// for a fit of its own, or too narrow for the union's slope to change
// its colour by a printable step across it, does not vote; at least
// one side must.
func sidesFollow(a, b *group, union fitted) bool {
	voted := false
	for _, g := range []*group{a, b} {
		f := g.m.fit()
		if !f.ok || float64(union.slope*f.extent) < minSpan {
			continue
		}
		voted = true
		if f.slope < sideSlope*union.slope {
			return false
		}
	}
	return voted
}

// interior reports whether every in-image 4-neighbour of pixel p carries
// its label.
func interior(labels []uint8, w, h, p int) bool {
	l := labels[p]
	x, y := p%w, p/w
	return (x == 0 || labels[p-1] == l) && (x == w-1 || labels[p+1] == l) &&
		(y == 0 || labels[p-w] == l) && (y == h-1 || labels[p+w] == l)
}

// components labels the 4-connected components of equal labels, numbered
// in scan order of their first pixel.
func components(labels []uint8, w, h int) []int32 {
	np := w * h
	parent := make([]int32, np)
	for i := range parent {
		parent[i] = int32(i)
	}
	var find func(i int32) int32
	find = func(i int32) int32 {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	union := func(a, b int32) {
		ra, rb := find(a), find(b)
		if ra == rb {
			return
		}
		if ra > rb {
			ra, rb = rb, ra
		}
		parent[rb] = ra
	}
	for p := 0; p < np; p++ {
		if p%w+1 < w && labels[p] == labels[p+1] {
			union(int32(p), int32(p+1))
		}
		if p+w < np && labels[p] == labels[p+w] {
			union(int32(p), int32(p+w))
		}
	}
	id := make([]int32, np)
	next := int32(0)
	seen := make([]int32, np)
	for i := range seen {
		seen[i] = -1
	}
	for p := 0; p < np; p++ {
		r := find(int32(p))
		if seen[r] < 0 {
			seen[r] = next
			next++
		}
		id[p] = seen[r]
	}
	return id
}

func colourOf(rgb []uint8, p int) [3]float64 {
	return [3]float64{float64(rgb[3*p]) / 255, float64(rgb[3*p+1]) / 255, float64(rgb[3*p+2]) / 255}
}

func dist(a, b [3]float64) float64 {
	d0, d1, d2 := a[0]-b[0], a[1]-b[1], a[2]-b[2]
	return math.Sqrt(float64(d0*d0) + float64(d1*d1) + float64(d2*d2))
}
