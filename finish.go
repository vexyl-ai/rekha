package rekha

import (
	"fmt"
	"math"
	"sort"
	"sync"

	"github.com/vexyl-ai/rekha/internal/bitmap"
	"github.com/vexyl-ai/rekha/internal/colour"
	"github.com/vexyl-ai/rekha/internal/contour"
	"github.com/vexyl-ai/rekha/internal/fit"
	"github.com/vexyl-ai/rekha/internal/geom"
	"github.com/vexyl-ai/rekha/internal/gradient"
	"github.com/vexyl-ai/rekha/internal/par"
	"github.com/vexyl-ai/rekha/internal/planar"
	"github.com/vexyl-ai/rekha/internal/refine"
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
	band     []int32 // pixels the extension added, sorted; nil when none
	loops    []int   // planar loop of each contour, when traced through the map
}

// sharedRounds is how many Jacobi rounds refineShared runs. A second
// round was measured on the seam synthetics and gained nothing.
const sharedRounds = 1

// arcFit is the fitted curve of one planar arc, shared by the two shapes
// along it: one gets it forwards, the other reversed.
type arcFit struct{ segs []fit.Segment }

// usePlanar reports whether the shapes are traced through the shared-
// boundary map: colour cutout in spline mode. Stacked output overlaps its
// shapes instead, pixel and polygon modes are exact already, and the bw
// preset has one colour.
func usePlanar(o Options) bool {
	return o.Hierarchical == HierarchicalCutout && o.Preset != PresetBW && o.Mode == ModeSpline
}

// inBand reports whether pixel index p lies in the layer's extension band.
func (l *layer) inBand(p int) bool {
	i := sort.Search(len(l.band), func(i int) bool { return l.band[i] >= int32(p) })
	return i < len(l.band) && l.band[i] == int32(p)
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
	if o.Stroke == 0 {
		o.Stroke = o.CutoutStroke
	}
	res := &Result{Width: s.Width, Height: s.Height}
	if s.Width*s.Height == 0 || len(s.Palette) == 0 {
		return res, nil
	}
	if len(s.Labels) != s.Width*s.Height {
		return nil, fmt.Errorf("rekha: %d labels for a %d×%d segmentation", len(s.Labels), s.Width, s.Height)
	}

	layers, pm := s.buildLayersMap(o)
	stacked := o.Hierarchical == HierarchicalStacked && o.Preset != PresetBW

	// With the planar map every arc is fitted once, closed island loops
	// as loops, and the shapes assemble their outlines from the fits.
	var fits []arcFit
	var jpos map[geom.Point]geom.Vec2
	if pm != nil {
		preps := make([]arcPrep, len(pm.Arcs))
		parallel(len(pm.Arcs), func(i int) {
			if a := pm.Arcs[i]; !a.Closed {
				preps[i] = prepareArc(a.Pts, o)
			}
		})
		s.shiftFlanks(pm, preps)
		jpos = junctionPositions(pm, preps)
		fits = make([]arcFit, len(pm.Arcs))
		tick := o.counter("edges", len(pm.Arcs))
		parallel(len(pm.Arcs), func(i int) {
			a := pm.Arcs[i]
			if a.Closed {
				fits[i].segs = fitLoop(a.Pts, o)
			} else {
				fits[i].segs = preps[i].fit(jpos[a.Pts[0]], jpos[a.Pts[len(a.Pts)-1]], o)
			}
			tick()
		})
	}

	// Fit every contour in parallel, indexed by layer and subpath.
	subpaths := make([][]svg.Subpath, len(layers))
	var jobs [][2]int
	for i, l := range layers {
		subpaths[i] = make([]svg.Subpath, len(l.contours))
		for k := range l.contours {
			jobs = append(jobs, [2]int{i, k})
		}
	}
	// Longest contours first: a big loop started last would run alone at
	// the end. Results are indexed by job, so the order never shows.
	sort.SliceStable(jobs, func(a, b int) bool {
		return len(layers[jobs[a][0]].contours[jobs[a][1]].Pts) > len(layers[jobs[b][0]].contours[jobs[b][1]].Pts)
	})
	refining := o.Refine.Enabled && o.Mode == ModeSpline
	img := refine.Window{X0: 0, Y0: 0, W: s.Width, H: s.Height}
	if pm != nil && refining {
		tick := o.counter("refine", sharedRounds)
		for round := 0; round < sharedRounds; round++ {
			s.refineShared(pm, fits, layers, img, o)
			tick()
		}
	}
	tick := o.counter("curves", len(jobs))
	parallel(len(jobs), func(j int) {
		i, k := jobs[j][0], jobs[j][1]
		c := layers[i].contours[k]
		var segs []fit.Segment
		if pm != nil {
			segs = assembleSegs(pm.Loops[layers[i].loops[k]], fits)
		} else {
			segs = segments(c.Pts, o)
		}
		if refining && pm == nil {
			var band func(p int) bool
			if layers[i].band != nil {
				band = layers[i].inBand
			}
			m := s.mask(uint8(layers[i].colour), c.Pts, c.Area2 < 0, band)
			segs = refineSegments(segs, m, img, o)
		}
		subpaths[i][k] = svg.Subpath{Segs: segs}
		tick()
	})

	// Shapes of different colours meet along fitted edges that drift
	// apart by up to the tolerance (cutout neighbours everywhere, stacked
	// siblings at a sharp junction), and the background shows through
	// the hairline; stroking each path in its own colour closes it. Pixel
	// mode is exact and the bw preset has no abutting colours.
	stroke := o.Preset != PresetBW && o.Mode != ModePixel && o.Stroke > 0
	mergeable := false
	gradientIDs := map[int]int{}     // palette index → gradient id
	overlayIDs := map[int][]string{} // palette index → its overlays' fills
	for i, l := range layers {
		fill := colour.FromSRGB8(s.Palette[l.colour].R, s.Palette[l.colour].G, s.Palette[l.colour].B).Hex()
		if s.hasRamp(l.colour) {
			id, ok := gradientIDs[l.colour]
			if !ok {
				id = len(res.Gradients) + len(res.Radials)
				gradientIDs[l.colour] = id
				r := s.Ramps[l.colour]
				if r.Radial {
					res.Radials = append(res.Radials, radialGradient(fmt.Sprintf("g%d", id), r))
				} else {
					res.Gradients = append(res.Gradients, linearGradient(fmt.Sprintf("g%d", id), r))
				}
				for _, ov := range r.Overlays {
					oid := len(res.Gradients) + len(res.Radials)
					res.Radials = append(res.Radials, svg.RadialGradient{ID: fmt.Sprintf("g%d", oid), CX: ov.CX, CY: ov.CY, R: ov.R, Stops: svgStops(ov.Stops)})
					overlayIDs[l.colour] = append(overlayIDs[l.colour], fmt.Sprintf("url(#g%d)", oid))
				}
			}
			fill = fmt.Sprintf("url(#g%d)", id)
		}
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
		path := svg.Path{Fill: fill, Subpaths: subpaths[i]}
		if stroke {
			path.Stroke, path.StrokeWidth = fill, o.Stroke
		}
		res.Paths = append(res.Paths, path)
		mergeable = stacked && outersOnly
		// A layered region's overlays are the same shape again, each
		// filled with its fading gradient over what is painted so far;
		// the stroke stays on the base, so the overlays composite as
		// the model did. Nothing may share the base path afterwards:
		// a later shape added to it would slip under the overlays.
		for _, of := range overlayIDs[l.colour] {
			res.Paths = append(res.Paths, svg.Path{Fill: of, Subpaths: subpaths[i]})
			mergeable = false
		}
	}
	return res, nil
}

// linearGradient is the SVG form of a linear ramp.
func linearGradient(id string, r *gradient.Ramp) svg.LinearGradient {
	return svg.LinearGradient{ID: id, X1: r.X0, Y1: r.Y0, X2: r.X1, Y2: r.Y1, Stops: svgStops(r.Stops)}
}

// radialGradient is the SVG form of a radial ramp.
func radialGradient(id string, r *gradient.Ramp) svg.RadialGradient {
	return svg.RadialGradient{ID: id, CX: r.CX, CY: r.CY, R: r.R, Stops: svgStops(r.Stops)}
}

func svgStops(in []gradient.Stop) []svg.Stop {
	var stops []svg.Stop
	for _, st := range in {
		stops = append(stops, svg.Stop{Offset: st.Offset, Colour: fmt.Sprintf("#%02x%02x%02x", st.Colour.R, st.Colour.G, st.Colour.B), Opacity: float64(st.Colour.A) / 255})
	}
	return stops
}

// assembleSegs concatenates the arc fits of loop l, reversed where the
// loop walks an arc backwards.
func assembleSegs(l planar.Loop, fits []arcFit) []fit.Segment {
	var out []fit.Segment
	for _, r := range l.Refs {
		segs := fits[r.Arc].segs
		if r.Reverse {
			segs = reverseSegs(segs)
		}
		out = append(out, segs...)
	}
	return out
}

// arcOwner returns, per arc, the loop that refines it: an emitted loop
// before an omitted one, then the lower colour, then loop order; -1 for
// an arc between two omitted loops, which nothing refines.
func arcOwner(pm *planar.Map, emitted []bool) []int {
	owner := make([]int, len(pm.Arcs))
	for i := range owner {
		owner[i] = -1
	}
	better := func(a, b int) bool {
		if emitted[a] != emitted[b] {
			return emitted[a]
		}
		if ca, cb := pm.Loops[a].Colour, pm.Loops[b].Colour; ca != cb {
			return ca < cb
		}
		return a < b
	}
	for li, l := range pm.Loops {
		for _, r := range l.Refs {
			if cur := owner[r.Arc]; cur < 0 || better(li, cur) {
				owner[r.Arc] = li
			}
		}
	}
	for i, li := range owner {
		if li >= 0 && !emitted[li] {
			owner[i] = -1
		}
	}
	return owner
}

// refineShared runs one round of refinement over the emitted loops in
// parallel: each loop's bezigon is assembled from its arcs with every
// junction fixed and the arcs other loops own frozen, is refined against
// its own colour's coverage mask, and its owned arcs are written back
// into fits once every loop is done, so the order of work never shows.
func (s *Segmentation) refineShared(pm *planar.Map, fits []arcFit, layers []layer, img refine.Window, o Options) {
	emitted := make([]bool, len(pm.Loops))
	for _, l := range layers {
		for _, li := range l.loops {
			emitted[li] = true
		}
	}
	owner := arcOwner(pm, emitted)
	type job struct{ layer, k int }
	var jobs []job
	for i, l := range layers {
		for k := range l.loops {
			jobs = append(jobs, job{i, k})
		}
	}
	type result struct {
		segs   []fit.Segment
		bounds []int // index of the first segment of each ref, then len(segs)
	}
	results := make([]result, len(jobs))
	parallel(len(jobs), func(j int) {
		l := layers[jobs[j].layer]
		li := l.loops[jobs[j].k]
		var segs []fit.Segment
		var fixed, frozen []bool
		owned := false
		for _, r := range pm.Loops[li].Refs {
			a := fits[r.Arc].segs
			if r.Reverse {
				a = reverseSegs(a)
			}
			for i := range a {
				fixed = append(fixed, i == 0 && !pm.Arcs[r.Arc].Closed)
				frozen = append(frozen, owner[r.Arc] != li)
			}
			segs = append(segs, a...)
			owned = owned || owner[r.Arc] == li
		}
		if !owned || len(segs) < 2 {
			return
		}
		c := l.contours[jobs[j].k]
		m := s.mask(uint8(l.colour), c.Pts, c.Area2 < 0, nil)
		out, f := refineConstrained(segs, fixed, frozen, m, img, o)
		var bounds []int
		for i, b := range f {
			if b {
				bounds = append(bounds, i)
			}
		}
		if len(bounds) == 0 {
			bounds = []int{0} // a closed island loop: one arc
		}
		results[j] = result{segs: out, bounds: append(bounds, len(out))}
	})
	for j, res := range results {
		if res.segs == nil {
			continue
		}
		li := layers[jobs[j].layer].loops[jobs[j].k]
		refs := pm.Loops[li].Refs
		if len(res.bounds)-1 != len(refs) {
			continue // a junction joint went missing: this loop keeps its plain arcs
		}
		for k, r := range refs {
			if owner[r.Arc] != li {
				continue
			}
			out := append([]fit.Segment(nil), res.segs[res.bounds[k]:res.bounds[k+1]]...)
			if r.Reverse {
				out = reverseSegs(out)
			}
			fits[r.Arc].segs = out
		}
	}
}

// buildLayers traces the colours, nests the shapes, drops the white page,
// extends stacked siblings and returns the layers to emit in output order.
func (s *Segmentation) buildLayers(o Options) []layer {
	layers, _ := s.buildLayersMap(o)
	return layers
}

// buildLayersMap is buildLayers that also returns the planar map the
// layers were traced through, nil when usePlanar(o) is false.
func (s *Segmentation) buildLayersMap(o Options) ([]layer, *planar.Map) {
	var layers []layer
	var pm *planar.Map
	if usePlanar(o) {
		o.report("outlines")
		layers, pm = s.planarLayers()
	} else {
		layers = s.layers(o)
	}
	o.report("nest")
	stacked := o.Hierarchical == HierarchicalStacked && o.Preset != PresetBW
	shapes := make([]contour.Shape, len(layers))
	for i, l := range layers {
		shapes[i] = l.shape
	}
	parents, ids := contour.NestRaster(shapes, s.Width, s.Height)
	for i := range layers {
		layers[i].depth = depthOf(parents, i)
		layers[i].contours = []contour.Contour{layers[i].shape.Outer}
		if !stacked {
			layers[i].contours = append(layers[i].contours, layers[i].shape.Holes...)
		}
	}
	// Drop the white page: depth-0 shapes sit straight on the background,
	// which renders white, so leaving out the white ones changes nothing
	// and keeps a transparent input transparent. The page is the near-white
	// colour holding at least half the depth-0 area, or one that was
	// mostly transparent in the input however little it holds; white that
	// is an opaque minority at depth 0 is content on a darker page.
	area0 := make([]int64, len(s.Palette))
	var total0 int64
	for _, l := range layers {
		if l.depth == 0 {
			area0[l.colour] += int64(l.shape.Outer.Area2)
			total0 += int64(l.shape.Outer.Area2)
		}
	}
	omit := make([]bool, len(layers))
	for i, l := range layers {
		omit[i] = l.depth == 0 && s.nearWhite(l.colour) && (2*area0[l.colour] >= total0 || s.mostlyClear(l.colour)) && !s.hasRamp(l.colour)
		layers[i].omit = omit[i]
	}
	if stacked {
		s.extendSiblings(layers, parents, ids, omit, o)
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
	return kept, pm
}

// planarLayers traces every colour through the shared-boundary map and
// returns the shapes as layers in colour then scan order, each contour
// tagged with its loop, together with the map.
func (s *Segmentation) planarLayers() ([]layer, *planar.Map) {
	counts := make([]int, len(s.Palette))
	for _, l := range s.Labels {
		counts[l]++
	}
	colours := make([]uint8, len(s.Palette))
	for c := range colours {
		colours[c] = uint8(c)
	}
	pm := planar.Build(s.Labels, s.Width, s.Height, colours, func(c uint8) func(x, y int) bool {
		return s.saddle(c, counts)
	})
	loop := func(i int) contour.Contour {
		return contour.Contour{Pts: pm.Assemble(pm.Loops[i]), Area2: pm.Loops[i].Area2}
	}
	var out []layer
	for _, sh := range pm.Shapes() {
		l := layer{colour: int(pm.Loops[sh[0]].Colour), loops: sh}
		l.shape.Outer = loop(sh[0])
		for _, h := range sh[1:] {
			l.shape.Holes = append(l.shape.Holes, loop(h))
		}
		out = append(out, l)
	}
	return out, &pm
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
	tick := o.counter("outlines", len(colours))
	parallel(len(colours), func(i int) {
		defer tick()
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
func (s *Segmentation) extendSiblings(layers []layer, parents []int, ids []int32, omit []bool, o Options) {
	w, h := s.Width, s.Height
	n := len(layers)
	// Growth is in 4-neighbour steps (diagonal steps would bridge shapes
	// that only touch at a corner), so along a 45° edge the margin covers
	// 1/√2 of its length: it must still clear twice the tolerance.
	margin := int(math.Ceil(2*math.Sqrt2*math.Max(o.FitTolerance, o.SimplifyTolerance))) + 1

	// ids holds the innermost shape per pixel (contour.NestRaster).
	order := make([]int, n)
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
	stamp := make([]int32, w*h)
	var frontier, next, region []int32
	for k := range layers {
		if omit[k] {
			continue
		}
		// Grow from k's pixels, margin steps, into pixels whose sibling at
		// k's depth is painted after k.
		mark := int32(k + 1)
		region = region[:0]
		contour.FillOuter(layers[k].shape.Outer.Pts, w, h, func(x, y int) {
			p := int32(y*w + x)
			stamp[p] = mark
			region = append(region, p)
		})
		base := len(region) // the shape's own pixels; what follows is the band
		frontier = append(frontier[:0], region...)
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
		band := append([]int32(nil), region[base:]...)
		sort.Slice(band, func(a, b int) bool { return band[a] < band[b] })
		layers[k].band = band
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
func parallel(n int, fn func(i int)) { par.Do(n, fn) }
