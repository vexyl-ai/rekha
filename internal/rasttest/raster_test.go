package rasttest

import (
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

func TestFillRingEvenOdd(t *testing.T) {
	b := Fill(5, 5, []svg.Path{{Subpaths: []svg.Subpath{rect(0, 0, 5, 5), rect(1, 1, 4, 4)}}})
	if b.Count() != 16 {
		t.Fatalf("Count = %d, want 16", b.Count())
	}
	if b.Get(2, 2) || !b.Get(0, 2) {
		t.Fatal("hole not respected")
	}
}
