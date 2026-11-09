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

func spline(pts []geom.Point, o Options) []fit.Segment {
	// The dense points are pixel corners, up to half a pixel off the true
	// edge; without that allowance the max-error test sits on the noise
	// floor and splits curves into many short cubics.
	fitTol := o.FitTolerance + quantisation
	dense := geom.MergeCollinear(pts)
	if len(dense) < 3 {
		return lines(toVec(pts))
	}
	simp := geom.Simplify(pts, o.SimplifyTolerance)
	denseV := toVec(dense)

	// Corners are found on the simplified polygon and mapped to the dense
	// loop. End tangents come from the simplified polygon's edges, which are
	// already straightened; the dense stair points are too noisy for that.
	type corner struct{ dense, simp int }
	var corners []corner
	seen := map[int]bool{}
	for _, ci := range geom.Corners(simp, o.CornerThreshold) {
		di := geom.NearestIndex(dense, simp[ci])
		if !seen[di] {
			seen[di] = true
			corners = append(corners, corner{di, ci})
		}
	}
	sort.Slice(corners, func(a, b int) bool { return corners[a].dense < corners[b].dense })

	if len(corners) == 0 {
		return fit.FitClosed(denseV, fitTol)
	}
	n, m := len(dense), len(simp)
	var segs []fit.Segment
	for k, c := range corners {
		next := corners[(k+1)%len(corners)]
		run := []geom.Vec2{denseV[c.dense]}
		for i := (c.dense + 1) % n; i != next.dense; i = (i + 1) % n {
			run = append(run, denseV[i])
		}
		run = append(run, denseV[next.dense])
		t0 := simp[(c.simp+1)%m].Sub(simp[c.simp]).Norm()
		t1 := simp[(next.simp+m-1)%m].Sub(simp[next.simp]).Norm()
		if len(run) < 4 || t0 == (geom.Vec2{}) || t1 == (geom.Vec2{}) {
			segs = append(segs, fit.FitRun(run, fitTol)...)
			continue
		}
		segs = append(segs, fit.FitOpen(run, t0, t1, fitTol)...)
	}
	return segs
}
