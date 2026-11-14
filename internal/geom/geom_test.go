package geom

import (
	"math"
	"testing"
)

func TestArea2Square(t *testing.T) {
	sq := []Point{{0, 0}, {1, 0}, {1, 1}, {0, 1}}
	if got := Area2(sq); got != 2 {
		t.Fatalf("Area2 = %d, want 2", got)
	}
	rev := []Point{{0, 1}, {1, 1}, {1, 0}, {0, 0}}
	if got := Area2(rev); got != -2 {
		t.Fatalf("Area2 reversed = %d, want -2", got)
	}
}

func TestContains(t *testing.T) {
	sq := []Point{{0, 0}, {1, 0}, {1, 1}, {0, 1}}
	if !Contains(sq, Point{0, 0}) {
		t.Fatal("corner on edge should count as inside")
	}
	if Contains(sq, Point{2, 2}) {
		t.Fatal("(2,2) outside")
	}
	ring := []Point{{0, 0}, {3, 0}, {3, 3}, {0, 3}}
	if !Contains(ring, Point{1, 1}) {
		t.Fatal("centre of 3x3 inside")
	}
}

func TestNormZero(t *testing.T) {
	if got := (Vec2{}).Norm(); got != (Vec2{}) {
		t.Fatalf("Norm(0) = %v", got)
	}
}

func TestPerpDistDegenerate(t *testing.T) {
	if got := PerpDist(Vec2{3, 4}, Vec2{}, Vec2{}); math.Abs(got-5) > 1e-12 {
		t.Fatalf("PerpDist = %v, want 5", got)
	}
}

func TestAddScaled(t *testing.T) {
	got := Vec2{1, 2}.AddScaled(Vec2{3, -4}, 0.5)
	if got != (Vec2{2.5, 0}) {
		t.Fatalf("got %v", got)
	}
}
