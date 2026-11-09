package contour

import (
	"reflect"
	"testing"

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
	if got, want := Nest(shapes), []int{-1, 2, 0}; !reflect.DeepEqual(got, want) {
		t.Fatalf("parents %v, want %v", got, want)
	}

	side := bitmap.Parse([]string{
		"##..##",
		"##..##",
	})
	if got, want := Nest(BuildShapes(Trace(side, Minority), 0)), []int{-1, -1}; !reflect.DeepEqual(got, want) {
		t.Fatalf("side by side parents %v, want %v", got, want)
	}
}
