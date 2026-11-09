package rekha

import (
	"fmt"
	"image"
	"io"
	"math"
	"sort"

	"github.com/vexyl-ai/rekha/internal/fit"
	"github.com/vexyl-ai/rekha/internal/geom"
	"github.com/vexyl-ai/rekha/internal/svg"
)

// Result is a traced image: filled paths in pixel coordinates.
type Result struct {
	Width, Height int
	Paths         []svg.Path
}

// TraceReader decodes an image (PNG, JPEG or GIF) and traces it.
func TraceReader(r io.Reader, o Options) (*Result, error) {
	img, _, err := image.Decode(r)
	if err != nil {
		return nil, fmt.Errorf("rekha: decode: %w", err)
	}
	return Trace(img, o)
}

// Trace vectorises img according to o.
func Trace(img image.Image, o Options) (*Result, error) {
	s, err := Segment(img, o)
	if err != nil {
		return nil, err
	}
	return s.Finish(o)
}

// WriteSVG writes r as an SVG document with o.PathPrecision decimals.
func (r *Result) WriteSVG(w io.Writer, o Options) error {
	return svg.Write(w, r.Width, r.Height, r.Paths, o.PathPrecision)
}

// segments converts one closed crack loop into path segments for o.Mode.
func segments(pts []geom.Point, o Options) []fit.Segment {
	switch o.Mode {
	case ModePixel:
		return lines(toVec(pts))
	case ModePolygon:
		return lines(geom.Simplify(pts, o.SimplifyTolerance))
	default:
		return spline(pts, o)
	}
}

func toVec(pts []geom.Point) []geom.Vec2 {
	v := make([]geom.Vec2, len(pts))
	for i, p := range pts {
		v[i] = p.Vec()
	}
	return v
}

// lines joins consecutive points of a closed polygon with straight segments.
func lines(poly []geom.Vec2) []fit.Segment {
	n := len(poly)
	segs := make([]fit.Segment, 0, n)
	for i := 0; i < n; i++ {
		a, b := poly[i], poly[(i+1)%n]
		segs = append(segs, fit.Segment{Line: true, P: fit.Cubic{a, a, b, b}})
	}
	return segs
}

// spline detects corners on the simplified polygon and fits cubics to the
// dense boundary points between them.
// quantisation is how far, in pixels, an integer pixel corner can sit from
// the true edge it samples.
const quantisation = 0.5

// Corner placement constants, in dense (1 px) points. The flank window
// grows with the run on that side (a quarter of it, within
// [cornerWindow, cornerMaxWindow]); the skipped stretch and the allowed
// move scale with the window, because thresholding rounds a tip over a
// distance proportional to the image scale.
const (
	cornerCluster   = 3   // corners this close along the boundary are one vertex
	cornerWindow    = 12  // least points per side used to fit the flank lines
	cornerMaxWindow = 256 // most points per side
	cornerSkip      = 1   // least points nearest the corner ignored
	cornerMaxMove   = 3.0 // least distance a vertex may move from the crack corner
	// flankBend is the turn, in degrees, at a simplified-polygon vertex
	// beyond which a corner's flank window stops: past such a bend the
	// points belong to another edge and would skew the flank line. Noise
	// on a thresholded edge turns the polygon by far less.
	flankBend = 40.0

	// Straight-edge detection. A staircase-free edge of length L has
	// sagitta at most maxStairDev, so if it is a chord of a curve that
	// curve's radius is at least L²/(8·maxStairDev) ≈ L²/6. The direction
	// of its end half (tangent L/4 inside the end) and of the L/2 points
	// beyond the end (tangent L/4 outside) then differ by at most
	// (L/2)/R = 3/L radians. A genuine straight edge meeting an arc turns
	// far more. straightTurn is the safety factor on that bound; both
	// directions are least-squares fits, whose noise is well below it.
	minStraight  = 12.0
	straightTurn = 3.0
)

// vertex is a refined corner: a sub-pixel position where the least-squares
// lines through the two flanks meet, the dense indices that bound the
// runs on either side, and the flank directions used as run tangents.
type vertex struct {
	pos       geom.Vec2
	in, out   int       // dense indices: run arrives at in, leaves from out
	tIn, tOut geom.Vec2 // unit directions along the incoming and outgoing flanks
	span      int       // flank window used, in dense points
}

func spline(pts []geom.Point, o Options) []fit.Segment {
	// The dense points are pixel corners, up to half a pixel off the true
	// edge; without that allowance the max-error test sits on the noise
	// floor and splits curves into many short cubics.
	fitTol := o.FitTolerance + quantisation
	// Fit the raw crack vertices, spaced 1 px apart. Merging collinear
	// points first would leave a long straight edge as two points, so the
	// least-squares fit and its error check would see nothing along it.
	if len(geom.MergeCollinear(pts)) < 3 {
		return lines(toVec(pts))
	}
	dense := toVec(pts)
	// Simplification is tracked by index into the dense loop, never by
	// coordinate: a loop may visit the same lattice point twice.
	dssIdx := geom.RemoveStaircasesIndices(geom.MergeCollinear(pts))
	mergeIdx := geom.MergeCollinearIndices(pts)
	for k, i := range dssIdx {
		dssIdx[k] = mergeIdx[i]
	}
	dss := make([]geom.Vec2, len(dssIdx))
	for k, i := range dssIdx {
		dss[k] = dense[i]
	}
	simpIdx := geom.RDPClosedIndices(dss, o.SimplifyTolerance)
	simp := make([]geom.Vec2, len(simpIdx))
	for k, i := range simpIdx {
		simp[k] = dss[i]
	}

	// Corners are found on the simplified polygon and mapped to the dense
	// loop, then clustered and refined to sub-pixel positions.
	var corners []int
	for _, ci := range geom.Corners(simp, o.CornerThreshold) {
		corners = append(corners, dssIdx[simpIdx[ci]])
	}
	var bends []int
	for _, bi := range geom.Corners(simp, flankBend) {
		bends = append(bends, dssIdx[simpIdx[bi]])
	}
	verts := refineCorners(dense, clusterCorners(corners, len(dense)), bends)
	verts = append(verts, straightEdgeVertices(dense, dssIdx, verts)...)
	if len(verts) == 0 {
		return fit.FitClosed(dense, fitTol)
	}
	sort.Slice(verts, func(a, b int) bool { return verts[a].in < verts[b].in })
	n := len(dense)
	var segs []fit.Segment
	for k, v := range verts {
		next := verts[(k+1)%len(verts)]
		if len(verts) > 1 && next.in == v.out {
			// Two vertices at one dense index: nothing lies between them.
			if next.pos.Sub(v.pos).Len() > 1e-9 {
				segs = append(segs, fit.Segment{Line: true, P: fit.Cubic{v.pos, v.pos, next.pos, next.pos}})
			}
			continue
		}
		run := []geom.Vec2{v.pos}
		for i := (v.out + 1) % n; i != next.in; i = (i + 1) % n {
			run = append(run, dense[i])
		}
		if next.in != v.out {
			run = append(run, dense[next.in])
		}
		run = append(run, next.pos)
		if len(run) < 4 {
			segs = append(segs, fit.FitRun(run, fitTol)...)
			continue
		}
		segs = append(segs, fit.FitOpen(run, v.tOut, next.tIn.Scale(-1), fitTol)...)
	}
	return segs
}

// clusterCorners groups sorted dense corner indices that lie within
// cornerCluster of each other along the loop, wrapping at the end. Indices
// in a group that wraps are kept ascending by adding n to the early ones.
func clusterCorners(corners []int, n int) [][]int {
	var groups [][]int
	for _, c := range corners {
		if g := len(groups) - 1; g >= 0 && c-groups[g][len(groups[g])-1] <= cornerCluster {
			groups[g] = append(groups[g], c)
			continue
		}
		groups = append(groups, []int{c})
	}
	if len(groups) > 1 {
		first, last := groups[0], groups[len(groups)-1]
		if first[0]+n-last[len(last)-1] <= cornerCluster {
			for _, c := range first {
				last = append(last, c+n)
			}
			groups[0] = last
			groups = groups[:len(groups)-1]
		}
	}
	return groups
}

// refineCorners places one vertex per corner group where the least-squares
// lines through the flanks on either side intersect, which recovers sharp
// tips that thresholding blunted. bends holds the dense indices, ascending,
// of the simplified polygon's vertices that turn by more than flankBend: a
// flank never reaches past the neighbouring bend, where the boundary turns
// (perhaps by less than the corner threshold) and the points belong to
// another edge.
// A group whose flanks are too short to fit, nearly parallel (the end of
// a thin line), or meet farther than cornerMaxMove away is not merged:
// each of its corners becomes a vertex at its crack position.
func refineCorners(dense []geom.Vec2, groups [][]int, bends []int) []vertex {
	n := len(dense)
	var verts []vertex
	for g, grp := range groups {
		in, out := grp[0]%n, grp[len(grp)-1]%n
		// Flank windows are a quarter of the run on each side, and never
		// more than halfway to the neighbouring group or past the
		// neighbouring bend.
		prev, next := groups[(g+len(groups)-1)%len(groups)], groups[(g+1)%len(groups)]
		back := min(flankWindow(gap(prev[len(prev)-1]%n, in, n)), polyGap(bends, in, -1, n))
		fwd := min(flankWindow(gap(out, next[0]%n, n)), polyGap(bends, out, +1, n))
		span := max(back, fwd)
		skipIn, skipOut := max(cornerSkip, back/8), max(cornerSkip, fwd/8)
		maxMove := math.Max(cornerMaxMove, float64(span)/4)

		if back-skipIn >= 3 && fwd-skipOut >= 3 {
			pIn, dIn := fit.FitLine(window(dense, in, -1, skipIn, back))
			pOut, dOut := fit.FitLine(window(dense, out, +1, skipOut, fwd))
			crack := dense[in].Add(dense[out]).Scale(0.5)
			if x, ok := fit.Intersect(pIn, dIn, pOut, dOut); ok && x.Sub(crack).Len() <= maxMove {
				verts = append(verts, vertex{pos: x, in: in, out: out, tIn: dIn, tOut: dOut, span: span})
				continue
			}
		}
		for k, c := range grp {
			c %= n
			b, f := 1, 1
			if k == 0 {
				b = max(back, 1)
			}
			if k == len(grp)-1 {
				f = max(fwd, 1)
			}
			verts = append(verts, vertex{pos: dense[c], in: c, out: c, tIn: flankDir(dense, c, -1, b), tOut: flankDir(dense, c, +1, f), span: span})
		}
	}
	return verts
}

// gap is the number of dense steps from index a forward to index b.
func gap(a, b, n int) int { return ((b-a)%n + n) % n }

// polyGap is the number of dense steps from index i to the nearest
// polygon vertex in direction step (±1), other than i itself; n when the
// polygon has no other vertex.
func polyGap(polyIdx []int, i, step, n int) int {
	m := len(polyIdx)
	if m < 2 {
		return n
	}
	k := sort.SearchInts(polyIdx, i)
	if step > 0 {
		if k < m && polyIdx[k] == i {
			k++
		}
		return gap(i, polyIdx[k%m], n)
	}
	k--
	return gap(polyIdx[(k%m+m)%m], i, n)
}

// flankWindow is the number of dense points used to fit a flank line when
// the run on that side is runLen points long.
func flankWindow(runLen int) int {
	return min(max(cornerWindow, runLen/4), cornerMaxWindow, runLen/2)
}

// window returns the dense points from skip+1 to count steps away from i in
// direction step, ordered away from i.
func window(dense []geom.Vec2, i, step, skip, count int) []geom.Vec2 {
	n := len(dense)
	out := make([]geom.Vec2, 0, count-skip)
	for d := skip + 1; d <= count; d++ {
		out = append(out, dense[((i+step*d)%n+n)%n])
	}
	return out
}

// flankDir is the unit direction along the boundary from i over count
// steps in direction step, oriented forward along the loop.
func flankDir(dense []geom.Vec2, i, step, count int) geom.Vec2 {
	n := len(dense)
	count = max(count, 1)
	far := dense[((i+step*count)%n+n)%n]
	d := far.Sub(dense[i]).Norm()
	if step < 0 {
		d = d.Scale(-1)
	}
	if d == (geom.Vec2{}) {
		d = dense[(i+1)%n].Sub(dense[i]).Norm()
	}
	return d
}

// straightEdgeVertices finds the long straight edges of the staircase-free
// polygon (given as dense indices dssIdx) and returns a smooth breakpoint
// at each end, so that straight edges are fitted as lines joined
// tangentially to the curves beside them. Ends that fall within an
// existing vertex's flank window are skipped. Where two straight edges
// share an end, the two breakpoints merge into one vertex at the
// intersection of the edges' lines.
func straightEdgeVertices(dense []geom.Vec2, dssIdx []int, have []vertex) []vertex {
	n := len(dense)
	// Ends inside a corner's flank window are already covered by that
	// corner's line fit.
	near := func(i int) bool {
		for _, v := range have {
			if gap(v.out, i, n) <= v.span || gap(i, v.in, n) <= v.span {
				return true
			}
		}
		return false
	}
	m := len(dssIdx)
	if m < 3 {
		return nil
	}
	type end struct {
		idx  int
		p, d geom.Vec2 // the edge's least-squares line
	}
	var ends []end
	for i := 0; i < m; i++ {
		ia, ib := dssIdx[i], dssIdx[(i+1)%m]
		a, b := dense[ia], dense[ib]
		l := b.Sub(a).Len()
		if l < minStraight {
			continue
		}
		run := make([]geom.Vec2, 0, gap(ia, ib, n)+1)
		for j := ia; ; j = (j + 1) % n {
			run = append(run, dense[j])
			if j == ib {
				break
			}
		}
		// Least-squares directions, not chords: the chord between two
		// crack vertices is skewed by up to 1/L radians.
		p, d := fit.FitLine(run)
		look := max(3, min(int(l/2), n/4))
		half := min(look, len(run))
		_, dEnd := fit.FitLine(run[len(run)-half:])
		_, dStart := fit.FitLine(run[:half])
		_, after := fit.FitLine(window(dense, ib, +1, 0, look))
		_, before := fit.FitLine(window(dense, ia, -1, 0, look))
		before = before.Scale(-1) // window runs backwards; orient forward
		turnStart, turnEnd := angle(before, dStart), angle(dEnd, after)
		thr := straightTurn * 3 / l
		if max(turnStart, turnEnd) < thr {
			continue // turns like a chord of a curve: fit it as a curve
		}
		// A breakpoint goes only at an end where the boundary turns; an
		// end the boundary continues straight through is left for the run
		// fit, which joins collinear pieces into one line.
		for _, e := range []struct {
			idx  int
			turn float64
		}{{ia, turnStart}, {ib, turnEnd}} {
			if e.turn < thr || near(e.idx) {
				continue
			}
			ends = append(ends, end{idx: e.idx, p: p, d: d})
		}
	}
	// Breakpoint positions are the crack ends projected onto the edge's
	// least-squares line; a shared end takes the lines' intersection.
	project := func(e end) geom.Vec2 { return e.p.Add(e.d.Scale(dense[e.idx].Sub(e.p).Dot(e.d))) }
	var out []vertex
	for k := 0; k < len(ends); k++ {
		e := ends[k]
		var next *end
		if k+1 < len(ends) && ends[k+1].idx == e.idx {
			next = &ends[k+1]
		} else if k == len(ends)-1 && len(out) > 0 && ends[0].idx == e.idx && len(ends) > 1 {
			// The loop's last edge ends where its first edge begins.
			out[0].tIn = e.d
			if x, ok := fit.Intersect(e.p, e.d, ends[0].p, ends[0].d); ok && x.Sub(dense[e.idx]).Len() <= cornerMaxMove {
				out[0].pos = x
			} else {
				out[0].pos = project(e).Add(project(ends[0])).Scale(0.5)
			}
			continue
		}
		v := vertex{pos: project(e), in: e.idx, out: e.idx, tIn: e.d, tOut: e.d, span: cornerWindow}
		if next != nil {
			v.tOut = next.d
			if x, ok := fit.Intersect(e.p, e.d, next.p, next.d); ok && x.Sub(dense[e.idx]).Len() <= cornerMaxMove {
				v.pos = x
			} else {
				v.pos = project(e).Add(project(*next)).Scale(0.5)
			}
			k++
		}
		out = append(out, v)
	}
	return out
}

// angle returns the unsigned angle in radians between unit vectors a and b.
func angle(a, b geom.Vec2) float64 {
	return math.Abs(math.Atan2(geom.Cross(a, b), a.Dot(b)))
}
