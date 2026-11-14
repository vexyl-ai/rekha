package fit

import (
	"math"
	"testing"

	"github.com/vexyl-ai/rekha/internal/geom"
)

// onEllipse is n points of the ellipse (centre, a, b, theta) at equal
// parametric angles.
func onEllipse(centre geom.Vec2, a, b, theta float64, n int) []geom.Vec2 {
	pts := make([]geom.Vec2, n)
	for i := range pts {
		pts[i] = Ellipse{Centre: centre, A: a, B: b, Theta: theta}.At(2 * math.Pi * float64(i) / float64(n))
	}
	return pts
}

func TestEllipseRecoversParameters(t *testing.T) {
	c := geom.Vec2{X: 40, Y: 30}
	e, ok := FitEllipse(onEllipse(c, 25, 10, 0.6, 72))
	if !ok {
		t.Fatal("exact ellipse not fitted")
	}
	if e.Centre.Sub(c).Len() > 1e-6 || math.Abs(e.A-25) > 1e-6 || math.Abs(e.B-10) > 1e-6 || math.Abs(e.Theta-0.6) > 1e-6 {
		t.Fatalf("got %+v", e)
	}
}

func TestEllipseRecoversAxisAlignedAndSteep(t *testing.T) {
	// Theta 0 and an angle past 45°, where the A axis is nearer y.
	for _, th := range []float64{0, 1.2, -1.4} {
		e, ok := FitEllipse(onEllipse(geom.Vec2{X: 128, Y: 128}, 100, 60, th, 48))
		if !ok || math.Abs(e.A-100) > 1e-6 || math.Abs(e.B-60) > 1e-6 || math.Abs(e.Theta-th) > 1e-6 {
			t.Fatalf("theta %v: ok %v, got %+v", th, ok, e)
		}
	}
}

func TestEllipseRejectsCollinear(t *testing.T) {
	var pts []geom.Vec2
	for i := 0; i < 12; i++ {
		pts = append(pts, geom.Vec2{X: float64(i), Y: 2 * float64(i)})
	}
	if _, ok := FitEllipse(pts); ok {
		t.Fatal("collinear points fitted")
	}
}

func TestEllipseRejectsHyperbola(t *testing.T) {
	var pts []geom.Vec2
	for i := 1; i <= 12; i++ {
		x := float64(i)
		pts = append(pts, geom.Vec2{X: x, Y: 100 / x}, geom.Vec2{X: -x, Y: -100 / x})
	}
	if e, ok := FitEllipse(pts); ok {
		t.Fatalf("hyperbola fitted as %+v", e)
	}
}

func TestEllipseRejectsTooFew(t *testing.T) {
	if _, ok := FitEllipse(onEllipse(geom.Vec2{X: 40, Y: 30}, 25, 10, 0.6, 5)); ok {
		t.Fatal("five points fitted")
	}
}

func TestEllipseDist(t *testing.T) {
	e := Ellipse{Centre: geom.Vec2{X: 40, Y: 30}, A: 25, B: 10, Theta: 0.6}
	for _, phi := range []float64{0, 0.7, 2, 4.1, 6} {
		if d := e.Dist(e.At(phi)); d > 1e-9 {
			t.Fatalf("phi %v: on-curve distance %v", phi, d)
		}
	}
	u := geom.Vec2{X: math.Cos(0.6), Y: math.Sin(0.6)}
	if d := e.Dist(e.Centre.Add(u.Scale(26))); math.Abs(d-1) > 1e-6 {
		t.Fatalf("a point 1 px past the A vertex is %v away", d)
	}
	w := geom.Vec2{X: -math.Sin(0.6), Y: math.Cos(0.6)}
	if d := e.Dist(e.Centre.Add(w.Scale(9.5))); math.Abs(d-0.5) > 1e-6 {
		t.Fatalf("a point half a px inside the B vertex is %v away", d)
	}
}
