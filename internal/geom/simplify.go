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
// a single diagonal. A staircase is a maximal chain of at least three
// consecutive vertices whose turns alternate left/right and whose adjacent
// segments are all at most 2 px long; the chain's interior vertices are
// dropped so the segment runs straight from the vertex before the chain to
// the vertex after it.
func RemoveStaircases(poly []Point) []Point {
	n := len(poly)
	if n < 5 {
		return append([]Point(nil), poly...)
	}
	segShort := make([]bool, n) // segment i: poly[i] → poly[i+1]
	for i := 0; i < n; i++ {
		a, b := poly[i], poly[(i+1)%n]
		dx, dy := b.X-a.X, b.Y-a.Y
		segShort[i] = dx*dx+dy*dy <= 4
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
	// A vertex can sit inside a chain when both its segments are short.
	inChain := func(i int) bool { return segShort[(i+n-1)%n] && segShort[i] && sign[i] != 0 }
	// continues reports whether vertex i can follow vertex i-1 in a chain.
	continues := func(i int) bool { return inChain(i) && inChain((i+n-1)%n) && sign[i] == -sign[(i+n-1)%n] }

	// Find a vertex that starts no continuation; if every vertex continues
	// the loop is one endless staircase (a diamond) and is left alone.
	start := -1
	for i := 0; i < n; i++ {
		if !continues(i) {
			start = i
			break
		}
	}
	if start < 0 {
		return append([]Point(nil), poly...)
	}

	drop := make([]bool, n)
	for k := 0; k < n; {
		i := (start + k) % n
		if !inChain(i) {
			k++
			continue
		}
		// Chain begins at i; extend while the next vertex continues it.
		j := i
		length := 1
		for length < n && continues((j+1)%n) {
			j = (j + 1) % n
			length++
		}
		if length >= 3 {
			for m := 0; m < length; m++ {
				drop[(i+m)%n] = true
			}
		}
		k += length
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
