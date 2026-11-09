package contour

import (
	"testing"
	"time"

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
	want := []geom.Point{{X: 4, Y: 0}, {X: 1, Y: 1}, {X: 4, Y: 2}}
	for i, s := range shapes {
		if s.Outer.Pts[0] != want[i] {
			t.Fatalf("shape %d starts at %v, want %v", i, s.Outer.Pts[0], want[i])
		}
	}
}

// A page of small rings (text-like input) has thousands of holes and
// thousands of outers; hole assignment must not test every hole against
// every outer's full polygon.
func TestBuildShapesManyRingsIsFast(t *testing.T) {
	const g, cell = 80, 8
	bm := bitmap.New(g*cell, g*cell)
	for gy := 0; gy < g; gy++ {
		for gx := 0; gx < g; gx++ {
			for y := 1; y < cell-1; y++ {
				for x := 1; x < cell-1; x++ {
					if x == 1 || y == 1 || x == cell-2 || y == cell-2 {
						bm.Set(gx*cell+x, gy*cell+y, true)
					}
				}
			}
		}
	}
	cs := Trace(bm, Minority)
	start := time.Now()
	shapes := BuildShapes(cs, 0)
	elapsed := time.Since(start)
	if len(shapes) != g*g {
		t.Fatalf("got %d shapes, want %d", len(shapes), g*g)
	}
	for _, s := range shapes {
		if len(s.Holes) != 1 {
			t.Fatalf("shape at %v has %d holes, want 1", s.Outer.Pts[0], len(s.Holes))
		}
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("BuildShapes took %v for %d rings, want well under 500ms", elapsed, g*g)
	}
}
