package rekha

import (
	"fmt"
	"image"
	"io"
	"math"
	"sort"

	"github.com/vexyl-ai/rekha/internal/fit"
	"github.com/vexyl-ai/rekha/internal/geom"
	"github.com/vexyl-ai/rekha/internal/refine"
	"github.com/vexyl-ai/rekha/internal/svg"
)

// Result is a traced image: filled paths in pixel coordinates, and the
// gradients (Options.Gradients) that some of them are filled with.
type Result struct {
	Width, Height int
	Gradients     []svg.LinearGradient
	Radials       []svg.RadialGradient
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

// Trace vectorises img according to o, or, when o.Auto is set, under
// the candidate Auto picks.
func Trace(img image.Image, o Options) (*Result, error) {
	if o.Auto {
		r, _, err := Auto(img, o)
		return r, err
	}
	s, err := Segment(img, o)
	if err != nil {
		return nil, err
	}
	return s.Finish(o)
}

// WriteSVG writes r as an SVG document with o.PathPrecision decimals,
// marked with Rekha's name and version unless o.NoMark is set.
func (r *Result) WriteSVG(w io.Writer, o Options) error {
	return svg.WriteDocument(w, r.document(o), o.PathPrecision)
}

// segments converts one closed crack loop into path segments for o.Mode.
func segments(pts []geom.Point, o Options) []fit.Segment {
	switch o.Mode {
	case ModePixel:
		return lines(toVec(pts))
	case ModePolygon:
		return lines(geom.Simplify(pts, o.SimplifyTolerance))
	default:
		return fitLoop(pts, o)
	}
}

// lineTol is how far, in pixels, a cubic's handles may sit from its chord
// for the refined segment to be written as a line: such a cubic is within
// 0.19 px of straight, below what the output precision can show, and the
// optimiser leaves straight edges bent by about a tenth of a pixel.
const lineTol = 0.25

// refineSegments runs the Phase 3 refinement on one fitted contour:
// optimise against the mask, merge and split segments, recognise a circle,
// and write straight cubics back as lines.
func refineSegments(segs []fit.Segment, m refine.Mask, img refine.Window, o Options) []fit.Segment {
	out, _ := refineConstrained(segs, nil, nil, m, img, o)
	return out
}

// refineConstrained is refineSegments with fixed anchors and frozen
// segments (see refine.Options): a loop assembled from shared arcs keeps
// its junctions in place and the arcs other loops own untouched. It
// returns the fixed flags renumbered for the result, so the caller can
// find the junctions again. Circle recognition needs the whole loop free.
func refineConstrained(segs []fit.Segment, fixed, frozen []bool, m refine.Mask, img refine.Window, o Options) ([]fit.Segment, []bool) {
	if len(segs) < 2 {
		return segs, fixed
	}
	ro := refine.Options{Sweeps: o.Refine.Sweeps, MaxIter: o.Refine.MaxIter, Weights: o.Refine.Weights, Fixed: fixed, Frozen: frozen}
	b := refine.FromSegments(segs)
	b = refine.Refine(b, m, img, ro)
	if o.Refine.MergeSplit {
		b, ro = refine.MergeSplitTracked(b, m, img, ro)
	}
	if o.Refine.Primitives {
		// Primitives judge what is emitted: a straight edge leaves the
		// refinement as a cubic bowing within lineTol of its chord.
		b = b.Straightened(lineTol)
		tol := 0.5 * o.FitTolerance
		free := !anyTrue(ro.Fixed) && !anyTrue(ro.Frozen)
		if free {
			b, _ = refine.SnapCircle(b, m, img, tol)
		}
		b, ro = refine.SnapPieces(b, m, img, tol, ro)
		b, ro = refine.CommonRadius(b, m, img, tol, ro)
		if free {
			b, _ = refine.FitRectangle(b, m, img, tol)
		}
	}
	return b.Segments(lineTol), ro.Fixed
}

func anyTrue(v []bool) bool {
	for _, x := range v {
		if x {
			return true
		}
	}
	return false
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
	// flankTol is how far, in pixels, a flank window's points may stray
	// from the line, or circle, they are read as (fitFlank): a
	// thresholded edge staircases within half a pixel.
	flankTol = 0.75
	// flankArcTol is the same allowance for a circle: a digital circle's
	// crack vertices stray up to half a diagonal from it.
	flankArcTol = 0.9
	// cornerMinCross is the sine of the smallest angle at which a curved
	// flank may cross the other for their crossing to be a corner: 10°.
	cornerMinCross = 0.17365

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

// fitLoop fits one closed crack loop as a spline: corners refined to
// sub-pixel vertices, straight edges as lines, the runs between as cubics.
func fitLoop(pts []geom.Point, o Options) []fit.Segment {
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
	// A corner of the simplified polygon must also turn on the staircase-
	// free polyline, whose vertices are dense: on a small circle the
	// simplified vertices sit far apart and turn sharply between
	// themselves while the boundary turns gently.
	var corners []int
	for _, ci := range geom.Corners(simp, o.CornerThreshold) {
		if geom.Turn(dss, simpIdx[ci]) > o.CornerThreshold {
			corners = append(corners, dssIdx[simpIdx[ci]])
		}
	}
	var bends []int
	for _, bi := range geom.Corners(simp, flankBend) {
		bends = append(bends, dssIdx[simpIdx[bi]])
	}
	verts := refineCorners(dense, clusterCorners(corners, len(dense)), bends)
	verts = append(verts, straightEdgeVertices(dense, dssIdx, verts, false)...)
	if len(verts) == 0 {
		return fit.FitClosed(dense, fitTol)
	}
	return fitRuns(dense, verts, fitTol, false)
}

// fitArc fits an open crack polyline whose ends are junctions shared with
// other arcs, ending exactly on its crack ends. The planar path prepares
// every arc first, places each junction from the flank lines of the arcs
// meeting there and fits with those ends (arcPrep.fit).
func fitArc(pts []geom.Point, o Options) []fit.Segment {
	n := len(pts)
	if n < 2 {
		return nil
	}
	return prepareArc(pts, o).fit(pts[0].Vec(), pts[n-1].Vec(), o)
}

// arcPrep is an open arc analysed for fitting: its dense points, the
// staircase-free polyline, the interior corner vertices, and at each end
// the flank line that gives the end tangent and places the junction.
type arcPrep struct {
	pts    []geom.Point
	dense  []geom.Vec2
	dssIdx []int
	verts  []vertex
	short  bool     // fewer than four points, or collinear: one line
	flank  [2]flank // at pts[0] and at pts[n-1]
}

// flank is the least-squares line through the straight stretch of dense
// points next to an arc end: a point on it, its unit direction oriented
// forward along the arc, and the steps it spans.
type flank struct {
	p, d geom.Vec2
	span int
}

// flankDev is how far the points of a flank window may lie from their
// least-squares line for the window to count as straight: the deviation
// of a digital line's crack vertices (geom's staircase rule).
const flankDev = 0.75

// prepareArc analyses an open arc of at least two points for fit.
func prepareArc(pts []geom.Point, o Options) arcPrep {
	n := len(pts)
	a := arcPrep{pts: pts, dense: toVec(pts)}
	mergeIdx := geom.MergeCollinearOpenIndices(pts)
	if n < 4 || len(mergeIdx) < 3 {
		a.short = true
		a.flank[0] = endFlank(a.dense, 0, +1, n-1)
		a.flank[1] = endFlank(a.dense, n-1, -1, n-1)
		return a
	}
	merged := make([]geom.Point, len(mergeIdx))
	for k, i := range mergeIdx {
		merged[k] = pts[i]
	}
	a.dssIdx = geom.RemoveStaircasesOpenIndices(merged)
	for k, i := range a.dssIdx {
		a.dssIdx[k] = mergeIdx[i]
	}
	dss := make([]geom.Vec2, len(a.dssIdx))
	for k, i := range a.dssIdx {
		dss[k] = a.dense[i]
	}
	simpIdx := geom.RDPIndices(dss, o.SimplifyTolerance)
	simp := make([]geom.Vec2, len(simpIdx))
	for k, i := range simpIdx {
		simp[k] = dss[i]
	}
	// Interior corners and bends as in fitLoop; the ends bound every flank
	// window as bends do, so no window crosses them.
	var corners []int
	bends := []int{0}
	for ci := 1; ci+1 < len(simp); ci++ {
		turn := geom.TurnOpen(simp, ci)
		if turn > o.CornerThreshold && geom.TurnOpen(dss, simpIdx[ci]) > o.CornerThreshold {
			corners = append(corners, a.dssIdx[simpIdx[ci]])
		}
		if turn > flankBend {
			bends = append(bends, a.dssIdx[simpIdx[ci]])
		}
	}
	bends = append(bends, n-1)
	a.verts = refineCorners(a.dense, clusterCorners(corners, n), bends)
	// The end flanks stop at the first vertex.
	first, last := n-1, 0
	for _, v := range a.verts {
		first, last = min(first, v.in), max(last, v.out)
	}
	a.flank[0] = endFlank(a.dense, 0, +1, min(flankWindow(n), first))
	a.flank[1] = endFlank(a.dense, n-1, -1, min(flankWindow(n), n-1-last))
	return a
}

// endFlank fits the line through dense[i] and up to count points beyond
// it in direction step, halving the window until the points lie within
// flankDev of the line (a curved flank gives a short window, a straight
// staircase the whole one). The direction is oriented forward along the
// arc.
func endFlank(dense []geom.Vec2, i, step, count int) flank {
	for count = max(count, 1); ; count /= 2 {
		w := append([]geom.Vec2{dense[i]}, window(dense, i, step, 0, count)...)
		p, d := fit.FitLine(w)
		straight := true
		for _, q := range w {
			if geom.PerpDist(q, p, p.Add(d)) > flankDev {
				straight = false
				break
			}
		}
		if straight || count <= 2 {
			if step < 0 {
				d = d.Scale(-1)
			}
			return flank{p: p, d: d, span: count}
		}
	}
}

// fit fits the prepared arc from p0 to p1, the placed positions of its
// two junctions: the end vertices sit exactly there with the flank lines
// as tangents, so both sides of the arc get the same ends.
func (a arcPrep) fit(p0, p1 geom.Vec2, o Options) []fit.Segment {
	n := len(a.pts)
	if n < 2 {
		return nil
	}
	if a.short {
		return []fit.Segment{{Line: true, P: fit.Cubic{p0, p0, p1, p1}}}
	}
	fitTol := o.FitTolerance + quantisation
	verts := append([]vertex(nil), a.verts...)
	verts = append(verts,
		vertex{pos: p0, span: a.flank[0].span, tOut: a.flank[0].d},
		vertex{pos: p1, in: n - 1, out: n - 1, span: a.flank[1].span, tIn: a.flank[1].d})
	verts = append(verts, straightEdgeVertices(a.dense, a.dssIdx, verts, true)...)
	return fitRuns(a.dense, verts, fitTol, true)
}

// reverseSegs returns the path segs walked backwards: the same numbers,
// each segment's control points in the opposite order.
func reverseSegs(segs []fit.Segment) []fit.Segment {
	out := make([]fit.Segment, len(segs))
	for i, s := range segs {
		out[len(segs)-1-i] = fit.Segment{Line: s.Line, P: fit.Cubic{s.P[3], s.P[2], s.P[1], s.P[0]}}
	}
	return out
}

// fitRuns fits the runs of dense between consecutive vertices, wrapping
// from the last vertex to the first unless open.
func fitRuns(dense []geom.Vec2, verts []vertex, fitTol float64, open bool) []fit.Segment {
	sort.Slice(verts, func(a, b int) bool { return verts[a].in < verts[b].in })
	n := len(dense)
	var segs []fit.Segment
	for k, v := range verts {
		if open && k == len(verts)-1 {
			break
		}
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
	// A placed end that lands on the first vertex leaves an empty run.
	kept := segs[:0]
	for _, sg := range segs {
		if !sg.Line || sg.P[0] != sg.P[3] {
			kept = append(kept, sg)
		}
	}
	return kept
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
			fIn := fitFlank(window(dense, in, -1, skipIn, back))
			fOut := fitFlank(window(dense, out, +1, skipOut, fwd))
			crack := dense[in].Add(dense[out]).Scale(0.5)
			if x, ok := cornerOf(fIn, fOut, crack); ok && x.Sub(crack).Len() <= maxMove {
				verts = append(verts, vertex{pos: x, in: in, out: out, tIn: fIn.tangentAt(x), tOut: fOut.tangentAt(x), span: span})
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

// cornerFlank is the primitive a corner's flank window lies on: its
// least-squares line (p, d, ordered away from the corner), and, when
// the window bows away from that line but lies on a circle, that circle
// (curved set). A flank that runs along a curve is read as the curve:
// the chord line of an arc misses the corner by its sagitta and more (a
// flat side meeting an arc of r = 80 was extended 8 px past the
// corner), and no shorter chord helps, since the window starts past the
// skipped tip where the tangent has already turned.
type cornerFlank struct {
	p, d   geom.Vec2
	centre geom.Vec2
	r      float64
	curved bool
}

// fitFlank fits the flank window: a line when every point lies within
// flankTol of the least-squares line; else a circle when the window has
// at least cornerWindow points (fewer give no usable circle fit) that
// all lie within flankArcTol of the geometric least-squares circle;
// else the line, as before. A window the line holds is read as the line
// even when a circle would hold it closer: on a nearly straight window
// the two fits place the corner within hundredths of a pixel of each
// other, and which fit wins would turn on the last digits (and differ
// between arm64 and amd64).
func fitFlank(pts []geom.Vec2) cornerFlank {
	p, d := fit.FitLine(pts)
	f := cornerFlank{p: p, d: d}
	worst := 0.0
	for _, q := range pts {
		worst = math.Max(worst, math.Abs(geom.Cross(d, q.Sub(p))))
	}
	if worst <= flankTol || len(pts) < cornerWindow {
		return f
	}
	centre, r, ok := fit.CircleGeometric(pts)
	if !ok {
		return f
	}
	for _, q := range pts {
		if math.Abs(q.Sub(centre).Len()-r) > flankArcTol {
			return f
		}
	}
	f.centre, f.r, f.curved = centre, r, true
	return f
}

// tangentAt is the flank's direction at x, oriented as its line is: the
// line's direction, or the circle's tangent there.
func (f cornerFlank) tangentAt(x geom.Vec2) geom.Vec2 {
	if !f.curved {
		return f.d
	}
	rad := x.Sub(f.centre)
	t := geom.Vec2{X: -rad.Y, Y: rad.X}.Norm()
	if t.Dot(f.d) < 0 {
		t = t.Scale(-1)
	}
	return t
}

// cornerOf is where two flanks meet: the intersection of their lines, of
// a line and a circle, or of two circles, the one nearest crack when
// there are two; ok is false when they do not meet, or when a curved
// pair crosses at under cornerMinCross: a line grazing a circle, or two
// circles all but tangent, cross at a point that moves by pixels with
// the last digits of the fits (and differs between arm64 and amd64),
// and the junction is a smooth one, not a corner.
func cornerOf(a, b cornerFlank, crack geom.Vec2) (geom.Vec2, bool) {
	var cands []geom.Vec2
	switch {
	case !a.curved && !b.curved:
		x, ok := fit.Intersect(a.p, a.d, b.p, b.d)
		return x, ok
	case a.curved && b.curved:
		cands = fit.CircleCircle(a.centre, a.r, b.centre, b.r)
	case a.curved:
		cands = fit.LineCircle(b.p, b.d, a.centre, a.r)
	default:
		cands = fit.LineCircle(a.p, a.d, b.centre, b.r)
	}
	best, bd := geom.Vec2{}, math.Inf(1)
	for _, c := range cands {
		if d := c.Sub(crack).Len(); d < bd {
			best, bd = c, d
		}
	}
	if len(cands) == 0 || math.Abs(geom.Cross(a.tangentAt(best), b.tangentAt(best))) < cornerMinCross {
		return geom.Vec2{}, false
	}
	return best, true
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
func straightEdgeVertices(dense []geom.Vec2, dssIdx []int, have []vertex, open bool) []vertex {
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
	edges := m
	if open {
		edges = m - 1 // no closing edge
	}
	for i := 0; i < edges; i++ {
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
		thr := straightTurn * 3 / l
		// An open polyline ends at a junction, where the boundary turns
		// by definition; the end vertex there is already a breakpoint.
		turnStart, turnEnd := math.Inf(1), math.Inf(1)
		if !open || ib+look <= n-1 {
			_, after := fit.FitLine(window(dense, ib, +1, 0, look))
			turnEnd = angle(dEnd, after)
		}
		if !open || ia-look >= 0 {
			_, before := fit.FitLine(window(dense, ia, -1, 0, look))
			before = before.Scale(-1) // window runs backwards; orient forward
			turnStart = angle(before, dStart)
		}
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
		} else if !open && k == len(ends)-1 && len(out) > 0 && ends[0].idx == e.idx && len(ends) > 1 {
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
