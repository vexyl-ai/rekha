package bitmap

import "testing"

func TestGetOutsideIsFalse(t *testing.T) {
	b := New(4, 4)
	b.Set(0, 0, true)
	for _, p := range [][2]int{{-1, 0}, {0, -1}, {4, 0}, {0, 4}, {100, 100}} {
		if b.Get(p[0], p[1]) {
			t.Fatalf("Get(%d,%d) outside should be false", p[0], p[1])
		}
	}
}

func TestSetGetRoundTrip(t *testing.T) {
	b := New(70, 3)
	b.Set(65, 2, true)
	if !b.Get(65, 2) || b.Get(64, 2) || b.Get(65, 1) {
		t.Fatal("set/get mismatch across word boundary")
	}
	if b.Count() != 1 {
		t.Fatalf("Count = %d, want 1", b.Count())
	}
	b.Set(65, 2, false)
	if b.Get(65, 2) || b.Count() != 0 {
		t.Fatal("clear failed")
	}
}

func TestParse(t *testing.T) {
	b := Parse([]string{"#.", ".#"})
	if b.W != 2 || b.H != 2 {
		t.Fatalf("size %dx%d", b.W, b.H)
	}
	if !b.Get(0, 0) || !b.Get(1, 1) || b.Get(1, 0) || b.Get(0, 1) {
		t.Fatal("Parse pattern wrong")
	}
}
