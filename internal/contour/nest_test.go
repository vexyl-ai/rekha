package contour

import (
	"reflect"
	"testing"
	"time"

	"github.com/vexyl-ai/rekha/internal/bitmap"
)

// Shapes of two colours: a 9×9 block with a 5×5 hole (colour A) and, in
// the hole, a 3×3 ring with a 1-px hole (colour B) and, in that, one pixel
// (colour A again). Nest must chain pixel → ring → block.
func TestNestRingDepths(t *testing.T) {
	a := bitmap.Parse([]string{
		"#########",
		"#########",
		"##.....##",
		"##.....##",
		"##..#..##",
		"##.....##",
		"##.....##",
		"#########",
		"#########",
	})
	b := bitmap.Parse([]string{
		".........",
		".........",
		".........",
		"...###...",
		"...#.#...",
		"...###...",
		".........",
		".........",
		".........",
	})
	shapes := append(BuildShapes(Trace(a, Minority), 0), BuildShapes(Trace(b, Minority), 0)...)
	if len(shapes) != 3 {
		t.Fatalf("got %d shapes", len(shapes))
	}
	// BuildShapes sorts by first point: block (0,0), pixel (4,4); then ring (3,3).
	if got, want := Nest(shapes, a.W, a.H), []int{-1, 2, 0}; !reflect.DeepEqual(got, want) {
		t.Fatalf("parents %v, want %v", got, want)
	}

	side := bitmap.Parse([]string{
		"##..##",
		"##..##",
	})
	if got, want := Nest(BuildShapes(Trace(side, Minority), 0), side.W, side.H), []int{-1, -1}; !reflect.DeepEqual(got, want) {
		t.Fatalf("side by side parents %v, want %v", got, want)
	}
}

// TraceFunc lets the caller resolve each saddle: the same checkerboard
// comes out as one 8-connected diagonal or as two separate pixels.
func TestTraceFuncResolvesSaddlesPerVertex(t *testing.T) {
	bm := bitmap.Parse([]string{
		"#.",
		".#",
	})
	joined := TraceFunc(bm, func(x, y int) bool { return true })
	if len(joined) != 1 {
		t.Fatalf("joined: %d contours, want 1", len(joined))
	}
	split := TraceFunc(bm, func(x, y int) bool { return x != 1 || y != 1 })
	if len(split) != 2 {
		t.Fatalf("split: %d contours, want 2", len(split))
	}
}

// Nest must scale with the image, not with the square of the shape count:
// a 2-pixel checkerboard at 1024² traces to about 130 000 shapes.
func TestNestScalesToManyShapes(t *testing.T) {
	const n = 1024
	bm := bitmap.New(n, n)
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			if (x/2+y/2)%2 == 0 {
				bm.Set(x, y, true)
			}
		}
	}
	shapes := BuildShapes(Trace(bm, Minority), 0)
	if len(shapes) < 100000 {
		t.Fatalf("only %d shapes", len(shapes))
	}
	start := time.Now()
	parents := Nest(shapes, n, n)
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("Nest took %v for %d shapes", d, len(shapes))
	}
	for i, p := range parents {
		if p != -1 {
			t.Fatalf("shape %d has parent %d; checkerboard squares do not nest", i, p)
		}
	}
}
