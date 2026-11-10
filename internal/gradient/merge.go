package gradient

import (
	"math"
	"slices"
	"sort"
)

// maxLabels is the palette size a uint8 label map can address.
const maxLabels = 256

// maxReplay caps the model checks spent re-joining one dissolved group.
const maxReplay = 32

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
// three bytes per pixel) that form colour ramps. A first pass joins
// bands that together fit one linear ramp within tol (RMS sRGB
// distance, channels 0..1), greedily, closest mean colours first, until
// no pair qualifies: a pair needs a soft boundary (mean colour step
// across it no more than three times the ramp's slope per pixel, or
// tol) and each side carrying the ramp's slope. A second pass joins the
// groups so formed, and bands that bend, along a shared direction when
// the boundary is as soft and each side either carries the slope or is
// bent (its colour spreads by tol or more); a joined group's ramp
// samples the colour along the direction, and a joined group whose
// samples do not reproduce its pixels within tol falls back to the
// first pass's groups. A ramp that changes colour by less than tol
// stays flat.
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
			// The tensor sees gentle changes only: a step of tol or more
			// per pixel is an edge (an outline's anti-aliasing, a band
			// boundary's ringing), not a ramp's direction.
			if x, y := p%w, p/w; x > 0 && x < w-1 && y > 0 && y < h-1 {
				gx, gy := half(colourOf(rgb, p+1), colourOf(rgb, p-1)), half(colourOf(rgb, p+w), colourOf(rgb, p-w))
				if float64(dist(gx, [3]float64{})*dist(gx, [3]float64{}))+float64(dist(gy, [3]float64{})*dist(gy, [3]float64{})) <= float64(tol*tol) {
					c.m.addGrad(gx, gy)
				}
			}
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
	// passes runs merge passes until none merges; admit judges a pair
	// from the union's fit.
	// release says whether joining two merged groups gives a label back:
	// in the join pass it does not, because the join may still dissolve
	// into both groups, which then need their labels again.
	release := true
	passes := func(admit func(ga, gb *group, e *edge, f fitted) bool) {
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
				f := m.fitWithin(tol)
				if !f.ok || e.step/float64(e.pairs) > math.Max(3*f.slope, tol) || !admit(ga, gb, e, f) {
					continue
				}
				// Merge b into a: sums, flags, and b's neighbours become a's.
				if !ga.merged && !gb.merged {
					mergedGroups++
				} else if ga.merged && gb.merged && release {
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
	}
	passes(func(ga, gb *group, e *edge, f fitted) bool { return f.rms <= tol && sidesFollow(ga, gb, f) })
	// Snapshot the linear groups, then join across bends.
	r1 := make([]int32, ncomp)
	for i := range r1 {
		r1[i] = find(int32(i))
	}
	groups1 := slices.Clone(groups)
	release = false
	passes(func(ga, gb *group, e *edge, f fitted) bool { return sidesBend(ga, gb, f, tol) })
	r2 := make([]int32, ncomp)
	for i := range r2 {
		r2[i] = find(int32(i))
	}

	// Every merged group, and every lone band with enough interior
	// pixels that is linear within tol, is a ramp candidate; its colour
	// is sampled along its direction. A group joined across bends must
	// reproduce its pixels within tol or it dissolves into the linear
	// groups it was made of, which are then joined again pair by pair,
	// closest mean colours first, each join kept only when the model of
	// the union fits (at most maxReplay checks per dissolved group, so a
	// hub with thousands of members costs a bounded number of passes).
	// Labels: a group formed from lone bands only needs a label the join
	// pass has not reserved, so it forms only while the budget allows.
	ext := sample(rgb, comp, w, h, r2, groups, tol)
	fails := func(e *extent) bool { return e == nil || e.model == nil || e.err > float64(tol*tol)*float64(e.cnt) }
	var dissolved []int32
	for r, e := range ext {
		if groups[r].comps != groups1[r].comps && fails(e) {
			dissolved = append(dissolved, r)
		}
	}
	slices.Sort(dissolved)
	if len(dissolved) > 0 {
		isDissolved := map[int32]bool{}
		for _, r := range dissolved {
			isDissolved[r] = true
		}
		members := map[int32][]int32{} // dissolved root → its linear groups
		for i := range comps {
			if r := r2[i]; isDissolved[r] {
				delete(ext, r)
				if r1[i] == int32(i) {
					members[r] = append(members[r], int32(i))
				}
				r2[i] = r1[i]
				groups[r1[i]] = groups1[r1[i]]
			}
		}
		mergedNow := 0
		for i := range groups {
			if r2[i] == int32(i) && groups[i].merged {
				mergedNow++
			}
		}
		inGroup := make([]bool, ncomp) // per linear root, for the check
		for _, r := range dissolved {
			ms := members[r]
			slices.Sort(ms)
			isMember := map[int32]bool{}
			for _, m := range ms {
				isMember[m] = true
			}
			type pair struct {
				a, b  int32
				dmean float64
			}
			seen := map[[2]int32]bool{}
			var pairs []pair
			for _, k := range edgeKeys {
				a, b := r1[k[0]], r1[k[1]]
				if a == b || !isMember[a] || !isMember[b] {
					continue
				}
				if a > b {
					a, b = b, a
				}
				if seen[[2]int32{a, b}] {
					continue
				}
				seen[[2]int32{a, b}] = true
				pairs = append(pairs, pair{a: a, b: b, dmean: dist(groups1[a].m.mean(), groups1[b].m.mean())})
			}
			sort.Slice(pairs, func(i, j int) bool {
				if pairs[i].dmean != pairs[j].dmean {
					return pairs[i].dmean < pairs[j].dmean
				}
				if pairs[i].a != pairs[j].a {
					return pairs[i].a < pairs[j].a
				}
				return pairs[i].b < pairs[j].b
			})
			parent3 := map[int32]int32{}
			under := map[int32][]int32{}
			for _, m := range ms {
				parent3[m] = m
				under[m] = []int32{m}
			}
			find3 := func(i int32) int32 {
				for parent3[i] != i {
					i = parent3[i]
				}
				return i
			}
			checks := 0
			for _, pr := range pairs {
				ra, rb := find3(pr.a), find3(pr.b)
				if ra == rb {
					continue
				}
				if checks >= maxReplay {
					break
				}
				checks++
				if ra > rb {
					ra, rb = rb, ra
				}
				m := groups1[ra].m
				for _, x := range under[ra][1:] {
					m.merge(&groups1[x].m)
				}
				for _, x := range under[rb] {
					m.merge(&groups1[x].m)
				}
				for _, x := range under[ra] {
					inGroup[x] = true
				}
				for _, x := range under[rb] {
					inGroup[x] = true
				}
				ok := modelFits(rgb, comp, w, h, r1, inGroup, &m, tol)
				for _, x := range under[ra] {
					inGroup[x] = false
				}
				for _, x := range under[rb] {
					inGroup[x] = false
				}
				if !ok {
					continue
				}
				parent3[rb] = ra
				under[ra] = append(under[ra], under[rb]...)
				delete(under, rb)
			}
			var subRoots []int32
			for root, us := range under {
				if len(us) > 1 {
					subRoots = append(subRoots, root)
				}
			}
			slices.Sort(subRoots)
			for _, root := range subRoots {
				us := under[root]
				slices.Sort(us)
				lone := 0
				for _, m := range us {
					if !groups1[m].merged {
						lone++
					}
				}
				if lone == len(us) && mergedNow+1+n > maxLabels {
					continue // no label free for a new ramp
				}
				g := groups1[root]
				for _, m := range us[1:] {
					g.m.merge(&groups1[m].m)
					g.comps += groups1[m].comps
				}
				g.merged = true
				groups[root] = g
				for i := range comps {
					if slices.Contains(us, r1[i]) {
						r2[i] = root
					}
				}
				mergedNow += 1 - (len(us) - lone)
			}
		}
		// Sample what changed: the reverted linear groups and the new ones.
		for r, e := range sample(rgb, comp, w, h, r2, groups, tol) {
			if isDissolved[find(r)] {
				ext[r] = e
			}
		}
	}
	var roots []int32
	for i := range groups {
		if r2[i] == int32(i) {
			roots = append(roots, int32(i))
		}
	}
	sort.Slice(roots, func(i, j int) bool { return groups[roots[i]].first < groups[roots[j]].first })
	fresh := map[int32]int{}
	rampOf := map[int32]*Ramp{}
	// A lone band must also change colour end to end by its linear fit:
	// a pocket of another colour in its middle bumps the samples but is
	// no ramp.
	rampFor := func(r int32) *Ramp {
		e := ext[r]
		if e == nil || e.b == nil {
			return nil
		}
		if !groups[r].merged && float64(e.f.slope*(e.tmax-e.tmin)) < math.Max(tol, minSpan) {
			return nil
		}
		return rampFrom(&groups[r].m, e.f, e.b, tol)
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
		if l, ok := fresh[r2[i]]; ok {
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
			ramps = append(ramps, rampOf[r2[comp[p]]])
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

// sidesBend reports whether two groups may join across a bend along the
// union's direction: each side either spreads in colour by tol or more
// (a bent band, or a ramp) or, when the union's slope would change its
// colour by a printable step across it, carries at least half that
// slope along u; a flat side that is wide enough to tell refuses, and at
// least one side must vote.
func sidesBend(a, b *group, union fitted, tol float64) bool {
	voted := false
	for _, g := range []*group{a, b} {
		if g.m.spread() >= tol {
			voted = true
			continue
		}
		f := g.m.along(union.u)
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

// extent is a ramp candidate: its direction, the range of projections
// of its pixels on it, its colour samples and, for a joined group, the
// sampled model and the squared error of its interior pixels against it.
type extent struct {
	f          fitted
	tmin, tmax float64
	b          *bins
	model      []rawStop
	err        float64
	cnt        int
}

// sample measures every root group (root[i] per component) that may
// carry a ramp: a merged group, or a lone band with enough interior
// pixels that is linear within tol. The extent along u is measured over all the group's
// pixels; the colour is sampled from the pixels interior to the group
// (every 4-neighbour in the same group), so a band boundary's blend and
// the group's own anti-aliased edge do not pull a stop.
func sample(rgb []uint8, comp []int32, w, h int, root []int32, groups []group, tol float64) map[int32]*extent {
	np := w * h
	find := func(i int32) int32 { return root[i] }
	ext := map[int32]*extent{}
	for i := range groups {
		g := &groups[i]
		if find(int32(i)) != int32(i) || g.m.n < minPixels {
			continue
		}
		if f := g.m.fitWithin(tol); f.ok && (g.merged || f.rms <= tol) {
			ext[int32(i)] = &extent{f: f, tmin: math.Inf(1), tmax: math.Inf(-1)}
		}
	}
	proj := func(e *extent, p int) float64 {
		return float64(e.f.u[0]*(float64(p%w)+0.5)) + float64(e.f.u[1]*(float64(p/w)+0.5))
	}
	for p := 0; p < np; p++ {
		if e := ext[find(comp[p])]; e != nil {
			t := proj(e, p)
			e.tmin = math.Min(e.tmin, t)
			e.tmax = math.Max(e.tmax, t)
		}
	}
	for _, e := range ext {
		if e.tmax-e.tmin >= minLength {
			e.b = newBins(e.tmin, e.tmax)
		}
	}
	inside := func(p int, r int32) bool {
		x, y := p%w, p/w
		return (x == 0 || find(comp[p-1]) == r) && (x == w-1 || find(comp[p+1]) == r) &&
			(y == 0 || find(comp[p-w]) == r) && (y == h-1 || find(comp[p+w]) == r)
	}
	for p := 0; p < np; p++ {
		r := find(comp[p])
		if e := ext[r]; e != nil && e.b != nil && inside(p, r) {
			e.b.add(proj(e, p), colourOf(rgb, p))
		}
	}
	for r, e := range ext {
		if groups[r].merged && e.b != nil {
			e.model = e.b.model(tol)
		}
	}
	for p := 0; p < np; p++ {
		r := find(comp[p])
		e := ext[r]
		if e == nil || e.model == nil || !inside(p, r) {
			continue
		}
		d := dist(colourAt(e.model, (proj(e, p)-e.tmin)/(e.tmax-e.tmin)), colourOf(rgb, p))
		e.err += float64(d * d)
		e.cnt++
	}
	return ext
}

// modelFits reports whether the union of the linear groups flagged in
// in (per linear root, r1 per component) is reproduced within tol by a
// ramp sampled along the union's direction: the same measure sample
// applies to a joined group, over the union's interior pixels.
func modelFits(rgb []uint8, comp []int32, w, h int, r1 []int32, in []bool, m *moments, tol float64) bool {
	np := w * h
	f := m.fitWithin(tol)
	if !f.ok {
		return false
	}
	proj := func(p int) float64 {
		return float64(f.u[0]*(float64(p%w)+0.5)) + float64(f.u[1]*(float64(p/w)+0.5))
	}
	tmin, tmax := math.Inf(1), math.Inf(-1)
	for p := 0; p < np; p++ {
		if in[r1[comp[p]]] {
			t := proj(p)
			tmin = math.Min(tmin, t)
			tmax = math.Max(tmax, t)
		}
	}
	if tmax-tmin < minLength {
		return false
	}
	inside := func(p int) bool {
		x, y := p%w, p/w
		return (x == 0 || in[r1[comp[p-1]]]) && (x == w-1 || in[r1[comp[p+1]]]) &&
			(y == 0 || in[r1[comp[p-w]]]) && (y == h-1 || in[r1[comp[p+w]]])
	}
	b := newBins(tmin, tmax)
	for p := 0; p < np; p++ {
		if in[r1[comp[p]]] && inside(p) {
			b.add(proj(p), colourOf(rgb, p))
		}
	}
	model := b.model(tol)
	if model == nil {
		return false
	}
	var err float64
	cnt := 0
	for p := 0; p < np; p++ {
		if in[r1[comp[p]]] && inside(p) {
			d := dist(colourAt(model, (proj(p)-tmin)/(tmax-tmin)), colourOf(rgb, p))
			err += float64(d * d)
			cnt++
		}
	}
	return err <= float64(tol*tol)*float64(cnt)
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

// half is (a − b)/2, the central difference of two neighbours' colours.
func half(a, b [3]float64) [3]float64 {
	return [3]float64{(a[0] - b[0]) / 2, (a[1] - b[1]) / 2, (a[2] - b[2]) / 2}
}

func colourOf(rgb []uint8, p int) [3]float64 {
	return [3]float64{float64(rgb[3*p]) / 255, float64(rgb[3*p+1]) / 255, float64(rgb[3*p+2]) / 255}
}

func dist(a, b [3]float64) float64 {
	d0, d1, d2 := a[0]-b[0], a[1]-b[1], a[2]-b[2]
	return math.Sqrt(float64(d0*d0) + float64(d1*d1) + float64(d2*d2))
}
