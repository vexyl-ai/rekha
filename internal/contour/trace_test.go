package contour

import (
	"math/rand/v2"
	"testing"

	"github.com/vexyl-ai/rekha/internal/bitmap"
	"github.com/vexyl-ai/rekha/internal/geom"
)

func TestSinglePixel(t *testing.T) {
	cs := Trace(bitmap.Parse([]string{"#"}), Minority)
	if len(cs) != 1 {
		t.Fatalf("got %d contours", len(cs))
	}
	c := cs[0]
	if c.Area2 != 2 || len(c.Pts) != 4 || c.Pts[0] != (geom.Point{X: 0, Y: 0}) {
		t.Fatalf("contour %+v", c)
	}
}

func TestAllBlackTouchesBorder(t *testing.T) {
	cs := Trace(bitmap.Parse([]string{"####", "####", "####", "####"}), Minority)
	if len(cs) != 1 || cs[0].Area2 != 32 {
		t.Fatalf("got %+v", cs)
	}
}

func TestRingHasHole(t *testing.T) {
	cs := Trace(bitmap.Parse([]string{
		"#####",
		"#...#",
		"#...#",
		"#...#",
		"#####",
	}), Minority)
	if len(cs) != 2 {
		t.Fatalf("got %d contours", len(cs))
	}
	areas := map[int]bool{cs[0].Area2: true, cs[1].Area2: true}
	if !areas[50] || !areas[-18] {
		t.Fatalf("areas %v, want {50,-18}", areas)
	}
}

func TestCheckerboardPolicies(t *testing.T) {
	bm := bitmap.Parse([]string{"#.", ".#"})
	cases := map[TurnPolicy]int{Black: 1, White: 2, Minority: 2, Majority: 1}
	for pol, want := range cases {
		if got := len(Trace(bm, pol)); got != want {
			t.Errorf("%s: %d contours, want %d", pol, got, want)
		}
	}
}

func TestEveryContourClosed(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 7))
	for iter := 0; iter < 50; iter++ {
		bm := bitmap.New(16, 16)
		for y := 0; y < 16; y++ {
			for x := 0; x < 16; x++ {
				bm.Set(x, y, rng.IntN(2) == 0)
			}
		}
		for _, pol := range []TurnPolicy{Minority, Majority, Black, White} {
			for _, c := range Trace(bm, pol) {
				n := len(c.Pts)
				if n < 4 {
					t.Fatalf("contour with %d points", n)
				}
				for i := 0; i < n; i++ {
					a, b := c.Pts[i], c.Pts[(i+1)%n]
					dx, dy := a.X-b.X, a.Y-b.Y
					if dx*dx+dy*dy != 1 {
						t.Fatalf("non-unit step %v -> %v", a, b)
					}
				}
				if c.Area2 == 0 || c.Area2 != geom.Area2(c.Pts) {
					t.Fatalf("bad area %d", c.Area2)
				}
			}
		}
	}
}
