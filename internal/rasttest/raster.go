// Package rasttest is a test-only even-odd scanline filler for paths made of
// straight segments. It samples pixel centres, which is exact for the
// pixel-mode output whose vertices all lie on the lattice.
package rasttest

import (
	"sort"

	"github.com/vexyl-ai/rekha/internal/bitmap"
	"github.com/vexyl-ai/rekha/internal/svg"
)

// Fill rasterises paths into a w×h bitmap. Each path is filled even-odd on
// its own and the results are OR-ed together.
func Fill(w, h int, paths []svg.Path) *bitmap.Bitmap {
	out := bitmap.New(w, h)
	for _, p := range paths {
		for y := 0; y < h; y++ {
			yc := float64(y) + 0.5
			var xs []float64
			for _, sp := range p.Subpaths {
				for _, s := range sp.Segs {
					a, b := s.P[0], s.P[3]
					if (a.Y <= yc) == (b.Y <= yc) {
						continue
					}
					xs = append(xs, a.X+(yc-a.Y)*(b.X-a.X)/(b.Y-a.Y))
				}
			}
			sort.Float64s(xs)
			for i := 0; i+1 < len(xs); i += 2 {
				for x := 0; x < w; x++ {
					xc := float64(x) + 0.5
					if xc > xs[i] && xc < xs[i+1] {
						out.Set(x, y, true)
					}
				}
			}
		}
	}
	return out
}
