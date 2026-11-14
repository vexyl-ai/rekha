package refine

import (
	"math"
	"testing"

	"github.com/vexyl-ai/rekha/internal/fit"
	"github.com/vexyl-ai/rekha/internal/geom"
)

func TestEllipseBezigonStartAndOrientation(t *testing.T) {
	e := fit.Ellipse{Centre: v(50, 40), A: 17, B: 9, Theta: 0.4}
	for _, ccw := range []bool{true, false} {
		b := EllipseBezigon(e, e.At(0), ccw)
		if b.N() != 4 {
			t.Fatalf("N = %d", b.N())
		}
		if !near(b.Pts[0], e.At(0), 1e-9) {
			t.Fatalf("ccw %v: start %v, want %v", ccw, b.Pts[0], e.At(0))
		}
		if a := signedArea2(b); (a > 0) != ccw {
			t.Fatalf("ccw %v: signed area %v", ccw, a)
		}
		for _, p := range fine(b) {
			if d := e.Dist(p); d > 0.03 {
				t.Fatalf("point %v is %v off the ellipse", p, d)
			}
		}
	}
	// A start off the curve begins at the nearest parametric angle: a
	// point beyond the B vertex starts there.
	b := EllipseBezigon(e, e.At(math.Pi/2).Add(e.At(math.Pi/2).Sub(e.Centre)), true)
	if !near(b.Pts[0], e.At(math.Pi/2), 1e-9) {
		t.Fatalf("start %v, want %v", b.Pts[0], e.At(math.Pi/2))
	}
}

// nudgedEllipse is the ellipse as n cubics between equal parametric
// angles, each anchor moved along its normal by d, alternately out and
// in, handles along the tangents.
func nudgedEllipse(e fit.Ellipse, n int, d float64) Bezigon {
	step := 2 * math.Pi / float64(n)
	k := 4.0 / 3 * math.Tan(step/4)
	deriv := func(phi float64) geom.Vec2 {
		return e.At(phi + 1e-6).Sub(e.At(phi - 1e-6)).Scale(1 / 2e-6)
	}
	anchor := func(i int) geom.Vec2 {
		phi := float64(i%n) * step
		p := e.At(phi)
		nrm := deriv(phi)
		nrm = v(nrm.Y, -nrm.X).Norm()
		if i%2 == 1 {
			return p.Sub(nrm.Scale(d))
		}
		return p.Add(nrm.Scale(d))
	}
	var segs []fit.Segment
	for i := 0; i < n; i++ {
		p0, p1 := anchor(i), anchor(i+1)
		d0, d1 := deriv(float64(i)*step), deriv(float64(i+1)*step)
		segs = append(segs, cubic(p0, p0.Add(d0.Scale(k)), p1.Sub(d1.Scale(k)), p1))
	}
	return FromSegments(segs)
}

func TestFitEllipseOnIdealEllipse(t *testing.T) {
	e := fit.Ellipse{Centre: v(128, 128), A: 100, B: 60, Theta: 0.5}
	got, ok := FitEllipse(nudgedEllipse(e, 8, 0), 0.5)
	if !ok {
		t.Fatal("ideal ellipse not recognised")
	}
	if got.Centre.Sub(e.Centre).Len() > 0.01 || math.Abs(got.A-100) > 0.05 || math.Abs(got.B-60) > 0.05 || math.Abs(got.Theta-0.5) > 1e-3 {
		t.Fatalf("got %+v", got)
	}
}

func TestFitEllipseRejectsCircle(t *testing.T) {
	// A circle is the circle snap's job; the ellipse refuses an axis
	// ratio under ellipseMinRatio.
	if e, ok := FitEllipse(Circle(v(50, 40), 17, v(67, 40), true), 0.5); ok {
		t.Fatalf("circle taken for an ellipse %+v", e)
	}
}

func TestFitEllipseRejectsRoundedRect(t *testing.T) {
	if e, ok := FitEllipse(roundedRect(), 0.5); ok {
		t.Fatalf("rounded rectangle taken for an ellipse %+v", e)
	}
}

func TestFitEllipseRejectsThin(t *testing.T) {
	e := fit.Ellipse{Centre: v(50, 40), A: 20, B: 2, Theta: 0}
	if _, ok := FitEllipse(nudgedEllipse(e, 8, 0), 0.5); ok {
		t.Fatal("an ellipse with a 2 px semi-axis accepted")
	}
}

// TestSnapEllipseAcceptsNudgedEllipse: an eight-cubic outline of the
// ellipse (100, 60) at 0.5 rad, each anchor nudged ±0.15 px, judged
// against the exact ellipse's mask, becomes four cubics on the ellipse.
func TestSnapEllipseAcceptsNudgedEllipse(t *testing.T) {
	win := Window{0, 0, 256, 256}
	e := fit.Ellipse{Centre: v(128, 128), A: 100, B: 60, Theta: 0.5}
	m := supersampled(EllipseBezigon(e, e.At(0), true), win)
	b := nudgedEllipse(e, 8, 0.15)
	out, ok := SnapEllipse(b, m, win, 0.5)
	if !ok || out.N() != 4 {
		t.Fatalf("ok=%v N=%d", ok, out.N())
	}
	got, ok := FitEllipse(out, 0.5)
	if !ok || math.Abs(got.A-100) > 0.15 || math.Abs(got.B-60) > 0.15 || got.Centre.Sub(e.Centre).Len() > 0.1 {
		t.Fatalf("snapped to %+v", got)
	}
}

func TestSnapEllipseRefusesOutsideTheWindow(t *testing.T) {
	win := Window{0, 0, 32, 32}
	e := fit.Ellipse{Centre: v(200, 200), A: 20, B: 12, Theta: 0}
	out, ok := SnapEllipse(nudgedEllipse(e, 8, 0), maskOf(circle(16, 16, 8), win), win, 0.5)
	if ok {
		t.Fatalf("snapped an ellipse the window does not see: N=%d", out.N())
	}
}
