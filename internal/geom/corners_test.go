package geom

import (
	"math"
	"testing"
)

func TestCornersSquare(t *testing.T) {
	sq := []Vec2{{0, 0}, {10, 0}, {10, 10}, {0, 10}}
	got := Corners(sq, 60)
	if len(got) != 4 {
		t.Fatalf("got %v", got)
	}
	for i, c := range got {
		if c != i {
			t.Fatalf("got %v, want [0 1 2 3]", got)
		}
	}
}

func TestCornersOctagonNone(t *testing.T) {
	var oct []Vec2
	for i := 0; i < 8; i++ {
		a := float64(i) * math.Pi / 4
		oct = append(oct, Vec2{10 * math.Cos(a), 10 * math.Sin(a)})
	}
	if got := Corners(oct, 60); len(got) != 0 {
		t.Fatalf("got %v, want none", got)
	}
}

// Two squares touching at one corner trace as one loop that visits the
// shared lattice point twice. Index mapping must tell the visits apart:
// both survive simplification as distinct vertices.
func TestSimplifyIndicesKeepsRevisitedVertex(t *testing.T) {
	var loop []Point
	// Square A: (0,0)-(4,4); square B: (4,4)-(8,8), joined at (4,4).
	for x := 0; x < 4; x++ {
		loop = append(loop, Point{x, 0})
	}
	for y := 0; y < 4; y++ {
		loop = append(loop, Point{4, y})
	}
	for x := 4; x < 8; x++ {
		loop = append(loop, Point{x, 4})
	}
	for y := 4; y < 8; y++ {
		loop = append(loop, Point{8, y})
	}
	for x := 8; x > 4; x-- {
		loop = append(loop, Point{x, 8})
	}
	for y := 8; y > 4; y-- {
		loop = append(loop, Point{4, y})
	}
	for x := 4; x > 0; x-- {
		loop = append(loop, Point{x, 4})
	}
	for y := 4; y > 0; y-- {
		loop = append(loop, Point{0, y})
	}
	idx := SimplifyIndices(loop, 0.75)
	if len(idx) != 8 {
		t.Fatalf("got %d vertices %v, want 8", len(idx), idx)
	}
	for k := 1; k < len(idx); k++ {
		if idx[k] <= idx[k-1] {
			t.Fatalf("indices not ascending: %v", idx)
		}
	}
	visits := 0
	for _, i := range idx {
		if loop[i] == (Point{4, 4}) {
			visits++
		}
	}
	if visits != 2 {
		t.Fatalf("shared vertex kept %d times, want 2: %v", visits, idx)
	}
}

// A 1 px jog inside a gently curving run is a digitisation artefact, not a
// corner: the turn must be measured over a longer arm than one edge.
func TestCornersIgnoreShortJog(t *testing.T) {
	poly := []Vec2{
		{106, 35}, {119, 33}, {119, 32}, {150, 34}, {165, 39},
		{165, 200}, {106, 200},
	}
	got := Corners(poly, 60)
	for _, i := range got {
		if i == 1 || i == 2 {
			t.Fatalf("jog vertex %d flagged as corner: %v", i, got)
		}
	}
	if len(got) != 4 {
		t.Fatalf("want the 4 real corners (0,4,5,6), got %v", got)
	}
}
