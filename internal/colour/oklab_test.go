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

// An sRGB-blended mixture of two colours is a mixture (distance ~0), though
// it can sit far from the straight OKLab segment between them.
func TestMixDistFollowsSRGBBlend(t *testing.T) {
	red, blue := FromSRGB8(220, 30, 30), FromSRGB8(30, 60, 220)
	mix := FromSRGB8(125, 45, 125)
	if d := MixDist(mix, red, blue); d > 0.01 {
		t.Fatalf("MixDist of the 50%% blend = %.3f, want ~0", d)
	}
	if d := SegmentDist(mix, red, blue); d < 0.05 {
		t.Fatalf("SegmentDist of the 50%% blend = %.3f; expected the OKLab segment to miss it", d)
	}
	if d := MixDist(FromSRGB8(0, 255, 0), red, blue); d < 0.2 {
		t.Fatalf("green reads as a red/blue mixture: %.3f", d)
	}
}

// srgbToLinear is a table; it must equal the sRGB transfer formula for
// every 8-bit value so quantisation output is unchanged by the table.
func TestSRGBToLinearTableMatchesFormula(t *testing.T) {
	for c := 0; c < 256; c++ {
		v := float64(c) / 255
		want := v / 12.92
		if v > 0.04045 {
			want = math.Pow((v+0.055)/1.055, 2.4)
		}
		if got := srgbToLinear(uint8(c)); got != want {
			t.Fatalf("%d: table %v formula %v", c, got, want)
		}
	}
}
