package fit

import (
	"math"
	"testing"

	"github.com/vexyl-ai/rekha/internal/geom"
)

// crackLine returns the integer crack vertices of the digitised half-plane
// y < m*x + c for x in [0, n): the staircase a tracer would walk.
func crackLine(m, c float64, n int) []geom.Vec2 {
	var pts []geom.Vec2
	for x := 0; x < n; x++ {
		y := math.Ceil(m*float64(x) + c) // crack sits on the integer above the edge
		p := geom.Vec2{X: float64(x), Y: y}
		if len(pts) > 0 && pts[len(pts)-1].Y != y {
			pts = append(pts, geom.Vec2{X: float64(x), Y: pts[len(pts)-1].Y})
		}
		pts = append(pts, p)
	}
	return pts
}

func TestFitLineRecoversSubpixelLine(t *testing.T) {
	m, c := 0.3, 0.37
	p, dir := FitLine(crackLine(m, c, 40))
	// The crack of a half-plane sits a fraction of a pixel above the edge
	// (the staircase encloses whole pixels); the fit must land on it, not
	// on an outer corner.
	wantY := m*p.X + c + 0.5
	if math.Abs(p.Y-wantY) > 0.3 {
		t.Fatalf("point %v is %.2f off the edge line", p, p.Y-wantY)
	}
	ang := math.Atan2(dir.Y, dir.X) * 180 / math.Pi
	if math.Abs(ang-math.Atan(m)*180/math.Pi) > 1 {
		t.Fatalf("direction %v (%.1f°) off", dir, ang)
	}
}

func TestIntersectLines(t *testing.T) {
	p, ok := Intersect(geom.Vec2{X: 0, Y: 0}, geom.Vec2{X: 1, Y: 1}, geom.Vec2{X: 10, Y: 0}, geom.Vec2{X: -1, Y: 1})
	if !ok || math.Abs(p.X-5) > 1e-9 || math.Abs(p.Y-5) > 1e-9 {
		t.Fatalf("got %v %v, want (5,5)", p, ok)
	}
	if _, ok := Intersect(geom.Vec2{}, geom.Vec2{X: 1, Y: 0}, geom.Vec2{Y: 3}, geom.Vec2{X: 1, Y: 0}); ok {
		t.Fatal("parallel lines must not intersect")
	}
}

// Split points inside FitClosed must not be pinned to integer crack
// corners: each segment endpoint should sit within the quantisation noise
// of the true circle.
func TestFitClosedJunctionsOffGrid(t *testing.T) {
	const r = 96.0
	segs := FitClosed(digitalCircle(r), 1.5)
	for i, s := range segs {
		if d := math.Abs(s.P[0].Len() - r); d > 0.4 {
			t.Errorf("segment %d starts %.2f off the circle at %v", i, d, s.P[0])
		}
	}
}
