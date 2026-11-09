package rekha

import (
	"fmt"
	"math"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/vexyl-ai/rekha/internal/bitmap"
	"github.com/vexyl-ai/rekha/internal/colour"
	"github.com/vexyl-ai/rekha/internal/contour"
	"github.com/vexyl-ai/rekha/internal/geom"
	"github.com/vexyl-ai/rekha/internal/svg"
)

// canvasWhite is the OKLab distance within which a depth-0 shape counts as
// the white page and is left out: SVG's transparent background renders
// white, and a logo traced from a transparent PNG stays transparent. 0.02
// is about one just-noticeable difference, the drift an anti-aliased
// background gives its centroid.
const canvasWhite = 0.02

// layer is one shape of one colour queued for output. contours is what is
// emitted: the outer alone when stacked (possibly several once touching
// siblings are merged), outer and holes when cut out.
type layer struct {
	colour   int
	shape    contour.Shape
	depth    int
	omit     bool // the white page: not emitted, not extended into
	contours []contour.Contour
}

// Finish traces every colour of s, orders the shapes (stacked: nesting
// depth, outer contours only, consecutive paths of one colour sharing a
// <path>; cutout: colour then scan order, holes cut out, one path per
// shape), fits each contour under o.Mode and returns the paths with their
// palette fills. White shapes at depth 0 are left out.
func (s *Segmentation) Finish(o Options) (*Result, error) {
	if err := o.Validate(); err != nil {
		return nil, err
	}
	res := &Result{Width: s.Width, Height: s.Height}
	if s.Width*s.Height == 0 || len(s.Palette) == 0 {
		return res, nil
	}
	if len(s.Labels) != s.Width*s.Height {
		return nil, fmt.Errorf("rekha: %d labels for a %d×%d segmentation", len(s.Labels), s.Width, s.Height)
	}

	layers := s.layers(o)
	stacked := o.Hierarchical == HierarchicalStacked && o.Preset != PresetBW
	shapes := make([]contour.Shape, len(layers))
	for i, l := range layers {
		shapes[i] = l.shape
	}
	parents := contour.Nest(shapes)
	for i := range layers {
		layers[i].depth = depthOf(parents, i)
		layers[i].contours = []contour.Contour{layers[i].shape.Outer}
		if !stacked {
			layers[i].contours = append(layers[i].contours, layers[i].shape.Holes...)
		}
	}
	// Drop the white page: depth-0 shapes sit straight on the background,
	// which renders white, so leaving the white ones out changes nothing
	// and keeps a transparent input transparent.
	omit := make([]bool, len(layers))
	for i, l := range layers {
		omit[i] = l.depth == 0 && s.nearWhite(l.colour)
	}
	if stacked {
		s.extendSiblings(layers, parents, omit, o)
		sort.SliceStable(layers, func(a, b int) bool {
			if layers[a].depth != layers[b].depth {
				return layers[a].depth < layers[b].depth
			}
			return pointLess(layers[a].shape.Outer.Pts[0], layers[b].shape.Outer.Pts[0])
		})
	}

	kept := layers[:0]
	for _, l := range layers {
		if !l.omit {
			kept = append(kept, l)
		}
	}
	layers = kept

	// Fit every contour in parallel, indexed by layer and subpath.
	subpaths := make([][]svg.Subpath, len(layers))
	var jobs [][2]int
	for i, l := range layers {
		subpaths[i] = make([]svg.Subpath, len(l.contours))
		for k := range l.contours {
			jobs = append(jobs, [2]int{i, k})
		}
	}
	parallel(len(jobs), func(j int) {
		i, k := jobs[j][0], jobs[j][1]
		subpaths[i][k] = svg.Subpath{Segs: segments(layers[i].contours[k].Pts, o)}
	})

	mergeable := false
	for i, l := range layers {
		fill := colour.FromSRGB8(s.Palette[l.colour].R, s.Palette[l.colour].G, s.Palette[l.colour].B).Hex()
		// Stacked outers of one colour share a <path>; cutout keeps one
		// path per shape, as Phase 1 did. A layer with holes (an extended
		// region around a pocket) stays alone: a later shape of the same
		// colour inside the pocket would cancel under nonzero winding.
		outersOnly := true
		for _, c := range l.contours {
			if c.Area2 < 0 {
				outersOnly = false
			}
		}
		if n := len(res.Paths); stacked && mergeable && outersOnly && n > 0 && res.Paths[n-1].Fill == fill {
			res.Paths[n-1].Subpaths = append(res.Paths[n-1].Subpaths, subpaths[i]...)
			continue
		}
		res.Paths = append(res.Paths, svg.Path{Fill: fill, Subpaths: subpaths[i]})
		mergeable = stacked && outersOnly
	}
	return res, nil
}

// layers traces each colour's label mask into shapes, colours in a worker
// pool each owning one bitmap, and returns them in colour then scan order.
// The bw preset traces foreground only, with Phase 1's despeckling.
func (s *Segmentation) layers(o Options) []layer {
	colours := make([]int, 0, len(s.Palette))
	minArea := 0
	if o.Preset == PresetBW {
		colours, minArea = append(colours, 1), o.FilterSpeckle
	} else {
		for c := range s.Palette {
			colours = append(colours, c)
		}
	}
	perColour := make([][]contour.Shape, len(colours))
	counts := make([]int, len(s.Palette))
	for _, l := range s.Labels {
		counts[l]++
	}
	var bufs sync.Pool
	parallel(len(colours), func(i int) {
		bm, _ := bufs.Get().(*bitmap.Bitmap)
		if bm == nil {
			bm = bitmap.New(s.Width, s.Height)
		} else {
			bm.Clear()
		}
		label := uint8(colours[i])
		for y := 0; y < s.Height; y++ {
			row := s.Labels[y*s.Width : (y+1)*s.Width]
			for x, l := range row {
				if l == label {
					bm.Set(x, y, true)
				}
			}
		}
		if o.Preset == PresetBW {
			perColour[i] = contour.BuildShapes(contour.Trace(bm, o.TurnPolicy), minArea)
		} else {
			perColour[i] = contour.BuildShapes(contour.TraceFunc(bm, s.saddle(label, counts)), minArea)
		}
		bufs.Put(bm)
	})
	var out []layer
	for i, shapes := range perColour {
		for _, sh := range shapes {
			out = append(out, layer{colour: colours[i], shape: sh})
		}
	}
	return out
}

// extendSiblings removes the seams stacked output would show between
// abutting shapes of one parent: where two siblings share a pixel border,
// each is painted with an anti-aliased edge and the background bleeds
// through the join. Every shape's region is therefore widened by a few
// pixels into the siblings painted after it, so its colour runs under the
// later one's edge; the later sibling still covers its own area opaquely,
// so nothing else shows. The margin exceeds twice the fitting tolerance,
// the most the two fitted edges can drift apart.
func (s *Segmentation) extendSiblings(layers []layer, parents []int, omit []bool, o Options) {
	w, h := s.Width, s.Height
	n := len(layers)
	for i := range layers {
		layers[i].omit = omit[i]
	}
	// Growth is in 4-neighbour steps (diagonal steps would bridge shapes
	// that only touch at a corner), so along a 45° edge the margin covers
	// 1/√2 of its length: it must still clear twice the tolerance.
	margin := int(math.Ceil(2*math.Sqrt2*math.Max(o.FitTolerance, o.SimplifyTolerance))) + 1

	// Innermost shape per pixel: fill outers largest first.
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		return layers[order[a]].shape.Outer.Area2 > layers[order[b]].shape.Outer.Area2
	})
	ids := make([]int32, w*h)
	for i := range ids {
		ids[i] = -1
	}
	for _, i := range order {
		fillOuter(layers[i].shape.Outer.Pts, w, h, func(x, y int) { ids[y*w+x] = int32(i) })
	}
	// Painting position of each shape among its siblings.
	rank := make([]int, n)
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		return pointLess(layers[order[a]].shape.Outer.Pts[0], layers[order[b]].shape.Outer.Pts[0])
	})
	for r, i := range order {
		rank[i] = r
	}
	// ancestorAt returns the shape at depth d on the chain of shape id, or
	// -1 when id is shallower.
	ancestorAt := func(id int32, d int) int {
		a := int(id)
		for a >= 0 && layers[a].depth > d {
			a = parents[a]
		}
		if a < 0 || layers[a].depth != d {
			return -1
		}
		return a
	}
	// Pixels inside each shape's outer, as flood-fill seeds.
	pixels := make([][]int32, n)
	for p, id := range ids {
		for a := int(id); a >= 0; a = parents[a] {
			pixels[a] = append(pixels[a], int32(p))
		}
	}

	stamp := make([]int32, w*h)
	var frontier, next, region []int32
	for k := range layers {
		if omit[k] {
			continue
		}
		// Grow from k's pixels, margin steps, into pixels whose sibling at
		// k's depth is painted after k.
		mark := int32(k + 1)
		frontier = append(frontier[:0], pixels[k]...)
		region = append(region[:0], pixels[k]...)
		for _, p := range frontier {
			stamp[p] = mark
		}
		grown := false
		for step := 0; step < margin && len(frontier) > 0; step++ {
			next = next[:0]
			for _, p := range frontier {
				x, y := int(p)%w, int(p)/w
				for _, d := range [4][2]int{{1, 0}, {0, 1}, {-1, 0}, {0, -1}} {
					qx, qy := x+d[0], y+d[1]
					if qx < 0 || qy < 0 || qx >= w || qy >= h {
						continue
					}
					q := int32(qy*w + qx)
					if stamp[q] == mark {
						continue
					}
					stamp[q] = mark
					a := ancestorAt(ids[q], layers[k].depth)
					if a < 0 || omit[a] || parents[a] != parents[k] || rank[a] <= rank[k] {
						continue
					}
					region = append(region, q)
					next = append(next, q)
					grown = true
				}
			}
			frontier, next = next, frontier
		}
		if !grown {
			continue
		}
		layers[k].contours = traceRegion(region, w, o.TurnPolicy)
	}
}

// traceRegion traces a pixel set (indices into a w-wide image) on a bitmap
// cropped to its bounds and returns its contours, each outer followed by
// its holes. The set is a filled outer plus a band grown into later
// siblings, so any hole is a pocket of pixels painted earlier or not at
// all, which the band must not cover.
func traceRegion(region []int32, w int, policy contour.TurnPolicy) []contour.Contour {
	x0, y0, x1, y1 := math.MaxInt, math.MaxInt, 0, 0
	for _, p := range region {
		x, y := int(p)%w, int(p)/w
		x0, y0, x1, y1 = min(x0, x), min(y0, y), max(x1, x), max(y1, y)
	}
	bm := bitmap.New(x1-x0+1, y1-y0+1)
	for _, p := range region {
		bm.Set(int(p)%w-x0, int(p)/w-y0, true)
	}
	var cs []contour.Contour
	for _, sh := range contour.BuildShapes(contour.Trace(bm, policy), 0) {
		cs = append(append(cs, sh.Outer), sh.Holes...)
	}
	for _, c := range cs {
		for i := range c.Pts {
			c.Pts[i].X += x0
			c.Pts[i].Y += y0
		}
	}
	return cs
}

// fillOuter calls set for every pixel inside the crack polygon pts under
// the even-odd rule, by bucketing its vertical edges per row.
func fillOuter(pts []geom.Point, w, h int, set func(x, y int)) {
	y0, y1 := h, 0
	for _, p := range pts {
		y0, y1 = min(y0, p.Y), max(y1, p.Y)
	}
	y0, y1 = max(y0, 0), min(y1, h)
	if y0 >= y1 {
		return
	}
	rows := make([][]int, y1-y0)
	for i, a := range pts {
		b := pts[(i+1)%len(pts)]
		if a.X != b.X {
			continue
		}
		lo, hi := max(min(a.Y, b.Y), y0), min(max(a.Y, b.Y), y1)
		for y := lo; y < hi; y++ {
			rows[y-y0] = append(rows[y-y0], a.X)
		}
	}
	for i, xs := range rows {
		sort.Ints(xs)
		for k := 0; k+1 < len(xs); k += 2 {
			for x := max(xs[k], 0); x < min(xs[k+1], w); x++ {
				set(x, y0+i)
			}
		}
	}
}

// saddle returns the per-vertex saddle rule for tracing colour c: at a
// 2×2 checkerboard of c against one other colour d, the colour with fewer
// pixels in the image (counts; ties to the lower palette index) stays
// connected and the other is cut, so the two colours' contours never both
// claim the corner and every pixel lies inside exactly one shape. The
// minority is the thin feature: a one-pixel diagonal line stays one shape
// on any background. Against two different colours, c connects. Cleanup
// joins components by the same rule (region.minorityConnects).
func (s *Segmentation) saddle(c uint8, counts []int) func(x, y int) bool {
	w := s.Width
	return func(x, y int) bool {
		nw, ne := s.Labels[(y-1)*w+x-1], s.Labels[(y-1)*w+x]
		sw, se := s.Labels[y*w+x-1], s.Labels[y*w+x]
		var d1, d2 uint8
		if nw == c {
			d1, d2 = ne, sw
		} else {
			d1, d2 = nw, se
		}
		return d1 != d2 || counts[c] < counts[d1] || (counts[c] == counts[d1] && c < d1)
	}
}

// depthOf counts the ancestors of shape i.
func depthOf(parents []int, i int) int {
	d := 0
	for p := parents[i]; p >= 0; p = parents[p] {
		d++
	}
	return d
}

// nearWhite reports whether palette entry i is within canvasWhite of white.
func (s *Segmentation) nearWhite(i int) bool {
	c := s.Palette[i]
	return colour.Dist(colour.FromSRGB8(c.R, c.G, c.B), colour.FromSRGB8(255, 255, 255)) <= canvasWhite
}

func pointLess(a, b geom.Point) bool {
	if a.Y != b.Y {
		return a.Y < b.Y
	}
	return a.X < b.X
}

// parallel runs fn(0..n-1) on GOMAXPROCS workers and waits. Callers index
// results by i, so output never depends on scheduling.
func parallel(n int, fn func(i int)) {
	workers := min(runtime.GOMAXPROCS(0), n)
	if workers <= 1 {
		for i := 0; i < n; i++ {
			fn(i)
		}
		return
	}
	var next atomic.Int64
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				i := int(next.Add(1)) - 1
				if i >= n {
					return
				}
				fn(i)
			}
		}()
	}
	wg.Wait()
}
