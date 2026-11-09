// Package region cleans a per-pixel label map before tracing: specks and
// anti-aliasing slivers are merged into their neighbours and the palette
// is compacted.
package region

import "sort"

// neighbour is a label adjacent to a component and the length of the
// shared border, in 4-adjacent pixel pairs.
type neighbour struct {
	label  uint8
	border int
}

// component is one 4-connected run of equal labels.
type component struct {
	label      uint8
	pixels     []int32 // pixel indices, ascending
	perimeter  int     // boundary sides: pairs with other labels plus image edges
	neighbours []neighbour
}

// minorityConnects reports whether, at a 2×2 checkerboard of labels c and
// d, c is the colour that stays connected across the corner: the one with
// fewer pixels in the image, ties to the lower label. Tracing resolves
// saddles by the same rule, so a one-pixel diagonal line is one component
// here and one shape there.
func minorityConnects(c, d uint8, counts []int) bool {
	return counts[c] < counts[d] || (counts[c] == counts[d] && c < d)
}

// components labels the connected components of labels (w×h) with
// union–find and returns them in scan order of their first pixel, each
// with its neighbours sorted by border length descending then label
// ascending. Pixels connect across edges, and across a corner when the
// tracer would connect them there: at a two-colour checkerboard the
// minority colour (minorityConnects), at three colours the diagonal pair.
// Perimeters and borders count edges only.
func components(labels []uint8, w, h int) []component {
	n := w * h
	counts := make([]int, 256)
	for _, l := range labels {
		counts[l]++
	}
	parent := make([]int32, n)
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
		if ra < rb {
			parent[rb] = ra
		} else {
			parent[ra] = rb
		}
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := y*w + x
			if x+1 < w && labels[i] == labels[i+1] {
				union(int32(i), int32(i+1))
			}
			if y+1 < h && labels[i] == labels[i+w] {
				union(int32(i), int32(i+w))
			}
			if x+1 < w && y+1 < h {
				a, b, c, d := labels[i], labels[i+1], labels[i+w], labels[i+w+1]
				switch {
				case a == d && b == c && a != b:
					if minorityConnects(a, b, counts) {
						union(int32(i), int32(i+w+1))
					} else {
						union(int32(i+1), int32(i+w))
					}
				case a == d && a != b && a != c:
					union(int32(i), int32(i+w+1))
				case b == c && b != a && b != d:
					union(int32(i+1), int32(i+w))
				}
			}
		}
	}
	// Component ids in scan order of roots (roots are the smallest index).
	id := make([]int32, n)
	for i := range id {
		id[i] = -1
	}
	var comps []component
	for i := 0; i < n; i++ {
		r := find(int32(i))
		if id[r] < 0 {
			id[r] = int32(len(comps))
			comps = append(comps, component{label: labels[i]})
		}
		c := &comps[id[r]]
		c.pixels = append(c.pixels, int32(i))
	}
	// Perimeter and neighbour borders.
	type key struct {
		comp  int32
		label uint8
	}
	borders := map[key]int{}
	side := func(i int, nx, ny int) {
		c := id[find(int32(i))]
		if nx < 0 || ny < 0 || nx >= w || ny >= h {
			comps[c].perimeter++
			return
		}
		j := ny*w + nx
		if labels[j] != labels[i] {
			comps[c].perimeter++
			borders[key{c, labels[j]}]++
		}
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := y*w + x
			side(i, x-1, y)
			side(i, x+1, y)
			side(i, x, y-1)
			side(i, x, y+1)
		}
	}
	for k, b := range borders {
		comps[k.comp].neighbours = append(comps[k.comp].neighbours, neighbour{k.label, b})
	}
	for i := range comps {
		nb := comps[i].neighbours
		sort.Slice(nb, func(a, b int) bool {
			if nb[a].border != nb[b].border {
				return nb[a].border > nb[b].border
			}
			return nb[a].label < nb[b].label
		})
	}
	return comps
}
