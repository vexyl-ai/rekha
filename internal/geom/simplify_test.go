package geom

import "testing"

// crackRect returns the crack loop of a w×h rectangle at origin, clockwise,
// one point per unit step.
func crackRect(w, h int) []Point {
	var p []Point
	for x := 0; x < w; x++ {
		p = append(p, Point{x, 0})
	}
	for y := 0; y < h; y++ {
		p = append(p, Point{w, y})
	}
	for x := w; x > 0; x-- {
		p = append(p, Point{x, h})
	}
	for y := h; y > 0; y-- {
		p = append(p, Point{0, y})
	}
	return p
}

func TestMergeCollinearSquare(t *testing.T) {
	got := MergeCollinear(crackRect(10, 10))
	if len(got) != 4 {
		t.Fatalf("got %d points %v", len(got), got)
	}
}

func TestStaircaseBecomesDiagonal(t *testing.T) {
	// Triangle-ish loop: staircase from (0,0) to (8,8), then straight to (0,8), then up.
	var p []Point
	for i := 0; i < 8; i++ {
		p = append(p, Point{i, i}, Point{i + 1, i})
	}
	p = append(p, Point{8, 8}, Point{0, 8})
	got := RemoveStaircases(MergeCollinear(p))
	if len(got) != 3 {
		t.Fatalf("got %d points %v, want 3", len(got), got)
	}
}

func TestStaircaseKeepsOnePxRect(t *testing.T) {
	got := RemoveStaircases(MergeCollinear(crackRect(1, 5)))
	if len(got) != 4 {
		t.Fatalf("got %d points %v, want 4", len(got), got)
	}
}

func TestStaircaseKeepsIsolatedNotch(t *testing.T) {
	// 10x10 square with a 1-px notch cut into the top edge at x=5.
	sq := []Point{{0, 0}, {5, 0}, {5, 1}, {6, 1}, {6, 0}, {10, 0}, {10, 10}, {0, 10}}
	got := RemoveStaircases(sq)
	if len(got) != 8 {
		t.Fatalf("got %d points %v, want 8", len(got), got)
	}
}
