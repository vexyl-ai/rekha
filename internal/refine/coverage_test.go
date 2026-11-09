package refine

import (
	"math"
	"testing"

	"github.com/vexyl-ai/rekha/internal/geom"
)

func sum(cov []float32) float64 {
	s := 0.0
	for _, c := range cov {
		s += float64(c)
	}
	return s
}

func TestCoverageSquareExact(t *testing.T) {
	win := Window{X0: 0, Y0: 0, W: 8, H: 8}
	out := make([]float32, 64)
	Coverage(square(2, 2, 6, 6), win, out)
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			want := 0.0
			if x >= 2 && x < 6 && y >= 2 && y < 6 {
				want = 1
			}
			if math.Abs(float64(out[y*8+x])-want) > 1e-6 {
				t.Fatalf("(%d,%d) = %v, want %v", x, y, out[y*8+x], want)
			}
		}
	}
	Coverage(square(1.5, 2, 5.5, 6), win, out)
	if math.Abs(float64(out[3*8+1])-0.5) > 1e-6 || math.Abs(float64(out[3*8+5])-0.5) > 1e-6 || math.Abs(float64(out[3*8+3])-1) > 1e-6 {
		t.Fatalf("half-shifted row: %v", out[24:32])
	}
}

func TestCoverageCircleArea(t *testing.T) {
	win := Window{X0: 0, Y0: 0, W: 64, H: 64}
	out := make([]float32, 64*64)
	Coverage(circle(32, 32, 20), win, out)
	if got, want := sum(out), math.Pi*400; math.Abs(got-want)/want > 0.005 {
		t.Fatalf("circle area %v, want %v", got, want)
	}
}

func TestCoverageContinuity(t *testing.T) {
	win := Window{X0: 0, Y0: 0, W: 8, H: 8}
	a, b := make([]float32, 64), make([]float32, 64)
	sq := square(2, 2, 6, 6)
	Coverage(sq, win, a)
	sq.Pts[3] = sq.Pts[3].Add(geom.Vec2{X: 1e-3, Y: -1e-3})
	Coverage(sq, win, b)
	for i := range a {
		if d := math.Abs(float64(a[i] - b[i])); d > 1e-2 {
			t.Fatalf("pixel %d jumped by %v", i, d)
		}
	}
}

func TestCoverageWindowOffsetAndClip(t *testing.T) {
	// A shape partly left of and above the window still covers the window
	// cells it overlaps.
	win := Window{X0: 4, Y0: 4, W: 4, H: 4}
	out := make([]float32, 16)
	Coverage(square(2, 2, 6, 6), win, out)
	if math.Abs(float64(out[0])-1) > 1e-6 || math.Abs(float64(out[1*4+1])-1) > 1e-6 || out[3*4+3] != 0 {
		t.Fatalf("offset window: %v", out)
	}
}

func TestAccumulateThenResolveEqualsCoverage(t *testing.T) {
	win := Window{X0: 0, Y0: 0, W: 64, H: 64}
	c := circle(30, 34, 18)
	want := make([]float32, 64*64)
	Coverage(c, win, want)
	acc := make([]float32, 64*64)
	AccumulateEdges(c.Polyline(), true, win, acc)
	got := make([]float32, 64*64)
	Resolve(acc, win, got)
	for i := range want {
		if math.Abs(float64(want[i]-got[i])) > 1e-6 {
			t.Fatalf("pixel %d: %v vs %v", i, want[i], got[i])
		}
	}
}
