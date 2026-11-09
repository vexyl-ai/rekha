package geom

import "testing"

func TestRDPKeepsCorners(t *testing.T) {
	var pts []Vec2
	for x := 0; x <= 5; x++ {
		pts = append(pts, Vec2{float64(x), 0})
	}
	for y := 1; y <= 5; y++ {
		pts = append(pts, Vec2{5, float64(y)})
	}
	got := RDP(pts, 0.5)
	if len(got) != 3 {
		t.Fatalf("got %d points %v, want 3", len(got), got)
	}
}

func TestRDPRemovesStaircase(t *testing.T) {
	var pts []Vec2
	for i := 0; i < 8; i++ {
		pts = append(pts, Vec2{float64(i), float64(i)}, Vec2{float64(i + 1), float64(i)})
	}
	pts = append(pts, Vec2{8, 8})
	got := RDP(pts, 0.75)
	if len(got) != 2 {
		t.Fatalf("got %d points %v, want 2", len(got), got)
	}
}

func TestRDPClosedSquare(t *testing.T) {
	var pts []Vec2
	for i := 0; i < 10; i++ {
		pts = append(pts, Vec2{float64(i), 0})
	}
	for i := 0; i < 10; i++ {
		pts = append(pts, Vec2{10, float64(i)})
	}
	for i := 10; i > 0; i-- {
		pts = append(pts, Vec2{float64(i), 10})
	}
	for i := 10; i > 0; i-- {
		pts = append(pts, Vec2{0, float64(i)})
	}
	got := RDPClosed(pts, 0.5)
	if len(got) != 4 {
		t.Fatalf("got %d points %v, want 4", len(got), got)
	}
	want := map[Vec2]bool{{0, 0}: true, {10, 0}: true, {10, 10}: true, {0, 10}: true}
	for _, p := range got {
		if !want[p] {
			t.Fatalf("unexpected vertex %v", p)
		}
	}
}
