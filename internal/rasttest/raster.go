// Package rasttest is a test-only nonzero-winding scanline filler, the
// rule SVG applies by default. It samples pixel centres, which is exact
// for the pixel-mode output whose vertices all lie on the lattice; cubics
// are flattened into flatten chords.
package rasttest

import (
	"sort"

	"github.com/vexyl-ai/rekha/internal/bitmap"
	"github.com/vexyl-ai/rekha/internal/fit"
	"github.com/vexyl-ai/rekha/internal/geom"
	"github.com/vexyl-ai/rekha/internal/svg"
)

// Fill rasterises paths into a w×h bitmap. Each path is filled on its own
// and the results are OR-ed together.
func Fill(w, h int, paths []svg.Path) *bitmap.Bitmap {
	out := bitmap.New(w, h)
	for _, p := range paths {
		scan(w, h, p, func(x, y int) { out.Set(x, y, true) })
	}
	return out
}

// Paint fills each path in order with palette[p.Fill], later paths
// covering earlier ones, and returns one index per pixel; pixels no path
// covers are 255.
func Paint(w, h int, paths []svg.Path, palette map[string]uint8) []uint8 {
	out := make([]uint8, w*h)
	for i := range out {
		out[i] = 255
	}
	for _, p := range paths {
		idx := palette[p.Fill]
		scan(w, h, p, func(x, y int) { out[y*w+x] = idx })
	}
	return out
}

// crossing is where an edge crosses a scanline and in which direction.
type crossing struct {
	x    float64
	wind int
}

// scan calls set for every pixel whose centre is inside p under the
// nonzero winding rule.
func scan(w, h int, p svg.Path, set func(x, y int)) {
	for y := 0; y < h; y++ {
		yc := float64(y) + 0.5
		var xs []crossing
		for _, sp := range p.Subpaths {
			for _, s := range sp.Segs {
				for _, c := range chords(s) {
					a, b := c[0], c[1]
					if (a.Y <= yc) == (b.Y <= yc) {
						continue
					}
					wind := 1
					if b.Y < a.Y {
						wind = -1
					}
					xs = append(xs, crossing{a.X + (yc-a.Y)*(b.X-a.X)/(b.Y-a.Y), wind})
				}
			}
		}
		sort.Slice(xs, func(i, j int) bool { return xs[i].x < xs[j].x })
		wind := 0
		for i := 0; i+1 < len(xs); i++ {
			wind += xs[i].wind
			if wind == 0 {
				continue
			}
			for x := 0; x < w; x++ {
				xc := float64(x) + 0.5
				if xc > xs[i].x && xc < xs[i+1].x {
					set(x, y)
				}
			}
		}
	}
}

const flatten = 16

// chords returns the segment as straight pieces: one for a line, flatten
// for a cubic.
func chords(s fit.Segment) [][2]geom.Vec2 {
	if s.Line {
		return [][2]geom.Vec2{{s.P[0], s.P[3]}}
	}
	out := make([][2]geom.Vec2, 0, flatten)
	prev := s.P.At(0)
	for i := 1; i <= flatten; i++ {
		next := s.P.At(float64(i) / flatten)
		out = append(out, [2]geom.Vec2{prev, next})
		prev = next
	}
	return out
}
