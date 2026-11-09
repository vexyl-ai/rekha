package geom

import "math"

// MergeCollinear drops every vertex of a closed lattice polygon whose
// incoming and outgoing edges point the same way.
func MergeCollinear(poly []Point) []Point {
	return pick(poly, MergeCollinearIndices(poly))
}

// MergeCollinearIndices returns, in ascending order, the indices of the
// vertices MergeCollinear keeps. Simplification works on indices rather
// than coordinates because a loop may visit the same lattice point twice
// (two regions touching at a corner), so a coordinate does not identify a
// vertex.
func MergeCollinearIndices(poly []Point) []int {
	n := len(poly)
	out := make([]int, 0, n)
	if n < 3 {
		for i := range poly {
			out = append(out, i)
		}
		return out
	}
	for i := 0; i < n; i++ {
		prev, cur, next := poly[(i+n-1)%n], poly[i], poly[(i+1)%n]
		din := Point{cur.X - prev.X, cur.Y - prev.Y}
		dout := Point{next.X - cur.X, next.Y - cur.Y}
		if din.X*dout.Y-din.Y*dout.X == 0 && din.X*dout.X+din.Y*dout.Y > 0 {
			continue // same direction
		}
		out = append(out, i)
	}
	return out
}

// pick returns the points of poly at the given indices.
func pick(poly []Point, idx []int) []Point {
	out := make([]Point, len(idx))
	for k, i := range idx {
		out[k] = poly[i]
	}
	return out
}

// RemoveStaircases replaces each staircase of a closed lattice polygon by
// a single diagonal. A staircase is a maximal chain of vertices whose turns
// alternate left/right and whose segments form a digital straight line:
// every step segment is 1 px and the run segments between steps differ in
// length by at most one. The chain's first and last vertices are the ends
// of the straightened segment and are kept; its interior is dropped.
// Where two chains abut (two same-sign turns, as at a spike's apex) both
// boundary vertices survive, so sharp tips are preserved.
// maxStairDev is how far, in pixels, a crack vertex of a true digital line
// can lie from the least-squares line through its chain: at most half the
// one-pixel band the crack of a line occupies, with some slack.
const maxStairDev = 0.75

// lsqLine returns the centroid of pts and the unit direction of the total
// least-squares line through them.
func lsqLine(pts []Vec2) (c, d Vec2) {
	for _, p := range pts {
		c = c.Add(p)
	}
	c = c.Scale(1 / float64(len(pts)))
	var sxx, sxy, syy float64
	for _, p := range pts {
		v := p.Sub(c)
		sxx += float64(v.X * v.X)
		sxy += float64(v.X * v.Y)
		syy += float64(v.Y * v.Y)
	}
	theta := 0.5 * math.Atan2(2*sxy, sxx-syy)
	return c, Vec2{X: math.Cos(theta), Y: math.Sin(theta)}
}

func RemoveStaircases(poly []Point) []Point {
	return pick(poly, RemoveStaircasesIndices(poly))
}

// RemoveStaircasesIndices returns, in ascending order, the indices of the
// vertices RemoveStaircases keeps.
func RemoveStaircasesIndices(poly []Point) []int {
	n := len(poly)
	all := func() []int {
		out := make([]int, n)
		for i := range out {
			out[i] = i
		}
		return out
	}
	if n < 5 {
		return all()
	}
	segLen := make([]int, n)   // segment i: poly[i] → poly[i+1]
	segVert := make([]bool, n) // vertical?
	for i := 0; i < n; i++ {
		a, b := poly[i], poly[(i+1)%n]
		dx, dy := b.X-a.X, b.Y-a.Y
		if dx < 0 {
			dx = -dx
		}
		if dy < 0 {
			dy = -dy
		}
		segLen[i] = dx + dy
		segVert[i] = dy > 0
	}
	sign := make([]int, n)
	for i := 0; i < n; i++ {
		prev, cur, next := poly[(i+n-1)%n], poly[i], poly[(i+1)%n]
		c := (cur.X-prev.X)*(next.Y-cur.Y) - (cur.Y-prev.Y)*(next.X-cur.X)
		switch {
		case c > 0:
			sign[i] = 1
		case c < 0:
			sign[i] = -1
		}
	}
	alternates := func(i int) bool { // vertex i continues a chain from i-1
		p := (i + n - 1) % n
		return sign[i] != 0 && sign[p] != 0 && sign[i] == -sign[p]
	}
	isolated := func(i int) bool { // vertex i is in no alternating pair
		return !alternates(i) && !alternates((i+1)%n)
	}
	// straight reports whether the segments between chain vertices
	// first..first+count-1 (count vertices) form a digital line, trying
	// both step orientations: step segments are 1 px, interior run
	// segments differ in length by at most one, and the two end segments
	// may be partial runs no longer than the interior ones. The run-length
	// rule alone accepts a 45° stretch of a circle (1- and 2-long runs in
	// any order), so every chain vertex must also lie within maxStairDev
	// of the least-squares line through the chain.
	straight := func(first, count int) bool {
		if count < 2 {
			return true
		}
		if count >= 3 {
			chain := make([]Vec2, count)
			for k := range chain {
				chain[k] = poly[(first+k)%n].Vec()
			}
			c, d := lsqLine(chain)
			for _, v := range chain {
				if PerpDist(v, c, c.Add(d)) > maxStairDev {
					return false
				}
			}
		}
		for _, stepVert := range []bool{false, true} {
			ok := true
			minRun, maxRun := 1<<30, 0
			var endRuns []int
			for k := 0; k < count-1 && ok; k++ {
				s := (first + k) % n
				switch {
				case segVert[s] == stepVert:
					ok = segLen[s] == 1
				case k == 0 || k == count-2:
					endRuns = append(endRuns, segLen[s])
				default:
					minRun = min(minRun, segLen[s])
					maxRun = max(maxRun, segLen[s])
					ok = maxRun-minRun <= 1
				}
			}
			for _, r := range endRuns {
				if maxRun > 0 && r > maxRun {
					ok = false
				}
			}
			if ok {
				return true
			}
		}
		return false
	}

	// chainDir is the unit least-squares direction of chain vertices
	// first..first+count-1.
	chainDir := func(first, count int) Vec2 {
		chain := make([]Vec2, count)
		for k := range chain {
			chain[k] = poly[(first+k)%n].Vec()
		}
		_, d := lsqLine(chain)
		return d
	}
	// backAlong reports whether the boundary beyond corner c, walking in
	// direction step, runs parallel to d: it follows up to four vertices
	// whose segments are no longer than the chain's longest run plus one
	// (longer ones belong to a real edge, not a parallel staircase) and
	// compares the direction reached with d.
	backAlong := func(c, step, first, count int, d Vec2) bool {
		longest := 0
		for k := 0; k < count-1; k++ {
			longest = max(longest, segLen[(first+k)%n])
		}
		j := c
		for k := 0; k < 4; k++ {
			seg := j // segment j → j+1
			if step < 0 {
				seg = (j + n - 1) % n
			}
			if segLen[seg] > longest+1 {
				break
			}
			j = ((j+step)%n + n) % n
		}
		far := poly[j].Vec().Sub(poly[c].Vec())
		if far == (Vec2{}) {
			return false
		}
		return math.Abs(far.Norm().Dot(d)) > 0.9
	}

	start := -1
	for i := 0; i < n; i++ {
		if !alternates(i) {
			start = i
			break
		}
	}
	if start < 0 {
		return all() // one endless staircase: leave it
	}

	drop := make([]bool, n)
	for k := 0; k < n; {
		i := (start + k) % n
		if sign[i] == 0 {
			k++
			continue
		}
		count := 1
		for k+count < n && alternates((i+count)%n) && straight(i, count+1) {
			count++
		}
		if count < 3 { // two alternating turns are a notch, not a staircase
			k += max(1, count-1)
			continue
		}
		// A chain bounded by a vertex that belongs to no chain (a true
		// corner) extends to that corner when the digital line still
		// holds, so no 1 px stub is left between them. The corner itself is
		// always kept, and abutting chains never extend into each other.
		// Nor does a chain extend into a corner whose far side runs back
		// along the chain: that is the end of a one-pixel-wide sliver, and
		// extending from both sides would collapse it to a chord.
		lo, cnt := i, count
		d := chainDir(i, count)
		if prev := (i + n - 1) % n; isolated(prev) && !backAlong(prev, -1, i, count, d) && straight(prev, cnt+1) {
			lo, cnt = prev, cnt+1
		}
		if next := (lo + cnt) % n; isolated(next) && !backAlong(next, +1, i, count, d) && straight(lo, cnt+1) {
			cnt++
		}
		for m := 1; m < cnt-1; m++ {
			drop[(lo+m)%n] = true
		}
		// The last vertex may begin the next chain; resume there.
		k += count - 1
	}
	out := make([]int, 0, n)
	for i := 0; i < n; i++ {
		if !drop[i] {
			out = append(out, i)
		}
	}
	return out
}

// Simplify runs MergeCollinear, RemoveStaircases and RDPClosed with tol.
func Simplify(poly []Point, tol float64) []Vec2 {
	idx := SimplifyIndices(poly, tol)
	v := make([]Vec2, len(idx))
	for k, i := range idx {
		v[k] = poly[i].Vec()
	}
	return v
}

// SimplifyIndices returns, in ascending order, the indices into poly of
// the vertices Simplify keeps.
func SimplifyIndices(poly []Point, tol float64) []int {
	merged := MergeCollinearIndices(poly)
	dss := RemoveStaircasesIndices(pick(poly, merged))
	for k, i := range dss {
		dss[k] = merged[i]
	}
	v := make([]Vec2, len(dss))
	for k, i := range dss {
		v[k] = poly[i].Vec()
	}
	rdp := RDPClosedIndices(v, tol)
	for k, i := range rdp {
		rdp[k] = dss[i]
	}
	return rdp
}
