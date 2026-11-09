package rekha

import (
	"fmt"
	"image"
	"io"
	"math"
	"sort"

	"github.com/vexyl-ai/rekha/internal/bitmap"
	"github.com/vexyl-ai/rekha/internal/contour"
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
	if err := o.Validate(); err != nil {
		return nil, err
	}
	if o.Preset != PresetBW {
		return nil, &ValidationError{"Preset", "not implemented until Phase 2"}
	}

	gray, w, h := bitmap.Gray(img)
	th := uint8(math.Round(o.Threshold * 255))
	if o.AutoThreshold {
		th = bitmap.Otsu(gray)
	}
	bm := bitmap.FromGray(gray, w, h, th)
	shapes := contour.BuildShapes(contour.Trace(bm, o.TurnPolicy), o.FilterSpeckle)

	res := &Result{Width: w, Height: h}
	for _, s := range shapes {
		p := svg.Path{Fill: "#000000"}
		p.Subpaths = append(p.Subpaths, svg.Subpath{Segs: segments(s.Outer.Pts, o)})
		for _, hole := range s.Holes {
			p.Subpaths = append(p.Subpaths, svg.Subpath{Segs: segments(hole.Pts, o)})
		}
		res.Paths = append(res.Paths, p)
	}
	return res, nil
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

// Corner placement constants, in dense (1 px) points.
const (
	cornerCluster = 3  // corners this close along the boundary are one vertex
	cornerWindow  = 12 // points per side used to fit the flank lines
	cornerSkip    = 1  // points nearest the corner ignored (thresholding rounds tips)
	cornerMaxMove = 3.0

	// Straight-edge detection. A staircase-free edge of length L has
	// sagitta at most maxStairDev, so if it is a chord of a curve that
	// curve's radius is at least L²/(8·maxStairDev) ≈ L²/6, and the
	// boundary direction L/4 beyond its end (the centre of a window of L/2
	// points) differs from the edge by at most (L/4)/R = 1.5/L radians. A
	// genuine straight edge meeting an arc turns far more. straightTurn is
	// the safety factor on that bound; the window direction is a
	// least-squares fit, whose noise is well below it.
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
	simp := geom.Simplify(pts, o.SimplifyTolerance)

	// Corners are found on the simplified polygon and mapped to the dense
	// loop, then clustered and refined to sub-pixel positions.
	var corners []int
	seen := map[int]bool{}
	for _, ci := range geom.Corners(simp, o.CornerThreshold) {
		di := geom.NearestIndex(pts, simp[ci])
		if !seen[di] {
			seen[di] = true
			corners = append(corners, di)
		}
	}
	sort.Ints(corners)
	verts := refineCorners(dense, clusterCorners(corners, len(dense)))
	verts = append(verts, straightEdgeVertices(pts, dense, verts)...)
	if len(verts) == 0 {
		return fit.FitClosed(dense, fitTol)
	}
	sort.Slice(verts, func(a, b int) bool { return verts[a].in < verts[b].in })
	n := len(dense)
	var segs []fit.Segment
	for k, v := range verts {
		next := verts[(k+1)%len(verts)]
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
// tips that thresholding blunted. A group whose flanks are too short to
// fit, nearly parallel (the end of a thin line), or meet farther than
// cornerMaxMove away is not merged: each of its corners becomes a vertex
// at its crack position.
func refineCorners(dense []geom.Vec2, groups [][]int) []vertex {
	n := len(dense)
	var verts []vertex
	for g, grp := range groups {
		in, out := grp[0]%n, grp[len(grp)-1]%n
		// Flank windows stop halfway to the neighbouring groups.
		prev, next := groups[(g+len(groups)-1)%len(groups)], groups[(g+1)%len(groups)]
		back := min(cornerWindow, gap(prev[len(prev)-1]%n, in, n)/2)
		fwd := min(cornerWindow, gap(out, next[0]%n, n)/2)

		if back-cornerSkip >= 3 && fwd-cornerSkip >= 3 {
			pIn, dIn := fit.FitLine(window(dense, in, -1, cornerSkip, back))
			pOut, dOut := fit.FitLine(window(dense, out, +1, cornerSkip, fwd))
			crack := dense[in].Add(dense[out]).Scale(0.5)
			if x, ok := fit.Intersect(pIn, dIn, pOut, dOut); ok && x.Sub(crack).Len() <= cornerMaxMove {
				verts = append(verts, vertex{pos: x, in: in, out: out, tIn: dIn, tOut: dOut})
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
			verts = append(verts, vertex{pos: dense[c], in: c, out: c, tIn: flankDir(dense, c, -1, b), tOut: flankDir(dense, c, +1, f)})
		}
	}
	return verts
}

// gap is the number of dense steps from index a forward to index b.
func gap(a, b, n int) int { return ((b-a)%n + n) % n }

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
// polygon and returns a smooth breakpoint at each end, so that straight
// edges are fitted as lines joined tangentially to the curves beside them.
// Ends that fall within cornerCluster of an existing vertex are skipped.
func straightEdgeVertices(pts []geom.Point, dense []geom.Vec2, have []vertex) []vertex {
	n := len(dense)
	index := make(map[geom.Point]int, n)
	for i, p := range pts {
		index[p] = i
	}
	// Ends inside a corner's flank window are already covered by that
	// corner's line fit.
	near := func(i int) bool {
		for _, v := range have {
			if gap(v.out, i, n) <= cornerWindow || gap(i, v.in, n) <= cornerWindow {
				return true
			}
		}
		return false
	}
	dss := geom.RemoveStaircases(geom.MergeCollinear(pts))
	m := len(dss)
	if m < 3 {
		return nil
	}
	var out []vertex
	for i := 0; i < m; i++ {
		a, b := dss[i].Vec(), dss[(i+1)%m].Vec()
		l := b.Sub(a).Len()
		if l < minStraight {
			continue
		}
		ia, ib := index[dss[i]], index[dss[(i+1)%m]]
		run := make([]geom.Vec2, 0, gap(ia, ib, n)+1)
		for j := ia; ; j = (j + 1) % n {
			run = append(run, dense[j])
			if j == ib {
				break
			}
		}
		// The edge's own least-squares direction, not its chord: the chord
		// between two crack vertices is skewed by up to 1/L radians.
		p, d := fit.FitLine(run)
		look := max(3, min(int(l/2), n/4))
		_, after := fit.FitLine(window(dense, ib, +1, 0, look))
		_, before := fit.FitLine(window(dense, ia, -1, 0, look))
		before = before.Scale(-1) // window runs backwards; orient forward
		if max(angle(d, after), angle(before, d)) < straightTurn*1.5/l {
			continue // turns like a chord of a curve: fit it as a curve
		}
		// Sub-pixel position: project each crack end onto that line.
		project := func(q geom.Vec2) geom.Vec2 { return p.Add(d.Scale(q.Sub(p).Dot(d))) }
		for _, e := range []struct {
			idx int
			pos geom.Vec2
		}{{ia, project(dense[ia])}, {ib, project(dense[ib])}} {
			if near(e.idx) {
				continue
			}
			out = append(out, vertex{pos: e.pos, in: e.idx, out: e.idx, tIn: d, tOut: d})
		}
	}
	return out
}

// angle returns the unsigned angle in radians between unit vectors a and b.
func angle(a, b geom.Vec2) float64 {
	return math.Abs(math.Atan2(a.X*b.Y-a.Y*b.X, a.Dot(b)))
}
