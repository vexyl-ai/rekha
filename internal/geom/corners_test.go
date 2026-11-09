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

func TestNearestIndex(t *testing.T) {
	dense := []Point{{0, 0}, {1, 0}, {2, 0}, {2, 1}}
	if got := NearestIndex(dense, Vec2{1.9, 0.9}); got != 3 {
		t.Fatalf("got %d, want 3", got)
	}
	if got := NearestIndex(dense, Vec2{1.5, 0}); got != 1 {
		t.Fatalf("tie should pick lower index, got %d", got)
	}
}
