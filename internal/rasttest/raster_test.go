package rasttest

import (
	"reflect"
	"testing"

	"github.com/vexyl-ai/rekha/internal/fit"
	"github.com/vexyl-ai/rekha/internal/geom"
	"github.com/vexyl-ai/rekha/internal/svg"
)

func ln(a, b geom.Vec2) fit.Segment { return fit.Segment{Line: true, P: fit.Cubic{a, a, b, b}} }

func rect(x0, y0, x1, y1 float64) svg.Subpath {
	return svg.Subpath{Segs: []fit.Segment{
		ln(geom.Vec2{X: x0, Y: y0}, geom.Vec2{X: x1, Y: y0}),
		ln(geom.Vec2{X: x1, Y: y0}, geom.Vec2{X: x1, Y: y1}),
		ln(geom.Vec2{X: x1, Y: y1}, geom.Vec2{X: x0, Y: y1}),
		ln(geom.Vec2{X: x0, Y: y1}, geom.Vec2{X: x0, Y: y0}),
	}}
}

func TestFillSquare(t *testing.T) {
	b := Fill(4, 4, []svg.Path{{Subpaths: []svg.Subpath{rect(1, 1, 3, 3)}}})
	if b.Count() != 4 {
		t.Fatalf("Count = %d, want 4", b.Count())
	}
	if !b.Get(1, 1) || !b.Get(2, 2) || b.Get(0, 0) || b.Get(3, 3) {
		t.Fatal("wrong pixels")
	}
}

// reverse returns the subpath walked the other way round, as a hole is.
func reverse(sp svg.Subpath) svg.Subpath {
	out := svg.Subpath{}
	for i := len(sp.Segs) - 1; i >= 0; i-- {
		s := sp.Segs[i]
		out.Segs = append(out.Segs, ln(s.P[3], s.P[0]))
	}
	return out
}

// A hole runs opposite to its outer, so nonzero winding leaves it empty.
func TestFillRingNonzero(t *testing.T) {
	b := Fill(5, 5, []svg.Path{{Subpaths: []svg.Subpath{rect(0, 0, 5, 5), reverse(rect(1, 1, 4, 4))}}})
	if b.Count() != 16 {
		t.Fatalf("Count = %d, want 16", b.Count())
	}
	if b.Get(2, 2) || !b.Get(0, 2) {
		t.Fatal("hole not respected")
	}
}

// Paint fills each path with its own colour index in order, so a later
// path covers an earlier one; untouched pixels stay 255.
func TestPaintOrderAndHoles(t *testing.T) {
	sq := func(x0, y0, x1, y1 int) svg.Subpath {
		p := func(x, y int) geom.Vec2 { return geom.Vec2{X: float64(x), Y: float64(y)} }
		line := func(a, b geom.Vec2) fit.Segment { return fit.Segment{Line: true, P: fit.Cubic{a, a, b, b}} }
		return svg.Subpath{Segs: []fit.Segment{
			line(p(x0, y0), p(x1, y0)), line(p(x1, y0), p(x1, y1)), line(p(x1, y1), p(x0, y1)), line(p(x0, y1), p(x0, y0)),
		}}
	}
	paths := []svg.Path{
		{Fill: "#ff0000", Subpaths: []svg.Subpath{sq(0, 0, 4, 4), reverse(sq(1, 1, 3, 3))}}, // red with a hole
		{Fill: "#0000ff", Subpaths: []svg.Subpath{sq(3, 3, 6, 6)}},                          // blue on top
	}
	got := Paint(6, 6, paths, map[string]uint8{"#ff0000": 1, "#0000ff": 2})
	want := []uint8{
		1, 1, 1, 1, 255, 255,
		1, 255, 255, 1, 255, 255,
		1, 255, 255, 1, 255, 255,
		1, 1, 1, 2, 2, 2,
		255, 255, 255, 2, 2, 2,
		255, 255, 255, 2, 2, 2,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got\n%v\nwant\n%v", got, want)
	}
}
