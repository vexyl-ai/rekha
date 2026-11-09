package refine

import (
	"math"
	"testing"

	"github.com/vexyl-ai/rekha/internal/fit"
)

// coverageMask is a Mask backed by a coverage array over a window.
type coverageMask struct {
	win Window
	cov []float32
}

func (m coverageMask) At(x, y int) (float32, float32) {
	x, y = x-m.win.X0, y-m.win.Y0
	if x < 0 || y < 0 || x >= m.win.W || y >= m.win.H {
		return 0, 1
	}
	return m.cov[y*m.win.W+x], 1
}

func maskOf(b Bezigon, win Window) coverageMask {
	cov := make([]float32, win.W*win.H)
	Coverage(b, win, cov)
	return coverageMask{win, cov}
}

// figureEight is a self-crossing bow tie: (0,0) → (40,40) → (40,0) → (0,40).
func figureEight() Bezigon {
	return FromSegments([]fit.Segment{
		ln(v(0, 0), v(40, 40)), ln(v(40, 40), v(40, 0)), ln(v(40, 0), v(0, 40)), ln(v(0, 40), v(0, 0)),
	})
}

func TestAnglePriorSquare(t *testing.T) {
	if got := AnglePrior(square(0, 0, 10, 10)); math.Abs(got-2*math.Pi) > 1e-6 {
		t.Fatalf("square angle prior %v, want 2π", got)
	}
	if got := AnglePrior(circle(32, 32, 20)); got > 1e-3 {
		t.Fatalf("circle angle prior %v, want ~0", got)
	}
	// Joint subset: the square's joints 0 and 1 give π.
	if got := AnglePrior(square(0, 0, 10, 10), 0, 1); math.Abs(got-math.Pi) > 1e-6 {
		t.Fatalf("two joints %v, want π", got)
	}
}

func TestHandlePriorNearZero(t *testing.T) {
	sq := square(0, 0, 10, 10)
	if got := HandlePrior(sq); got >= 10 {
		t.Fatalf("square handle prior %v", got)
	}
	sq.Pts[1] = sq.Pts[0].Add(v(1e-9, 0))
	if got := HandlePrior(sq); got <= 1e5 {
		t.Fatalf("near-zero handle prior %v", got)
	}
	if got := HandlePrior(sq, 2); got >= 10 {
		t.Fatalf("segment 2 alone should be unaffected: %v", got)
	}
}

func TestLengthPriorEqualsPolyline(t *testing.T) {
	c := circle(32, 32, 20)
	if got, want := LengthPrior(c), c.Length(); math.Abs(got-want) > 1e-9 {
		t.Fatalf("%v vs %v", got, want)
	}
	sq := square(0, 0, 10, 10)
	if got := LengthPrior(sq, 0, 1); math.Abs(got-20) > 1e-9 {
		t.Fatalf("two sides %v, want 20", got)
	}
}

func TestSelfIntersectionFigureEight(t *testing.T) {
	if got := SelfIntersectionPrior(figureEight()); got <= 0 {
		t.Fatalf("figure eight prior %v, want > 0", got)
	}
	if got := SelfIntersectionPrior(circle(32, 32, 20)); got != 0 {
		t.Fatalf("circle prior %v, want 0", got)
	}
}

func TestDataEnergyZeroOnOwnCoverage(t *testing.T) {
	win := Window{X0: 0, Y0: 0, W: 64, H: 64}
	c := circle(32, 32, 20)
	cov := make([]float32, 64*64)
	Coverage(c, win, cov)
	if got := DataEnergy(cov, maskOf(c, win), win, c.Length()); got != 0 {
		t.Fatalf("own coverage energy %v", got)
	}
	shifted := circle(33, 32, 20)
	if got := DataEnergy(cov, maskOf(shifted, win), win, c.Length()); got <= 0 {
		t.Fatalf("shifted energy %v", got)
	}
}

func TestWeightsPrior(t *testing.T) {
	sq := square(0, 0, 10, 10)
	w := DefaultWeights
	want := w.Spt*SelfIntersectionPrior(sq) + w.Apt*AnglePrior(sq) + w.Hpt*HandlePrior(sq) + w.Lpt*LengthPrior(sq)/40
	if got := w.Prior(sq, 40); math.Abs(got-want) > 1e-12 {
		t.Fatalf("%v vs %v", got, want)
	}
}
