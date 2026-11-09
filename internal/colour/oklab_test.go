package colour

import (
	"math"
	"testing"
)

func near(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

// Reference values from Ottosson's OKLab article.
func TestOKLabReferenceValues(t *testing.T) {
	cases := []struct {
		r, g, b  uint8
		l, a, bb float64
	}{
		{255, 255, 255, 1, 0, 0},
		{255, 0, 0, 0.628, 0.225, 0.126},
		{0, 0, 0, 0, 0, 0},
	}
	for _, c := range cases {
		got := FromSRGB8(c.r, c.g, c.b)
		if !near(got.L, c.l, 1e-3) || !near(got.A, c.a, 1e-3) || !near(got.B, c.bb, 1e-3) {
			t.Errorf("(%d,%d,%d) → %+v, want L %.3f A %.3f B %.3f", c.r, c.g, c.b, got, c.l, c.a, c.bb)
		}
	}
}

func TestOKLabRoundTripGreys(t *testing.T) {
	for g := 0; g < 256; g++ {
		v := uint8(g)
		r, gg, b := FromSRGB8(v, v, v).ToSRGB8()
		if r != v || gg != v || b != v {
			t.Fatalf("grey %d → %d,%d,%d", g, r, gg, b)
		}
	}
}

func TestOKLabRoundTripSampled(t *testing.T) {
	for r := 0; r < 256; r += 17 {
		for g := 0; g < 256; g += 17 {
			for b := 0; b < 256; b += 17 {
				rr, gg, bb := FromSRGB8(uint8(r), uint8(g), uint8(b)).ToSRGB8()
				if d := max(abs(int(rr)-r), abs(int(gg)-g), abs(int(bb)-b)); d > 1 {
					t.Fatalf("(%d,%d,%d) → (%d,%d,%d)", r, g, b, rr, gg, bb)
				}
			}
		}
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func TestOKLabDistWhiteBlack(t *testing.T) {
	if d := Dist(FromSRGB8(255, 255, 255), FromSRGB8(0, 0, 0)); !near(d, 1, 1e-6) {
		t.Fatalf("white–black distance %v, want 1", d)
	}
	if h := FromSRGB8(255, 0, 0).Hex(); h != "#ff0000" {
		t.Fatalf("hex %q", h)
	}
}
