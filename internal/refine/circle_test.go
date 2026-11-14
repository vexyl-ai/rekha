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

func TestFitCircleRejectsSmallSquare(t *testing.T) {
	// A 2×2 square is within 0.5 px of a circle of radius 1.2 in absolute
	// terms, but 15 % of the radius off: not a circle at any scale.
	for _, side := range []float64{2, 4, 6} {
		if _, _, ok := FitCircle(square(10, 10, 10+side, 10+side), 0.5); ok {
			t.Fatalf("%v px square taken for a circle", side)
		}
	}
}

// nudgedCircle is the circle (centre, r) as n arc cubics whose anchors
// are moved radially by d, alternately out and in, handles rebuilt along
// the circle's tangents: a free outline that fits an imperfect image a
// little better than the exact circle does.
func nudgedCircle(centre geom.Vec2, r float64, n int, d float64) Bezigon {
	step := 2 * math.Pi / float64(n)
	k := 4.0 / 3 * math.Tan(step/4) * r
	anchor := func(i int) geom.Vec2 {
		a := float64(i%n) * step
		rr := r + d
		if i%2 == 1 {
			rr = r - d
		}
		return v(centre.X+rr*math.Cos(a), centre.Y+rr*math.Sin(a))
	}
	tangent := func(i int) geom.Vec2 {
		a := float64(i%n) * step
		return v(-math.Sin(a), math.Cos(a))
	}
	var segs []fit.Segment
	for i := 0; i < n; i++ {
		p0, p1 := anchor(i), anchor(i+1)
		segs = append(segs, cubic(p0, p0.Add(tangent(i).Scale(k)), p1.Sub(tangent(i+1).Scale(k)), p1))
	}
	return FromSegments(segs)
}

// TestSnapCircleAcceptsNearPerfectRing: an 8-cubic outline of a circle
// of r = 110, each anchor nudged ±0.15 px radially, judged against the
// exact circle's mask, becomes four cubics even though the free outline
// fits the mask a little better than any circle (the Pokéball case).
func TestSnapCircleAcceptsNearPerfectRing(t *testing.T) {
	win := Window{0, 0, 256, 256}
	exact := Circle(v(128, 128), 110, v(238, 128), true)
	m := supersampled(exact, win)
	b := nudgedCircle(v(128, 128), 110, 8, 0.15)
	out, ok := SnapCircle(b, m, win, 0.5)
	if !ok || out.N() != 4 {
		t.Fatalf("ok=%v N=%d", ok, out.N())
	}
	if r := out.Pts[0].Sub(v(128, 128)).Len(); math.Abs(r-110) > 0.1 {
		t.Fatalf("r = %.3f", r)
	}
}

// discMask is the 4×4-sampled coverage of the disc (centre, r) over win.
func discMask(centre geom.Vec2, r float64, win Window) coverageMask {
	cov := make([]float32, win.W*win.H)
	for y := 0; y < win.H; y++ {
		for x := 0; x < win.W; x++ {
			n := 0
			for sy := 0; sy < 4; sy++ {
				for sx := 0; sx < 4; sx++ {
					px := float64(win.X0+x) + (float64(sx)+0.5)/4
					py := float64(win.Y0+y) + (float64(sy)+0.5)/4
					if dx, dy := px-centre.X, py-centre.Y; dx*dx+dy*dy <= r*r {
						n++
					}
				}
			}
			cov[y*win.W+x] = float32(n) / 16
		}
	}
	return coverageMask{win, cov}
}

// TestSnapCircleRefitsRadiusOnLargeCircle: the refined outline of a
// large circle is inflated by its own polygon sag (an eight-piece fit by
// a quarter of what a four-piece one is), so the Kåsa radius corrected
// for four pieces lands half a pixel short at r = 850 and the mask
// refuses it; the snap must refit the radius against the mask.
func TestSnapCircleRefitsRadiusOnLargeCircle(t *testing.T) {
	win := Window{0, 0, 1800, 1800}
	c, r := v(900, 900), 850.0
	m := discMask(c, r, win)
	inflated := r / chordFactor(2*math.Pi/(8*Flatten))
	b := nudgedCircle(c, inflated, 8, 0)
	out, ok := SnapCircle(b, m, win, 0.5)
	if !ok || out.N() != 4 {
		t.Fatalf("ok=%v N=%d", ok, out.N())
	}
	// A four-cubic circle bulges 2.7e-4·r outward between its anchors
	// (0.23 px here), so the radius the mask likes puts the anchors a
	// tenth of a pixel inside the true circle.
	if got := out.Pts[0].Sub(c).Len(); math.Abs(got-r) > 0.15 {
		t.Fatalf("r = %.3f, want %.3f ± 0.15", got, r)
	}
}

// TestSnapCircleRefusesOutsideTheWindow: a loop whose window holds no
// pixel cannot be judged, so the snap is refused rather than admitted.
func TestSnapCircleRefusesOutsideTheWindow(t *testing.T) {
	win := Window{0, 0, 32, 32}
	b := circle(200, 200, 10)
	out, ok := SnapCircle(b, maskOf(circle(16, 16, 8), win), win, 0.5)
	if ok {
		t.Fatalf("snapped a circle the window does not see")
	}
	for i := range b.Pts {
		if out.Pts[i] != b.Pts[i] {
			t.Fatalf("point %d moved", i)
		}
	}
}
