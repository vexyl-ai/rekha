package fit

import (
	"math"
	"testing"

	"github.com/vexyl-ai/rekha/internal/geom"
)

// TestCircleGeometricUnbiasedOnShortArc: integer crack points along 50°
// of an r = 80 circle pull Kåsa's radius down by several pixels; the
// geometric fit recovers the circle within half a pixel.
func TestCircleGeometricUnbiasedOnShortArc(t *testing.T) {
	var pts []geom.Vec2
	for a := -90.0; a <= -40; a += 0.8 {
		x := 128 + 80*math.Cos(a*math.Pi/180)
		y := 128 + 80*math.Sin(a*math.Pi/180)
		pts = append(pts, geom.Vec2{X: math.Round(x), Y: math.Round(y)})
	}
	_, rk, ok := Kasa(pts)
	if !ok {
		t.Fatal("kasa failed")
	}
	c, r, ok := CircleGeometric(pts)
	if !ok {
		t.Fatal("geometric fit failed")
	}
	if math.Abs(r-80) > 0.5 || c.Sub(geom.Vec2{X: 128, Y: 128}).Len() > 0.5 {
		t.Fatalf("geometric fit c=%v r=%.2f (kasa r=%.2f)", c, r, rk)
	}
	if math.Abs(r-80) >= math.Abs(rk-80) {
		t.Fatalf("geometric fit r=%.2f is no better than kasa r=%.2f", r, rk)
	}
}

func TestCircleGeometricRejectsCollinear(t *testing.T) {
	pts := []geom.Vec2{{X: 0, Y: 0}, {X: 1, Y: 1}, {X: 2, Y: 2}, {X: 3, Y: 3}}
	if _, _, ok := CircleGeometric(pts); ok {
		t.Fatal("collinear points fitted a circle")
	}
}
