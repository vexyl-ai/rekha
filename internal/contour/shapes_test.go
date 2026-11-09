package contour

import (
	"testing"

	"github.com/vexyl-ai/rekha/internal/bitmap"
	"github.com/vexyl-ai/rekha/internal/geom"
)

func TestHoleAssignedToSmallestOuter(t *testing.T) {
	// Big 9x9 block with a 3x3 ring (1-px wall) cut into it: the ring sits in
	// a 5x5 white hole of the block. Hole of the ring belongs to the ring.
	bm := bitmap.Parse([]string{
		"#########",
		"#########",
		"##.....##",
		"##.###.##",
		"##.#.#.##",
		"##.###.##",
		"##.....##",
		"#########",
		"#########",
	})
	shapes := BuildShapes(Trace(bm, Minority), 0)
	if len(shapes) != 2 {
		t.Fatalf("got %d shapes", len(shapes))
	}
	for _, s := range shapes {
		if len(s.Holes) != 1 {
			t.Fatalf("shape area %d has %d holes, want 1", s.Outer.Area2, len(s.Holes))
		}
	}
	// The ring (area2 = 18) must own the 1-px hole (area2 = -2).
	for _, s := range shapes {
		if s.Outer.Area2 == 18 && s.Holes[0].Area2 != -2 {
			t.Fatalf("ring hole area %d, want -2", s.Holes[0].Area2)
		}
	}
}

func TestDespeckleDropsSmallOuterAndHole(t *testing.T) {
	bm := bitmap.Parse([]string{
		"##########....",
		"##########....",
		"####.#####..#.",
		"##########....",
		"##########....",
		"##########....",
		"##########....",
		"##########....",
		"##########....",
		"##########....",
	})
	shapes := BuildShapes(Trace(bm, Minority), 4)
	if len(shapes) != 1 || len(shapes[0].Holes) != 0 {
		t.Fatalf("got %+v", shapes)
	}
}

func TestShapesSorted(t *testing.T) {
	bm := bitmap.Parse([]string{
		"....#",
		".#...",
		"....#",
	})
	shapes := BuildShapes(Trace(bm, Minority), 0)
	if len(shapes) != 3 {
		t.Fatalf("got %d", len(shapes))
	}
	want := []geom.Point{{4, 0}, {1, 1}, {4, 2}}
	for i, s := range shapes {
		if s.Outer.Pts[0] != want[i] {
			t.Fatalf("shape %d starts at %v, want %v", i, s.Outer.Pts[0], want[i])
		}
	}
}
