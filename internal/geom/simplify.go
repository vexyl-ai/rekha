package geom

// MergeCollinear drops every vertex of a closed lattice polygon whose
// incoming and outgoing edges point the same way.
func MergeCollinear(poly []Point) []Point {
	n := len(poly)
	if n < 3 {
		return append([]Point(nil), poly...)
	}
	out := make([]Point, 0, n)
	for i := 0; i < n; i++ {
		prev, cur, next := poly[(i+n-1)%n], poly[i], poly[(i+1)%n]
		din := Point{cur.X - prev.X, cur.Y - prev.Y}
		dout := Point{next.X - cur.X, next.Y - cur.Y}
		if din.X*dout.Y-din.Y*dout.X == 0 && din.X*dout.X+din.Y*dout.Y > 0 {
			continue // same direction
		}
		out = append(out, cur)
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
func RemoveStaircases(poly []Point) []Point {
	n := len(poly)
	if n < 5 {
		return append([]Point(nil), poly...)
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
	// may be partial runs no longer than the interior ones.
	straight := func(first, count int) bool {
		if count < 2 {
			return true
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

	start := -1
	for i := 0; i < n; i++ {
		if !alternates(i) {
			start = i
			break
		}
	}
	if start < 0 {
		return append([]Point(nil), poly...) // one endless staircase: leave it
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
		lo, cnt := i, count
		if prev := (i + n - 1) % n; isolated(prev) && straight(prev, cnt+1) {
			lo, cnt = prev, cnt+1
		}
		if next := (lo + cnt) % n; isolated(next) && straight(lo, cnt+1) {
			cnt++
		}
		for m := 1; m < cnt-1; m++ {
			drop[(lo+m)%n] = true
		}
		// The last vertex may begin the next chain; resume there.
		k += count - 1
	}
	out := make([]Point, 0, n)
	for i := 0; i < n; i++ {
		if !drop[i] {
			out = append(out, poly[i])
		}
	}
	return out
}

// Simplify runs MergeCollinear, RemoveStaircases and RDPClosed with tol.
func Simplify(poly []Point, tol float64) []Vec2 {
	p := RemoveStaircases(MergeCollinear(poly))
	v := make([]Vec2, len(p))
	for i, pt := range p {
		v[i] = pt.Vec()
	}
	return RDPClosed(v, tol)
}
