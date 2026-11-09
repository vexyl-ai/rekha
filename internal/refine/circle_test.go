package refine

import (
	"math"
	"testing"

	"github.com/vexyl-ai/rekha/internal/fit"
	"github.com/vexyl-ai/rekha/internal/geom"
)

// signedArea2 is twice the signed area of the flattened outline: positive
// when the outline runs counter-clockwise in y-down image coordinates.
func signedArea2(b Bezigon) float64 {
	pts := b.Polyline()
	a := 0.0
	for i := range pts {
		a += geom.Cross(pts[i], pts[(i+1)%len(pts)])
	}
	return a
}

func TestFitCircleOnIdealCircle(t *testing.T) {
	c, r, ok := FitCircle(Circle(v(50, 40), 17, v(67, 40), true), 0.5)
	if !ok {
		t.Fatal("ideal circle not recognised")
	}
	// The four-cubic circle is itself 0.00027·r off a true circle, so the
	// least-squares radius lands a few thousandths off.
	if !near(c, v(50, 40), 1e-6) || math.Abs(r-17) > 0.01 {
		t.Fatalf("centre %v r %v, want (50,40) 17", c, r)
	}
}

// roundedRect is a 60×40 rectangle at (10,10) with radius-10 corners:
// four lines and four quarter arcs.
func roundedRect() Bezigon {
	k := kappa * 10
	arc := func(a, b, c, d geom.Vec2) fit.Segment { return cubic(a, b, c, d) }
	return FromSegments([]fit.Segment{
		ln(v(20, 10), v(60, 10)),
		arc(v(60, 10), v(60+k, 10), v(70, 20-k), v(70, 20)),
		ln(v(70, 20), v(70, 40)),
		arc(v(70, 40), v(70, 40+k), v(60+k, 50), v(60, 50)),
		ln(v(60, 50), v(20, 50)),
		arc(v(20, 50), v(20-k, 50), v(10, 40+k), v(10, 40)),
		ln(v(10, 40), v(10, 20)),
		arc(v(10, 20), v(10, 20-k), v(20-k, 10), v(20, 10)),
	})
}

func TestFitCircleRejectsRoundedRect(t *testing.T) {
	if _, _, ok := FitCircle(roundedRect(), 0.5); ok {
		t.Fatal("rounded rectangle taken for a circle")
	}
}

func TestFitCircleRejectsTwoSegments(t *testing.T) {
	// Two half-circle cubics: on a circle, but too few segments to trust.
	b := Bezigon{Pts: []geom.Vec2{v(52, 32), v(52, 58.5), v(12, 58.5), v(12, 32), v(12, 5.5), v(52, 5.5)}}
	if _, _, ok := FitCircle(b, 0.5); ok {
		t.Fatal("two-segment shape accepted")
	}
}

func TestCircleStartAndOrientation(t *testing.T) {
	for _, ccw := range []bool{true, false} {
		b := Circle(v(50, 40), 17, v(67, 40), ccw)
		if b.N() != 4 {
			t.Fatalf("N = %d", b.N())
		}
		if !near(b.Pts[0], v(67, 40), 1e-9) {
			t.Fatalf("ccw %v: start %v, want (67,40)", ccw, b.Pts[0])
		}
		if h := b.Pts[1].Sub(b.Pts[0]).Len(); math.Abs(h-kappa*17) > 1e-9 {
			t.Fatalf("handle %v, want %v", h, kappa*17)
		}
		if a := signedArea2(b); (a > 0) != ccw {
			t.Fatalf("ccw %v: signed area %v", ccw, a)
		}
		// Every anchor and the curve lie on the circle.
		for _, p := range fine(b) {
			if d := math.Abs(p.Sub(v(50, 40)).Len() - 17); d > 0.03 {
				t.Fatalf("point %v is %v off the circle", p, d)
			}
		}
	}
	// A start off the circle snaps to the nearest circle point.
	b := Circle(v(50, 40), 17, v(50, 0), true)
	if !near(b.Pts[0], v(50, 23), 1e-9) {
		t.Fatalf("start %v, want (50,23)", b.Pts[0])
	}
}
